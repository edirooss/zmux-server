package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/edirooss/zmux-server/internal/domain/principal"
	"github.com/edirooss/zmux-server/internal/repo"
	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"
)

// SpecSyncService seeds Redis from a spec file on boot and live-updates Redis on changes.
// New(...) kicks off initial load and a debounced fs watcher; everything else is internal.
type SpecSyncService struct {
	log  *zap.Logger
	repo *repo.Repository

	specPath string
	debounce time.Duration
}

// StartSpecSync constructs the spec sync service, applies the spec once, and starts a debounced watcher.
// The service lives/lifetimes with the provided ctx; cancel ctx to stop the watcher.
func StartSpecSync(ctx context.Context, log *zap.Logger, repo *repo.Repository, specPath string, debounce time.Duration) error {
	if debounce <= 0 {
		debounce = 750 * time.Millisecond
	}
	if specPath == "" {
		specPath = defaultFilePath("configs/spec.json", "/etc/zmux-server/spec.json")
	}
	s := &SpecSyncService{
		log:      log.Named("spec_sync"),
		repo:     repo,
		specPath: specPath,
		debounce: debounce,
	}

	// Initial apply on boot. If it fails, we surface the error: caller can decide to abort startup.
	if err := s.applyOnce(ctx); err != nil {
		return fmt.Errorf("initial apply: %w", err)
	}

	// Spin the filesystem watcher in the background; debounced to coalesce editor writes.
	go s.watch(ctx)

	return nil
}

// --- internal model + helpers ------------------------------------------------

// specFile is the on-disk contract for bindings + tokens.
// Keep it tiny and explicit; validation happens in applyOnce.
type specFile struct {
	Clients map[string][]int64 `json:"clients"`
	Tokens  map[string]struct {
		PrincipalID string `json:"principal_id"`
		Kind        string `json:"kind"` // "admin" | "b2b_client"
	} `json:"tokens"`
}

func (s *SpecSyncService) loadSpec(path string) (*specFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open '%s': %w", path, err)
	}
	defer f.Close()
	raw, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	var spec specFile
	if err := json.Unmarshal(raw, &spec); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	// Normalize nil maps to empty maps to simplify downstream logic.
	if spec.Clients == nil {
		spec.Clients = map[string][]int64{}
	}
	if spec.Tokens == nil {
		spec.Tokens = map[string]struct {
			PrincipalID string `json:"principal_id"`
			Kind        string `json:"kind"`
		}{}
	}
	return &spec, nil
}

// applyOnce overwrites Redis to match the spec file.
// DEV: Simplicity over transactional semantics for MVP: we clear then repopulate.
func (s *SpecSyncService) applyOnce(ctx context.Context) error {
	abs, err := filepath.Abs(s.specPath)
	if err != nil {
		abs = s.specPath
	}
	spec, err := s.loadSpec(abs)
	if err != nil {
		return fmt.Errorf("load spec: %w", err)
	}

	// --- Principals (tokens) ---
	// Strategy: wipe all then reinsert from spec (small cardinality, read-heavy).
	if err := s.repo.Principal.DeleteAll(ctx); err != nil {
		return fmt.Errorf("principals delete all: %w", err)
	}
	for token, p := range spec.Tokens {
		kind, err := parseKind(p.Kind)
		if err != nil {
			return fmt.Errorf("token %q: %w", token, err)
		}
		if err := s.repo.Principal.Upsert(ctx, token, &principal.Principal{
			ID:   p.PrincipalID,
			Kind: kind,
		}); err != nil {
			return fmt.Errorf("principals upsert (%s): %w", token, err)
		}
	}

	// --- B2B client → channels ---
	// Strategy: wipe all client channel sets then reinsert per spec.
	// We use raw Redis SCAN here to remove *stale* clients not present in the spec.
	// Repo-level wipe of ALL client channel sets, then per-client re-create.
	if err := s.repo.B2BClntChnls.DeleteAllClients(ctx); err != nil {
		return fmt.Errorf("b2b delete all clients: %w", err)
	}
	for clientID, chans := range spec.Clients {
		if len(chans) > 0 {
			if err := s.repo.B2BClntChnls.BindChannelIDs(ctx, clientID, chans); err != nil {
				return fmt.Errorf("b2b bind ids (%s): %w", clientID, err)
			}
		}
	}

	s.log.Info("spec applied",
		zap.Int("clients", len(spec.Clients)),
		zap.Int("tokens", len(spec.Tokens)),
		zap.String("path", abs),
	)
	return nil
}

// watch sets up fsnotify and runs a debounced apply on relevant file events.
// DEV: Debounce guards against partial writes / save bursts from editors.
func (s *SpecSyncService) watch(ctx context.Context) {
	abs, err := filepath.Abs(s.specPath)
	if err != nil {
		abs = s.specPath
	}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		s.log.Error("watcher init", zap.Error(err))
		return
	}
	defer w.Close()

	dir := filepath.Dir(abs)
	if err := w.Add(dir); err != nil {
		s.log.Error("watch add dir", zap.String("dir", dir), zap.Error(err))
		return
	}

	// Debounce with a single timer reused; reset on each qualifying event.
	var t *time.Timer
	trigger := func() {
		// Apply with a fresh short-lived context; don't block indefinitely on Redis.
		cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if err := s.applyOnce(cctx); err != nil {
			s.log.Warn("apply failed", zap.Error(err))
		}
	}

	reset := func() {
		if t != nil {
			t.Stop()
		}
		t = time.AfterFunc(s.debounce, trigger)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-w.Events:
			if !ok {
				return
			}
			// Only react to changes on the spec file itself.
			if ev.Name != abs {
				continue
			}
			// Consider Write/Create/Rename as content changes; Remove means the file is gone (ignore until it reappears).
			if ev.Has(fsnotify.Write) || ev.Has(fsnotify.Create) || ev.Has(fsnotify.Rename) {
				reset()
			}
		case err, ok := <-w.Errors:
			if !ok {
				return
			}
			s.log.Warn("watch error", zap.Error(err))
		}
	}
}

// parseKind converts JSON kind strings to domain enum.
func parseKind(s string) (principal.PrincipalKind, error) {
	switch s {
	case "admin":
		return principal.Admin, nil
	case "b2b_client":
		return principal.B2BClient, nil
	default:
		return 0, fmt.Errorf("invalid principal kind: %s", s)
	}
}

// --- helpers -----------------------------------------------------------

var (
	// ErrInvalidSpec is returned when the spec file is missing required fields.
	ErrInvalidSpec = errors.New("invalid spec")
)

// defaultFilePath takes a list of file names as variadic arguments
// and returns the path of the first existing file.
// It returns an empty string if no file is found.
func defaultFilePath(fileNames ...string) string {
	for _, fileName := range fileNames {
		if fileExists(fileName) {
			return fileName
		}
	}
	return ""
}

// fileExists checks if a file or directory exists at the given path.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}
