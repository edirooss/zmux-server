package services

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	"github.com/edirooss/zmux-server/pkg/models/channelmodel"
	"github.com/edirooss/zmux-server/redis"
	"go.uber.org/zap"
)

// -----------------------------------------------------------------------------
// ChannelService
// -----------------------------------------------------------------------------
// DESIGN CONTRACT
//
// Runtime model
//  • Single server process, many concurrent requests.
//  • Mutations to the SAME channel ID are serialized via a per-ID *sync.Mutex*.
//  • Reads (Get/List) are lock-free for throughput.
//
// Consistency target
//  • systemd (runtime) is treated as the source of truth for what is actually
//    running. Redis must reflect the runtime state only after a side-effect
//    has succeeded. Therefore, we execute side-effects FIRST, then persist.
//
// Failure policy
//  • If a systemd operation fails → we DO NOT mutate Redis. Caller gets an
//    error and the previously persisted state remains intact.
//  • If Redis persistence fails AFTER a successful systemd change → we attempt
//    a best-effort rollback of the systemd change to avoid drift, then return
//    an error. Rollback errors are swallowed (logged by caller), never mask the
//    primary error.
//  • All mutators (Create/Update/Delete/Enable/Disable) run inside the per-ID
//    critical section so Update↔Update/Update↔Delete etc. cannot interleave.
//
// Idempotency contract
//  • EnableChannel & DisableChannel are idempotent: calling twice is safe.
//  • UpdateChannel will (re)enable the unit when Enabled=true (treat as a
//    restart if already enabled) and disable when Enabled=false.
//
// API mapping guidance (caller-level)
//  • Wraps redis.ErrChannelNotFound. Callers should map to HTTP 404.
//  • All other errors are 5xx (usually 500). Validation happens at handlers.
// -----------------------------------------------------------------------------

type ChannelService struct {
	repo    *redis.ChannelRepository
	systemd *SystemdService

	// per-channel locks to serialize mutations on the same ID
	muxes sync.Map // map[int64]*sync.Mutex
}

// NewChannelService constructs the ChannelService with its dependencies.
func NewChannelService(log *zap.Logger) (*ChannelService, error) {
	log = log.Named("channel_service")

	systemd, err := NewSystemdService(log)
	if err != nil {
		return nil, fmt.Errorf("new systemd service: %w", err)
	}
	return &ChannelService{
		repo:    redis.NewChannelRepository(log),
		systemd: systemd,
	}, nil
}

// lock acquires a per-channel mutex and returns an unlock func. Safe to call
// multiple times; the same ID always maps to the same *sync.Mutex.
func (s *ChannelService) lock(id int64) func() {
	v, _ := s.muxes.LoadOrStore(id, &sync.Mutex{})
	m := v.(*sync.Mutex)
	m.Lock()
	return func() { m.Unlock() }
}

// CreateChannel creates a new channel, optionally
// commits its systemd unit and enables it, and only then persists the channel document to Redis.
//
// Happy path
//  1. Generate ID (atomic via Redis INCR), acquire per-ID lock.
//  2. If ch.Enabled is true → commit systemd unit (definition exists/updated for this channel) and enable the service (runtime change).
//  3. Persist final object to Redis (reflecting the runtime state).
//
// Failure modes & resulting state
//   - ID generation fails → nothing happened (no side-effects), error returned.
//   - If ch.Enabled is true → commitSystemdService fails → NO Redis write; NO enable attempt. Unit may be
//     absent or partially written per systemd layer’s semantics. Caller gets err.
//   - If ch.Enabled is true → enableChannel fails → unit file was created/updated, service NOT enabled;
//     NO Redis write. An ID has been consumed (gap is acceptable). Caller gets err.
//   - repo.Set fails after optionally enabling the channel → service running with no record.
//     We best‑effort DISABLE the service to remove drift, then return error.
//
// Postconditions on success
//   - Unit committed; service enabled iff ch.Enabled=true; Redis reflects that.
func (s *ChannelService) CreateChannel(ctx context.Context, ch *channelmodel.ZmuxChannel) error {
	id, err := s.repo.GenerateID(ctx)
	if err != nil {
		return err
	}
	unlock := s.lock(id)
	defer unlock()

	// Write id to obj
	ch.ID = id

	// If requested, enable the channel now. If this fails, abort without persisting.
	if ch.Enabled {
		// Commit unit first so that the systemd definition exists.
		if err := s.commitSystemdService(ch); err != nil {
			// DEV: At this point nothing persisted; caller may retry safely.
			return fmt.Errorf("commit systemd service: %w", err)
		}

		if err := s.enableChannel(ch.ID); err != nil {
			// DEV: Unit file exists but runtime is not enabled; we purposely do not
			// persist to avoid Redis claiming Enabled=true when it is not. Skip ID
			// reuse. Observability: handler logs the error.
			return fmt.Errorf("enable channel: %w", err)
		}
	}

	// Persist the final, *actual* state to Redis.
	if err := s.repo.Set(ctx, ch); err != nil {
		// DEV: Avoid drift where runtime says enabled but Redis has no record.
		if ch.Enabled {
			_ = s.disableChannel(ch.ID) // best-effort rollback; do not mask Set error
		}
		return fmt.Errorf("set: %w", err)
	}
	return nil
}

// GetChannel returns a channel by ID (read-only, no locks).
// Failure modes
//   - redis.ErrChannelNotFound wrapped → callers should map to 404.
func (s *ChannelService) GetChannel(ctx context.Context, id int64) (*channelmodel.ZmuxChannel, error) {
	ch, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get: %w", err)
	}
	return ch, nil
}

// ListChannels returns all channels (read-only, no locks).
// Failure modes
//   - Any Redis error is returned as-is (wrapped) → callers map to 500.
func (s *ChannelService) ListChannels(ctx context.Context) ([]*channelmodel.ZmuxChannel, error) {
	chs, err := s.repo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list: %w", err)
	}
	return chs, nil
}

// UpdateChannel loads the prev channel config,
// toggles enablement and commits the unit if updated channel is enabled, and persists the resulting state.
//
// Happy path
//  1. Load current → prevEnabled snapshot.
//  2. If Enabled=true → commit systemd unit (ensure definition reflects new config).
//  3. If Enabled=true → (re)enable service (treat as restart if already running).
//     If Enabled=false & was enabled → disable service.
//  4. Persist the final object to Redis.
//
// Failure modes & resulting state
//   - repo.Get fails → nothing changed; error returned.
//   - commitSystemdService fails → NO runtime toggles, NO Redis write; error.
//   - enableChannel/disableChannel fails → NO Redis write; unit may be updated but
//     runtime not in the desired state; caller gets error; no drift in Redis.
//   - repo.Set fails at the end → runtime was changed; Redis still has old state.
//     We currently return error WITHOUT rollback (safer for update semantics
//     because prior config might already be applied). Consider compensating
//     actions if drift is unacceptable for your domain.
//
// Postconditions on success
//   - Runtime reflects desired config & enablement; Redis matches it.
func (s *ChannelService) UpdateChannel(ctx context.Context, ch *channelmodel.ZmuxChannel) error {
	unlock := s.lock(ch.ID)
	defer unlock()

	// Load current from Redis
	cur, err := s.repo.Get(ctx, ch.ID)
	if err != nil {
		return fmt.Errorf("get: %w", err)
	}
	prevEnabled := cur.Enabled

	// Reconcile enablement.
	if ch.Enabled {
		// Commit (idempotent) so the unit reflects new config.
		if err := s.commitSystemdService(ch); err != nil {
			return fmt.Errorf("commit systemd service: %w", err)
		}

		// DEV: Treat as restart semantics — (re)enable to pick up new config.
		if err := s.enableChannel(ch.ID); err != nil {
			return fmt.Errorf("enable channel: %w", err)
		}
	} else if prevEnabled {
		if err := s.disableChannel(ch.ID); err != nil {
			return fmt.Errorf("disable channel: %w", err)
		}
	}

	// Persist final state to Redis.
	if err := s.repo.Set(ctx, ch); err != nil {
		// DEV: We do not attempt to roll back runtime here because the unit config
		// already changed and may be live. Rolling back could be more disruptive.
		// If we want to require strict no-drift, we need to introduce a compensating write
		// or a background reconciler.
		return fmt.Errorf("set: %w", err)
	}
	return nil
}

// DeleteChannel disables the unit if needed and deletes the record.
//
// Happy path
//  1. Load current; snapshot wasEnabled.
//  2. If enabled → disable service.
//  3. Delete from Redis.
//
// Failure modes & resulting state
//   - repo.Get fails → nothing changed.
//   - disableChannel fails → Redis untouched; runtime remains enabled; error.
//   - repo.Delete fails after disabling → runtime is disabled but record remains;
//     best‑effort re-enable to avoid accidental outage; error returned.
//
// Postconditions on success
//   - Record removed from Redis; service disabled (if it had been enabled).
func (s *ChannelService) DeleteChannel(ctx context.Context, id int64) error {
	unlock := s.lock(id)
	defer unlock()

	ch, err := s.repo.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("get: %w", err)
	}

	wasEnabled := ch.Enabled
	if wasEnabled {
		if err := s.disableChannel(ch.ID); err != nil {
			return fmt.Errorf("disable channel: %w", err)
		}
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		// Recovery path: try to put the service back up if we brought it down.
		if wasEnabled {
			_ = s.enableChannel(ch.ID)
		}
		return fmt.Errorf("delete: %w", err)
	}

	s.muxes.Delete(id) // once deleted we can also drop the mutex entry.
	return nil
}

// EnableChannel ensures the channel exists, commits its unit (to guarantee
// definition is present), enables the service, and *persists Enabled=true*.
//
// Happy path
//  1. Load channel, ignore if already enabled (idempotent early return).
//  2. Commit unit to ensure definition exists/is current.
//  3. Enable service.
//  4. Persist Enabled=true to Redis (runtime and persisted converge).
//
// Failure modes & resulting state
//   - repo.Get fails → nothing changed.
//   - commitSystemdService fails → NO enable; NO Redis change.
//   - enableChannel fails → unit exists, runtime not enabled; NO Redis change.
//   - repo.Set fails after enabling → runtime enabled but Redis says false;
//     best‑effort DISABLE to remove drift; error returned.
func (s *ChannelService) EnableChannel(ctx context.Context, id int64) error {
	unlock := s.lock(id)
	defer unlock()

	ch, err := s.repo.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("get channel: %w", err)
	}
	if ch.Enabled {
		return nil // idempotent
	}

	if err := s.enableChannel(ch.ID); err != nil {
		return fmt.Errorf("enable channel: %w", err)
	}

	ch.Enabled = true
	if err := s.repo.Set(ctx, ch); err != nil {
		// rollback runtime effect so Redis/API and systemd do not drift
		_ = s.disableChannel(ch.ID)
		return fmt.Errorf("set: %w", err)
	}
	return nil
}

// DisableChannel ensures the channel exists, disables the systemd unit, and
// *persists Enabled=false*.
//
// Happy path
//  1. Load channel, ignore if already disabled (idempotent early return).
//  2. Disable service.
//  3. Persist Enabled=false to Redis.
//
// Failure modes & resulting state
//   - repo.Get fails → nothing changed.
//   - disableChannel fails → runtime remains enabled; NO Redis change.
//   - repo.Set fails after disabling → runtime disabled but Redis says true;
//     best‑effort RE-ENABLE to remove drift; error returned.
func (s *ChannelService) DisableChannel(ctx context.Context, id int64) error {
	unlock := s.lock(id)
	defer unlock()

	ch, err := s.repo.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("get channel: %w", err)
	}
	if !ch.Enabled {
		return nil // idempotent
	}

	if err := s.disableChannel(ch.ID); err != nil {
		return fmt.Errorf("disable channel: %w", err)
	}

	ch.Enabled = false
	if err := s.repo.Set(ctx, ch); err != nil {
		// rollback runtime effect so Redis/API and systemd do not drift
		_ = s.enableChannel(ch.ID)
		return fmt.Errorf("set: %w", err)
	}
	return nil
}

// enableChannel is a thin wrapper around systemd.EnableService for a channel.
// Kept private to make higher-level flows explicit and testable.
func (s *ChannelService) enableChannel(channelID int64) error {
	serviceName := fmt.Sprintf("zmux-channel-%d", channelID)
	if err := s.systemd.EnableService(serviceName); err != nil {
		return fmt.Errorf("enable systemd service: %w", err)
	}
	return nil
}

// disableChannel is a thin wrapper around systemd.DisableService for a channel.
func (s *ChannelService) disableChannel(channelID int64) error {
	serviceName := fmt.Sprintf("zmux-channel-%d", channelID)
	if err := s.systemd.DisableService(serviceName); err != nil {
		return fmt.Errorf("disable systemd service: %w", err)
	}
	return nil
}

// commitSystemdService renders/commits the systemd unit for a channel. This is
// called for create and update flows, and also before enabling to ensure the
// unit exists and is up-to-date. Consider this idempotent with respect to the
// same inputs; repeated calls are cheap compared to failed starts at runtime.
//
// Note: Channel has to be enabled (i,e. forces input.URL to be non-null)
func (s *ChannelService) commitSystemdService(channel *channelmodel.ZmuxChannel) error {
	cfg := SystemdServiceConfig{
		ServiceName: fmt.Sprintf("zmux-channel-%d", channel.ID),
		ExecStart:   BuildRemuxExecStart(channel),
		RestartSec:  strconv.FormatUint(uint64(channel.RestartSec), 10),
	}
	if err := s.systemd.CommitService(cfg); err != nil {
		return fmt.Errorf("commit systemd service: %w", err)
	}
	return nil
}
