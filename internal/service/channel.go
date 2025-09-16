package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/edirooss/zmux-server/internal/domain/channel"
	"github.com/edirooss/zmux-server/internal/repo"
	"github.com/edirooss/zmux-server/pkg/remuxcmd"
	"github.com/godbus/dbus/v5"
	"go.uber.org/zap"
)

// -----------------------------------------------------------------------------
// ChannelService
// -----------------------------------------------------------------------------
//
// Runtime model
//   • Single process, many concurrent requests.
//   • Mutations for the SAME channel ID are serialized via a per-ID gate.
//   • Reads (Get/List) are lock-free.
//
// Contract (runtime-first)
//   • systemd is source of truth. Side-effects land first, then we persist.
//   • If a systemd operation fails → no Redis changes are made.
//   • If Redis write fails AFTER a successful systemd change → we attempt to
//     roll back the systemd change (best-effort) and return an error.
//   • Update has compensation: if we stopped the old revision already and fail
//     to start the new one, we try to start the old revision again.
//
// Idempotency / semantics
//   • Create(Enabled=false): pure persist.
//   • Create(Enabled=true): start runtime first, then persist.
//   • Update:
//       - Enabled↔Enabled + config changed → stop old rev → start new rev → persist.
//       - Enabled↔Enabled + config same   → no runtime change → persist spec.
//       - false→true                      → start desired rev → persist.
//       - true→false                      → stop current rev → persist disabled.
//   • Delete: stop if running, then delete from Redis; on delete failure and if
//     we stopped it, best-effort re-start the prior revision.
//
// Naming / revisions
//   • Units are transient and revisioned: "zmux-channel-<id>_<rev>.service".
//   • Revision increments when effective runtime args (or restart policy) change.

// ChannelService coordinates repo (Redis) and systemd (via DBus).
type ChannelService struct {
	log     *zap.Logger
	repo    *repo.Repository
	systemd *SystemdManager

	// per-channel locks to serialize mutating requests on the same ID
	muxes sync.Map // map[int64]*gate
}

// gate is a tiny 1-token semaphore with TryLock semantics (non-blocking fast-fail).
type gate struct{ ch chan struct{} }

func newGate() *gate {
	g := &gate{ch: make(chan struct{}, 1)}
	g.ch <- struct{}{} // token present => unlocked
	return g
}
func (g *gate) Lock() { <-g.ch }
func (g *gate) TryLock() bool {
	select {
	case <-g.ch:
		return true
	default:
		return false
	}
}
func (g *gate) Unlock() {
	select {
	case g.ch <- struct{}{}:
	default:
		panic("unlock of unlocked gate")
	}
}

// ErrLocked signals a concurrent mutation is already in flight for this ID.
var ErrLocked = errors.New("channel locked")

// NewChannelService wires dependencies, establishes a DBus connection to systemd,
// then reconciles systemd runtime with the repo (stop zombies, start enabled).
func NewChannelService(log *zap.Logger) (*ChannelService, error) {
	log = log.Named("channel_service")

	// Connect to the system message bus (D-Bus).
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, fmt.Errorf("connect system bus: %w", err)
	}
	// TODO: close on process shutdown.
	// defer conn.Close()

	svc := &ChannelService{
		log:     log,
		repo:    repo.NewRepository(log),
		systemd: NewSystemdManager(conn),
	}

	// Boot-time reconciliation: enforce a clean slate + desired state.
	if err := svc.reconcileOnStart(context.Background()); err != nil {
		return nil, fmt.Errorf("bootstrap reconcile: %w", err)
	}

	return svc, nil
}

// lock acquires the per-ID gate (blocking). Always returns a valid unlock func.
// Safe to call multiple times; same ID maps to the same gate.
func (s *ChannelService) lock(id int64) func() {
	v, _ := s.muxes.LoadOrStore(id, newGate())
	g := v.(*gate)
	g.Lock()
	return func() { g.Unlock() }
}

// tryLock attempts to acquire the per-ID gate without blocking.
func (s *ChannelService) tryLock(id int64) (func(), error) {
	v, _ := s.muxes.LoadOrStore(id, newGate())
	g := v.(*gate)
	if !g.TryLock() {
		return func() {}, fmt.Errorf("id %d: %w", id, ErrLocked)
	}
	return func() { g.Unlock() }, nil
}

// ChannelExists returns true if the channel ID exists in Redis.
func (s *ChannelService) ChannelExists(ctx context.Context, id int64) (bool, error) {
	exists, err := s.repo.Channels.HasID(ctx, id)
	if err != nil {
		return false, fmt.Errorf("has id: %w", err)
	}
	return exists, nil
}

// CreateChannel creates a channel record and (optionally) starts its unit.
// Runtime-first semantics:
//   - If ch.Enabled==true: start transient unit first; on success, persist.
//     If persistence fails, stop unit (best-effort) and return error.
//   - If ch.Enabled==false: no runtime change; just persist.
func (s *ChannelService) CreateChannel(ctx context.Context, ch *channel.ZmuxChannel) error {
	id, err := s.repo.Channels.GenerateID(ctx)
	if err != nil {
		return err
	}

	unlock := s.lock(id)
	defer unlock()

	// Initialize identity and base revision.
	ch.ID = id
	ch.Revision = 1

	if ch.Enabled {
		// Start runtime first so Redis only reflects running state after success.
		unit := unitName(ch)
		argv := remuxArgv(ch)

		if _, err := s.systemd.StartTransientUnit(unit, "/usr/bin/remux", argv, "always", restartUSec(ch)); err != nil {
			return fmt.Errorf("start unit: %w", err)
		}

		// Persist the final (enabled) state.
		if err := s.repo.Channels.Upsert(ctx, ch); err != nil {
			// Rollback: stop new unit
			if _, stopErr := s.systemd.StopUnit(unit); stopErr != nil {
				s.log.Error("rollback systemd failed", zap.String("transition", "OFF→ON"), zap.Error(stopErr))
			}
			return fmt.Errorf("upsert: %w", err)
		}

		return nil
	}

	// Disabled create: pure persist path.
	if err := s.repo.Channels.Upsert(ctx, ch); err != nil {
		return fmt.Errorf("upsert: %w", err)
	}
	return nil
}

// GetChannel returns a single channel by ID (read-only).
func (s *ChannelService) GetChannel(ctx context.Context, id int64) (*channel.ZmuxChannel, error) {
	ch, err := s.repo.Channels.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get: %w", err)
	}
	return ch, nil
}

// ListChannels returns all channels (read-only).
func (s *ChannelService) ListChannels(ctx context.Context) ([]*channel.ZmuxChannel, error) {
	chs, err := s.repo.Channels.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("get all: %w", err)
	}
	return chs, nil
}

// ListChannelsByID returns a subset by IDs (read-only).
func (s *ChannelService) ListChannelsByID(ctx context.Context, ids []int64) ([]*channel.ZmuxChannel, error) {
	chs, err := s.repo.Channels.GetByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("get many: %w", err)
	}
	return chs, nil
}

// UpdateChannel reconciles a channel from its current spec to the desired spec
// using runtime-first semantics with compensation. Each transition always bumps
// the revision (cur.Revision+1), ensuring every state change has a unique unit.
//
// Transition rules (always new revision, no config-diffing):
//   - OFF → ON:
//     Start new unit → Upsert state.
//     If Upsert fails: stop new unit (avoid drift).
//   - ON → OFF:
//     Stop old unit → Upsert disabled.
//     If Upsert fails: restart old unit (avoid outage).
//   - ON → ON:
//     Stop old unit → Start new unit → Upsert state.
//     If start new fails: restart old unit (compensate).
//     If Upsert fails: stop new, restart old (avoid drift).
//   - OFF → OFF:
//     No runtime ops → Upsert only.
//
// Invariants:
//   - Redis only persists states that actually landed.
//   - Any side-effect failure aborts before persistence.
//
// Logging policy:
//   - Only rollback failures are logged (structured, zap-style).
func (s *ChannelService) UpdateChannel(ctx context.Context, ch *channel.ZmuxChannel) error {
	unlock, err := s.tryLock(ch.ID)
	if err != nil {
		return fmt.Errorf("try lock: %w", err)
	}
	defer unlock()

	// Fetch current spec for comparison & rollback context.
	curCh, err := s.repo.Channels.GetByID(ctx, ch.ID)
	if err != nil {
		return fmt.Errorf("get: %w", err)
	}
	oldUnit := unitName(curCh)

	// Always bump revision to ensure each state transition maps to a unique unit.
	// This mandatory for collision-free concurrent changes within systemd-manager layer itself, since it's async by design.
	ch.Revision = curCh.Revision + 1

	switch {
	// OFF → ON: start new unit, then persist.
	case !curCh.Enabled && ch.Enabled:
		newUnit := unitName(ch)
		newArgv := remuxArgv(ch)

		if _, err := s.systemd.StartTransientUnit(newUnit, "/usr/bin/remux", newArgv, "always", restartUSec(ch)); err != nil {
			return fmt.Errorf("start unit: %w", err)
		}
		if err := s.repo.Channels.Upsert(ctx, ch); err != nil {
			// Rollback: stop new unit
			if _, stopErr := s.systemd.StopUnit(newUnit); stopErr != nil {
				s.log.Error("rollback systemd failed", zap.String("transition", "OFF→ON"), zap.Error(stopErr))
			}
			return fmt.Errorf("upsert: %w", err)
		}
		return nil

	// ON → OFF: stop unit, then persist.
	case curCh.Enabled && !ch.Enabled:
		if _, err := s.systemd.StopUnit(oldUnit); err != nil {
			var dbusErr dbus.Error
			if errors.As(err, &dbusErr) && dbusErr.Name == "org.freedesktop.systemd1.NoSuchUnit" {
				// unit doesn’t exist — handle gracefully, just log
				s.log.Warn("unit not loaded", zap.String("transition", "ON→OFF"), zap.String("unit_name", oldUnit))
			} else {
				return fmt.Errorf("stop unit: %w", err)
			}
		}
		if err := s.repo.Channels.Upsert(ctx, ch); err != nil {
			// Rollback: restart old unit
			if _, startErr := s.systemd.StartTransientUnit(oldUnit, "/usr/bin/remux", remuxArgv(curCh), "always", restartUSec(curCh)); startErr != nil {
				s.log.Error("rollback systemd failed", zap.String("transition", "ON→OFF"), zap.Error(startErr))
			}
			return fmt.Errorf("upsert: %w", err)
		}
		return nil

	// ON → ON: stop old, start new, then persist.
	case curCh.Enabled && ch.Enabled:
		if _, err := s.systemd.StopUnit(oldUnit); err != nil {
			var dbusErr dbus.Error
			if errors.As(err, &dbusErr) && dbusErr.Name == "org.freedesktop.systemd1.NoSuchUnit" {
				// unit doesn’t exist — handle gracefully, just log
				s.log.Warn("old unit not loaded", zap.String("transition", "ON→ON"), zap.String("unit_name", oldUnit))
			} else {
				return fmt.Errorf("stop old unit: %w", err)
			}
		}

		newUnit := unitName(ch)
		newArgv := remuxArgv(ch)
		if _, err := s.systemd.StartTransientUnit(newUnit, "/usr/bin/remux", newArgv, "always", restartUSec(ch)); err != nil {
			// Rollback: restart old unit
			if _, startErr := s.systemd.StartTransientUnit(oldUnit, "/usr/bin/remux", remuxArgv(curCh), "always", restartUSec(curCh)); startErr != nil {
				s.log.Error("rollback systemd failed", zap.String("transition", "ON→ON"), zap.Error(startErr))
			}
			return fmt.Errorf("start new unit: %w", err)
		}
		if err := s.repo.Channels.Upsert(ctx, ch); err != nil {
			// Rollback: stop new, restart old
			_, stopErr := s.systemd.StopUnit(newUnit)
			_, startErr := s.systemd.StartTransientUnit(oldUnit, "/usr/bin/remux", remuxArgv(curCh), "always", restartUSec(curCh))
			if stopErr != nil || startErr != nil {
				s.log.Error("rollback systemd failed", zap.String("transition", "ON→ON"), zap.Errors("rollback_errors", []error{stopErr, startErr}))
			}
			return fmt.Errorf("upsert: %w", err)
		}
		return nil

	// OFF → OFF: no runtime ops, just persist.
	default:
		if err := s.repo.Channels.Upsert(ctx, ch); err != nil {
			return fmt.Errorf("upsert: %w", err)
		}
		return nil
	}
}

// DeleteChannel disables the runtime (if running) and deletes the record.
// If deletion fails after stopping the unit, we best-effort re-start the last
// known revision to avoid accidental outage masked by storage failure.
func (s *ChannelService) DeleteChannel(ctx context.Context, id int64) error {
	unlock, err := s.tryLock(id)
	if err != nil {
		return fmt.Errorf("try lock: %w", err)
	}
	defer unlock()

	ch, err := s.repo.Channels.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("get: %w", err)
	}

	wasEnabled := ch.Enabled
	oldUnit := unitName(ch)

	if wasEnabled {
		if _, err := s.systemd.StopUnit(oldUnit); err != nil {
			var dbusErr dbus.Error
			if errors.As(err, &dbusErr) && dbusErr.Name == "org.freedesktop.systemd1.NoSuchUnit" {
				// unit doesn’t exist — handle gracefully, just log
				s.log.Warn("unit not loaded", zap.String("unit_name", oldUnit))
			} else {
				return fmt.Errorf("stop unit: %w", err)
			}
		}
	}

	if err := s.repo.Channels.Delete(ctx, id); err != nil {
		if wasEnabled {
			// Rollback: restart old unit
			if _, startErr := s.systemd.StartTransientUnit(oldUnit, "/usr/bin/remux", remuxArgv(ch), "always", restartUSec(ch)); startErr != nil {
				s.log.Error("rollback systemd failed", zap.Error(startErr))
			}
		}
		return fmt.Errorf("delete: %w", err)
	}

	// Once deleted, we can discard the per-ID gate.
	s.muxes.Delete(id)
	return nil
}

func remuxID(ch *channel.ZmuxChannel) string {
	return strconv.FormatInt(ch.ID, 10) + "_" + strconv.FormatInt(ch.Revision, 10)
}

func unitName(ch *channel.ZmuxChannel) string {
	return "zmux-channel-" + remuxID(ch) + ".service"
}

func remuxArgv(ch *channel.ZmuxChannel) []string {
	return remuxcmd.BuildArgs(ch)
}

func restartUSec(ch *channel.ZmuxChannel) uint64 {
	return uint64(ch.RestartSec) * 1_000_000
}

// reconcileOnStart brings systemd runtime into alignment with the repo’s desired state.
//
// Strategy (two-phase, idempotent):
//  1. Enumerate all currently loaded systemd units and stop any transient unit we own
//     whose name matches "zmux-channel-*.service" but does NOT exist in the repo as
//     an enabled channel (i.e., zombies).
//  2. Start a transient unit for every repo-enabled channel that was not already
//     present in systemd from phase (1).
//
// Semantics & failure policy:
//   - “Zombies” are best-effort to stop: failures are WARNed and reconciliation continues.
//   - Starting desired-but-missing units is REQUIRED: on the first failure, we return an error.
//     This makes constructor/boot fail fast instead of drifting into split-brain.
//   - All systemd calls are asynchronous by design; we don’t wait for state transitions
//     here. Higher-level monitoring (e.g., JobRemoved) or health checks can be added if needed.
//
// Concurrency & idempotency:
//   - Safe to run multiple times; it converges to the same state.
//   - Assumes a single active controller instance; if you run multiple service instances,
//     you may want external leader election to avoid thrash.
//
// Complexity:
//   - O(N + M) where N = #repo channels, M = #loaded systemd units.
//
// Future improvements:
//   - If available in your target systemd, ListUnitsByPatterns could filter server-side.
//   - If you ever need to “refresh” already-present desired units, you’ll need an explicit
//     stop/restart or ReplaceTransientUnit (not universally available).
func (s *ChannelService) reconcileOnStart(ctx context.Context) error {
	// 1) Load desired state from repo and index enabled units by their final unit name.
	chs, err := s.repo.Channels.GetAll(ctx)
	if err != nil {
		return fmt.Errorf("load channels: %w", err)
	}

	// Map of desired unitName -> channel (only for Enabled channels).
	// Using a map gives O(1) membership checks when scanning ListUnits.
	enabledUnits := make(map[string]*channel.ZmuxChannel, len(chs))
	for _, ch := range chs {
		if ch.Enabled {
			enabledUnits[unitName(ch)] = ch
		}
	}

	// 2) Observe actual state from systemd.
	units, err := s.systemd.ListUnits()
	if err != nil {
		return fmt.Errorf("list units: %w", err)
	}

	// Pass A: stop any stray zmux transient services not declared enabled in repo.
	for _, u := range units {
		if !isZmuxServiceName(u.Name) {
			continue
		}

		// If this unit is desired and already present, leave it alone
		// and remove it from the "to start" set.
		if _, ok := enabledUnits[u.Name]; ok {
			delete(enabledUnits, u.Name)
			continue
		}

		// Otherwise it's a zombie; try to stop it (async). Non-fatal on failure.
		if _, err := s.systemd.StopUnit(u.Name); err != nil {
			var dbusErr dbus.Error
			if errors.As(err, &dbusErr) && dbusErr.Name == "org.freedesktop.systemd1.NoSuchUnit" {
				// The unit raced away; not a big deal.
				s.log.Warn("zombie unit already gone", zap.String("unit_name", u.Name))
				continue
			}
			s.log.Warn("stop zombie unit failed", zap.String("unit_name", u.Name), zap.Error(err))
		}
	}

	// Pass B: start any desired-but-missing units.
	// If any start fails, fail the reconcile to avoid silent drift.
	for unit, ch := range enabledUnits {
		if _, err := s.systemd.StartTransientUnit(unit, "/usr/bin/remux", remuxArgv(ch), "always", restartUSec(ch)); err != nil {
			return fmt.Errorf("start unit %q: %w", unit, err)
		}
	}

	return nil
}

// isZmuxServiceName reports whether the systemd unit name refers to a zmux-managed
// transient service. We match both the expected prefix and the .service suffix to
// avoid touching timers, slices, or other unit types with similar names.
func isZmuxServiceName(name string) bool {
	return strings.HasPrefix(name, "zmux-channel-") && strings.HasSuffix(name, ".service")
}
