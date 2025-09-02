package service

import (
	"context"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/edirooss/zmux-server/internal/http/dto"
	"github.com/edirooss/zmux-server/internal/redis"
	"go.uber.org/zap"
)

type SummaryOptions struct {
	// TTL controls how long we serve the in-memory snapshot.
	// 150–400ms works well for 1.5s polling; default 250ms.
	TTL time.Duration
	// RefreshTimeout bounds Redis work for a single refresh.
	// Keep this ≤ your handler budget; default 300ms.
	RefreshTimeout time.Duration
	// Allow serving stale on refresh error (graceful degrade).
	AllowStaleOnError bool
}

func (o *SummaryOptions) setDefaults() {
	if o.TTL <= 0 {
		o.TTL = 250 * time.Millisecond
	}
	if o.RefreshTimeout <= 0 {
		o.RefreshTimeout = 300 * time.Millisecond
	}
}

// SummaryResult lets the handler set headers/telemetry.
type SummaryResult struct {
	Data        []dto.ChannelSummary
	CacheHit    bool
	GeneratedAt time.Time // snapshot timestamp
}

type SummaryService struct {
	log       *zap.Logger
	chanRepo  *redis.ChannelRepository
	remuxRepo *redis.RemuxRepository

	mu      sync.RWMutex
	cache   []dto.ChannelSummary
	expires time.Time
	genAt   time.Time

	opts SummaryOptions
	now  func() time.Time

	sg singleflight.Group
}

// NewSummaryService wires repositories and cache policy.
// Reuse a single instance per process (handlers call Get()).
func NewSummaryService(log *zap.Logger, opts SummaryOptions) *SummaryService {
	log = log.Named("summary_service")
	opts.setDefaults()

	return &SummaryService{
		log:       log,
		chanRepo:  redis.NewChannelRepository(log),
		remuxRepo: redis.NewRemuxRepository(log),
		opts:      opts,
		now:       time.Now,
	}
}

// Get returns the cached snapshot or refreshes it when expired.
// Multiple concurrent refreshes are coalesced.
func (s *SummaryService) Get(ctx context.Context) (SummaryResult, error) {
	// Fast path: fresh cache
	s.mu.RLock()
	if s.cache != nil && s.now().Before(s.expires) {
		out := cloneSummaries(s.cache)
		genAt := s.genAt
		s.mu.RUnlock()
		return SummaryResult{Data: out, CacheHit: true, GeneratedAt: genAt}, nil
	}
	s.mu.RUnlock()

	// Slow path: singleflight refresh
	v, err, _ := s.sg.Do("summary-refresh", func() (any, error) {
		// Double-check freshness after we won the flight
		s.mu.RLock()
		if s.cache != nil && s.now().Before(s.expires) {
			out := cloneSummaries(s.cache)
			genAt := s.genAt
			s.mu.RUnlock()
			return SummaryResult{Data: out, CacheHit: true, GeneratedAt: genAt}, nil
		}
		s.mu.RUnlock()

		ctx, cancel := context.WithTimeout(ctx, s.opts.RefreshTimeout)
		defer cancel()

		start := s.now()
		data, err := s.refresh(ctx)
		if err != nil {
			// Refresh failed: optionally serve stale, else propagate error
			if s.opts.AllowStaleOnError {
				s.mu.RLock()
				if s.cache != nil {
					out := cloneSummaries(s.cache)
					genAt := s.genAt
					s.mu.RUnlock()
					s.log.Warn("summary refresh failed; serving stale", zap.Error(err))
					return SummaryResult{Data: out, CacheHit: true, GeneratedAt: genAt}, nil
				}
				s.mu.RUnlock()
			}
			return nil, err
		}

		// Publish new snapshot
		s.mu.Lock()
		s.cache = data
		s.expires = s.now().Add(s.opts.TTL)
		s.genAt = start
		s.mu.Unlock()

		return SummaryResult{Data: cloneSummaries(data), CacheHit: false, GeneratedAt: start}, nil
	})
	if err != nil {
		return SummaryResult{}, err
	}
	return v.(SummaryResult), nil
}

func (s *SummaryService) Invalidate() {
	s.mu.Lock()
	s.cache = nil
	s.expires = time.Time{}
	s.genAt = time.Time{}
	s.mu.Unlock()
}

// refresh runs the Redis pipeline: channels -> statuses -> ifmt/metrics
func (s *SummaryService) refresh(ctx context.Context) ([]dto.ChannelSummary, error) {
	chs, err := s.chanRepo.GetAll(ctx)
	if err != nil {
		return nil, err
	}

	enabledIDs := make([]int64, 0, len(chs))
	for _, ch := range chs {
		if ch.Enabled {
			enabledIDs = append(enabledIDs, ch.ID)
		}
	}

	statusMap, err := s.remuxRepo.BulkStatus(ctx, enabledIDs)
	if err != nil {
		// Non-fatal: still assemble response
		s.log.Warn("bulk status failed", zap.Error(err))
	}

	liveIDs := make([]int64, 0, len(enabledIDs))
	for _, id := range enabledIDs {
		if st, ok := statusMap[id]; ok && strings.EqualFold(st.Liveness, "Live") {
			liveIDs = append(liveIDs, id)
		}
	}

	extras, err := s.remuxRepo.BulkIfmtMetrics(ctx, liveIDs)
	if err != nil {
		s.log.Warn("bulk ifmt/metrics failed", zap.Error(err))
	}

	out := make([]dto.ChannelSummary, 0, len(chs))
	for _, ch := range chs {
		sum := dto.ChannelSummary{ZmuxChannel: *ch}
		if ch.Enabled {
			if st, ok := statusMap[ch.ID]; ok {
				sum.Status = st
				if extra, ok := extras[ch.ID]; ok {
					sum.Ifmt = extra.Ifmt
					sum.Metrics = extra.Metrics
				}
			}
		}
		out = append(out, sum)
	}
	return out, nil
}

func cloneSummaries(in []dto.ChannelSummary) []dto.ChannelSummary {
	if len(in) == 0 {
		return nil
	}
	out := make([]dto.ChannelSummary, len(in))
	copy(out, in)
	return out
}
