package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	b2bclient "github.com/edirooss/zmux-server/internal/domain/b2b-client"
	"github.com/edirooss/zmux-server/internal/domain/channel"
	"github.com/edirooss/zmux-server/internal/infrastructure/datastore"
	"github.com/edirooss/zmux-server/internal/infrastructure/objectstore"
	"github.com/edirooss/zmux-server/internal/infrastructure/processmgr"
	"github.com/edirooss/zmux-server/pkg/remuxcmd"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const (
	b2bClientKeyPrefix = "zmux:b2b_client:" // zmux:b2b_client:<id> → JSON(B2BClientModel)
)

type B2BClientService struct {
	log *zap.Logger

	mu        sync.RWMutex
	logmngr   *processmgr.LogManager
	procmngrs map[int64]*processmgr.ProcessManager2 // B2B clients channels runtime
	ds        *datastore.DataStore                  // Redis-based persistent store
	objs      *objectstore.ObjectStore              // in-memory object store of B2BClient domain objects
	byToken   map[string]*b2bclient.B2BClient       // in-memory token-based index of B2BClient domain objects

	channelB2BClientID  map[int64]int64   // maps channel ID to its associated B2B client ID
	b2bClientChannelIDs map[int64][]int64 // maps B2B client ID to a list of their channel IDs

	// internal counters
	b2bClientEnabledChannelsUsage map[int64]int64
	b2bClientEnabledOutputsUsage  map[int64]map[string]int64
	b2bClientOnlineChannelsUsage  map[int64]int64
}

func NewB2BClientService(ctx context.Context, log *zap.Logger, rdb *redis.Client, logmngr *processmgr.LogManager) (*B2BClientService, error) {
	if log == nil {
		log = zap.NewNop()
	}
	log = log.Named("b2b-client-service")

	ds, err := datastore.NewDataStore(ctx, log, rdb, b2bClientKeyPrefix)
	if err != nil {
		return nil, fmt.Errorf("datastore: %w", err)
	}

	s := &B2BClientService{
		log:     log,
		logmngr: logmngr,

		procmngrs: make(map[int64]*processmgr.ProcessManager2),
		ds:        ds,
		objs:      objectstore.NewObjectStore(log),
		byToken:   make(map[string]*b2bclient.B2BClient),

		channelB2BClientID:  make(map[int64]int64),
		b2bClientChannelIDs: make(map[int64][]int64),

		b2bClientEnabledChannelsUsage: make(map[int64]int64),
		b2bClientEnabledOutputsUsage:  make(map[int64]map[string]int64),
		b2bClientOnlineChannelsUsage:  make(map[int64]int64),
	}

	if err := s.reconcile(ctx); err != nil {
		return nil, fmt.Errorf("reconcile: %w", err)
	}

	return s, nil
}

func (s *B2BClientService) Create(ctx context.Context, r *b2bclient.B2BClientResource) (*b2bclient.B2BClientView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	token, err := genUniqueB2BToken(s.byToken)
	if err != nil {
		return nil, fmt.Errorf("generate token: %w", err)
	}

	model := b2bclient.NewB2BClientModel(r, token)
	raw, err := json.Marshal(model)
	if err != nil {
		return nil, fmt.Errorf("json marshal: %w", err)
	}

	b2bclntID, err := s.ds.Create(ctx, raw)
	if err != nil {
		return nil, fmt.Errorf("create: %w", err)
	}

	b2bclnt := b2bclient.NewB2BClient(model, b2bclntID)
	s.objs.Upsert(b2bclntID, b2bclnt)
	s.byToken[b2bclnt.BearerToken] = b2bclnt
	s.b2bClientEnabledOutputsUsage[b2bclntID] = make(map[string]int64)
	s.procmngrs[b2bclntID] = processmgr.NewProcessManager2(zap.NewNop(), s.logmngr, b2bclnt.Quotas.OnlineChannels.MaxPreflight, b2bclnt.Quotas.OnlineChannels.Quota)

	return s.buildViewUnsafe(b2bclnt), nil
}

func (s *B2BClientService) Update(ctx context.Context, b2bclntID int64, r *b2bclient.B2BClientResource) (*b2bclient.B2BClientView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	val, ok := s.objs.GetOne(b2bclntID)
	if !ok {
		return nil, ErrNotFound
	}
	b2bclnt := val.(*b2bclient.B2BClient)

	procmngr, ok := s.procmngrs[b2bclntID]
	if !ok {
		return nil, ErrNotFound
	}

	nextModel := b2bclient.NewB2BClientModel(r, b2bclnt.BearerToken)
	raw, err := json.Marshal(nextModel)
	if err != nil {
		return nil, fmt.Errorf("json marshal: %w", err)
	}
	if err := s.ds.Update(ctx, b2bclntID, raw); err != nil {
		return nil, fmt.Errorf("update: %w", err)
	}

	b2bclnt = b2bclient.NewB2BClient(nextModel, b2bclntID)
	s.objs.Upsert(b2bclntID, b2bclnt)
	s.byToken[b2bclnt.BearerToken] = b2bclnt
	procmngr.UpdateLimits(b2bclnt.Quotas.OnlineChannels.MaxPreflight, b2bclnt.Quotas.OnlineChannels.Quota)

	return s.buildViewUnsafe(b2bclnt), nil
}

// Delete removes a B2B client by ID and updates in-memory indices.
func (s *B2BClientService) Delete(ctx context.Context, b2bclntID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	val, ok := s.objs.GetOne(b2bclntID)
	if !ok {
		return ErrNotFound
	}
	b2bclnt := val.(*b2bclient.B2BClient)

	if len(s.b2bClientChannelIDs[b2bclntID]) != 0 {
		return fmt.Errorf("cannot delete; channels attached")
	}

	if err := s.ds.Delete(ctx, b2bclntID); err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	s.objs.Delete(b2bclntID)
	delete(s.byToken, b2bclnt.BearerToken)
	delete(s.procmngrs, b2bclntID)

	return nil
}

func (s *B2BClientService) RegisterChannel(b2bclntID int64, ch *channel.ZmuxChannel) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// index channel → client
	s.channelB2BClientID[ch.ID] = b2bclntID

	// append to client's list
	s.b2bClientChannelIDs[b2bclntID] = append(s.b2bClientChannelIDs[b2bclntID], ch.ID)

	// update counters
	if ch.Enabled {
		s.b2bClientEnabledChannelsUsage[b2bclntID]++
	}

	for _, out := range ch.Outputs {
		if out.Enabled {
			s.b2bClientEnabledOutputsUsage[b2bclntID][out.Ref]++
		}
	}

	// add to proc manager
	ch.Interactive = true
	s.procmngrs[b2bclntID].Add(
		ch.ID,
		remuxcmd.BuildArgv(ch),
		time.Duration(ch.RestartSec)*time.Second,
	)
}

func (s *B2BClientService) UnregisterChannel(b2bclntID int64, ch *channel.ZmuxChannel) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.channelB2BClientID, ch.ID)
	s.b2bClientChannelIDs[b2bclntID] = remove(s.b2bClientChannelIDs[b2bclntID], ch.ID)

	if ch.Enabled {
		s.b2bClientEnabledChannelsUsage[b2bclntID]--
	}

	for _, o := range ch.Outputs {
		if o.Enabled {
			s.b2bClientEnabledOutputsUsage[b2bclntID][o.Ref]--
		}
	}

	// remove from process manager
	s.procmngrs[b2bclntID].Remove(ch.ID)
}

func (s *B2BClientService) GetOne(id int64) (*b2bclient.B2BClientView, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	val, ok := s.objs.GetOne(id)
	if !ok {
		return nil, ErrNotFound
	}
	return s.buildViewUnsafe(val.(*b2bclient.B2BClient)), nil
}

func (s *B2BClientService) GetMany(ids []int64) ([]*b2bclient.B2BClientView, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	vals, _ := s.objs.GetMany(ids)
	if len(vals) == 0 {
		return []*b2bclient.B2BClientView{}, nil
	}

	views := make([]*b2bclient.B2BClientView, 0, len(vals))
	for _, val := range vals {
		views = append(views, s.buildViewUnsafe(val.(*b2bclient.B2BClient)))
	}
	return views, nil
}

func (s *B2BClientService) GetList() ([]*b2bclient.B2BClientView, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, vals := s.objs.GetList()
	if len(vals) == 0 {
		return []*b2bclient.B2BClientView{}, nil
	}

	views := make([]*b2bclient.B2BClientView, 0, len(vals))
	for _, val := range vals {
		views = append(views, s.buildViewUnsafe(val.(*b2bclient.B2BClient)))
	}
	return views, nil
}

func (s *B2BClientService) Exists(id int64) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, ok := s.objs.GetOne(id)
	return ok
}

// reconcile loads persisted records into domain objects and rebuilds in-memory indices.
//
//   - DB (Persistence Layer) → Domain (in-memory)
func (s *B2BClientService) reconcile(ctx context.Context) error {
	b2bclntIDs, csBytes, err := s.ds.GetList(ctx)
	if err != nil {
		return fmt.Errorf("get list: %w", err)
	}

	for i, b2bclntID := range b2bclntIDs {
		var model b2bclient.B2BClientModel
		if err := json.Unmarshal(csBytes[i], &model); err != nil {
			// Data corruption detected - should never happen in normal operation.
			// Possible causes: manual Redis edits, serialization bugs, bit flips.
			s.log.Error("corrupted data detected",
				zap.Int64("id", b2bclntID),
				zap.String("data_preview", safePreview(csBytes[i])),
				zap.Error(err))
			return fmt.Errorf("json unmarshal: %w", err)
		}

		b2bclnt := b2bclient.NewB2BClient(&model, b2bclntID)
		s.objs.Upsert(b2bclntID, b2bclnt)
		s.byToken[b2bclnt.BearerToken] = b2bclnt
		s.b2bClientEnabledOutputsUsage[b2bclntID] = make(map[string]int64)
		s.procmngrs[b2bclntID] = processmgr.NewProcessManager2(zap.NewNop(), s.logmngr, b2bclnt.Quotas.OnlineChannels.MaxPreflight, b2bclnt.Quotas.OnlineChannels.Quota)
	}

	return nil
}

// LookupByToken returns a domain object by bearer token from the in-memory index.
func (s *B2BClientService) LookupByToken(token string) (*b2bclient.B2BClient, bool) {
	s.mu.RLock()
	b2bClient, ok := s.byToken[token]
	s.mu.RUnlock()
	if !ok {
		return nil, false
	}
	return b2bClient, true
}

// LookupByChannelID returns a domain object by channel ID from the in-memory index.
func (s *B2BClientService) LookupByChannelID(chnlID int64) (int64, bool) {
	s.mu.RLock()
	b2bClient, ok := s.channelB2BClientID[chnlID]
	s.mu.RUnlock()
	if !ok {
		return 0, false
	}
	return b2bClient, true
}

// ----- helpers --------------------------------------------------------------

// genUniqueB2BToken returns a collision-free, URL-safe bearer token prefixed with "b2b_".
// Example: b2b_pXGgPptQyC4w2UE1-LyYuwXzle7bnBQ1JmBiY1Ev6xI
func genUniqueB2BToken(index map[string]*b2bclient.B2BClient) (string, error) {
	const tokenBytes = 32
	const prefix = "b2b_"

	for {
		b := make([]byte, tokenBytes)
		if _, err := rand.Read(b); err != nil {
			return "", fmt.Errorf("rand read: %w", err)
		}
		raw := base64.RawURLEncoding.EncodeToString(b)
		token := prefix + raw
		if _, exists := index[token]; exists {
			continue
		}
		return token, nil
	}
}

// safePreview shortens a byte slice for safe printing.
func safePreview(b []byte) string {
	n := 100
	if len(b) < n {
		n = len(b)
	}
	return string(b[:n])
}

var ErrConflict = errors.New("conflict")

// buildViewUnsafe; must be locked.
func (s *B2BClientService) buildViewUnsafe(b2bclnt *b2bclient.B2BClient) *b2bclient.B2BClientView {
	return b2bclnt.View(
		s.b2bClientEnabledChannelsUsage[b2bclnt.ID],
		s.b2bClientEnabledOutputsUsage[b2bclnt.ID],
		s.procmngrs[b2bclnt.ID].Onflight(),
		s.b2bClientChannelIDs[b2bclnt.ID],
	)
}

// remove returns a new slice with the first occurrence of the target integer removed.
// If the target is not present, the original slice is returned unchanged.
func remove[T comparable](s []T, target T) []T {
	for i, v := range s {
		if v == target {
			return append(s[:i], s[i+1:]...)
		}
	}
	return s // target not found
}
