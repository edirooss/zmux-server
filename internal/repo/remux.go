package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"go.uber.org/zap"
)

func remuxStatusKey(id int64) string  { return "remux:" + strconv.FormatInt(id, 10) + ":status" }
func remuxIfmtKey(id int64) string    { return "remux:" + strconv.FormatInt(id, 10) + ":ifmt" }
func remuxMetricsKey(id int64) string { return "remux:" + strconv.FormatInt(id, 10) + ":metrics" }

// RemuxRepository provides Redis-backed access to remux monitoring data.
//
// This repository operates over monitoring data that is continuously refreshed
// by remux processes and may disappear or changed at any time.
// Consumers must treat all reads as *eventually consistent snapshots* rather than durable state.
type RemuxRepository struct {
	client *RedisClient
	log    *zap.Logger
}

func newRemuxRepository(log *zap.Logger, client *RedisClient) *RemuxRepository {
	log = log.Named("remux")
	return &RemuxRepository{
		log:    log,
		client: client,
	}
}

// RemuxStatus mirrors the JSON stored at remux:<id>:status.
//
//   - Online: true when media reading/processing is currently active/successful.
//   - Event:  contains a user-facing message (step label or error details) and
//     a timestamp of when it was recorded.
type RemuxStatus struct {
	Online bool `json:"online"`
	Event  struct {
		Message string `json:"msg"` // step label OR error message
		At      int64  `json:"at"`  // UTC millis when this event was recorded
	} `json:"event"`
}

// RemuxSummary bundles RemuxStatus with optional ifmt/metrics JSON blobs for "Live" remuxers.
type RemuxSummary struct {
	Status  *RemuxStatus     `json:"status,omitempty"`
	Ifmt    *json.RawMessage `json:"ifmt,omitempty"`    // optional; if Status.Liveness != "Live", always missing
	Metrics *json.RawMessage `json:"metrics,omitempty"` // optional; if Status.Liveness != "Live", always missing
}

// GetStatusesByID fetches remux:<id>:status for the provided channel IDs via a single
// MGET operation.
//
//   - Missing keys are ignored silently (e.g. channel never created, already deleted or never enabled, remux not yet started).
//   - All results should be treated as *non-transactional snapshots*; concurrent
//     writes may cause transient inconsistencies.
func (r *RemuxRepository) GetStatusesByID(ctx context.Context, ids []int64) (map[int64]*RemuxStatus, error) {
	if len(ids) == 0 {
		return map[int64]*RemuxStatus{}, nil
	}

	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = remuxStatusKey(id)
	}

	vals, err := r.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("mget: %w", err)
	}

	out := make(map[int64]*RemuxStatus)
	for i, v := range vals {
		if v == nil {
			r.log.Warn(
				"remux status missing during MGET",
				zap.String("key", keys[i]),
				zap.Int("index", i),
			)
			continue
		}
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("key %s at index %d: unexpected type (got %T, want string)", keys[i], i, v)
		}
		var st RemuxStatus
		if err := json.Unmarshal([]byte(s), &st); err != nil {
			return nil, fmt.Errorf("key %s at index %d: unmarshal: %w", keys[i], i, err)
		}
		out[ids[i]] = &st
	}
	return out, nil
}

// GetSummariesByID retrieves a combined status/ifmt/metrics view for the
// given channel IDs.
//
// The workflow is:
//
//  1. Fetch all remux:<id>:status entries in a single MGET.
//     - Missing keys are skipped (e.g. channel never created, already deleted or never enabled, remux not yet started).
//     - Results are returned in a map keyed by ID.
//  2. For IDs where Status.Liveness == "Live", fetch corresponding optional
//     remux:<id>:ifmt and remux:<id>:metrics entries in a second batched MGET.
//     - Missing values are treated as nil/absent (e.g., yet to be set by remux changed to "Dead" between the calls).
//
// Return value:
//   - The returned map contains one RemuxSummary per ID that had a valid status.
//   - For non-live remuxers, Ifmt and Metrics will always be nil.
//   - For live remuxers, Ifmt and Metrics may still be nil if not present in Redis.
//
// Consistency model:
//   - Reads are *eventually consistent snapshots*.
//   - Status and ifmt/metrics are fetched in two separate MGETs and may not
//     reflect an atomic point-in-time view.
func (r *RemuxRepository) GetSummariesByID(ctx context.Context, ids []int64) (map[int64]*RemuxSummary, error) {
	if len(ids) == 0 {
		return map[int64]*RemuxSummary{}, nil
	}

	// fetch statuses
	currentStatusList, err := r.GetStatusesByID(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("get status by ids: %w", err)
	}

	// build map of summaries + collect live IDs
	summariesByID := make(map[int64]*RemuxSummary, len(currentStatusList))
	liveIDs := make([]int64, 0, len(currentStatusList))

	for id, status := range currentStatusList {
		summariesByID[id] = &RemuxSummary{Status: status}
		if status.Online {
			liveIDs = append(liveIDs, id)
		}
	}

	// fetch ifmt/metrics only for live IDs
	if len(liveIDs) > 0 {
		keys := make([]string, 0, len(liveIDs)*2)
		for _, id := range liveIDs {
			keys = append(keys, remuxIfmtKey(id), remuxMetricsKey(id))
		}

		vals, err := r.client.MGet(ctx, keys...).Result()
		if err != nil {
			return nil, fmt.Errorf("mget ifmt/metrics: %w", err)
		}

		// walk results in steps of 2 (ifmt, metrics)
		for i, id := range liveIDs {
			ifmt, err := optionalVal(vals[2*i])
			if err != nil {
				return nil, fmt.Errorf("ifmt for id %d: %w", id, err)
			}
			metrics, err := optionalVal(vals[2*i+1])
			if err != nil {
				return nil, fmt.Errorf("metrics for id %d: %w", id, err)
			}
			summariesByID[id].Ifmt = ifmt
			summariesByID[id].Metrics = metrics
		}
	}

	return summariesByID, nil
}

func optionalVal(v interface{}) (*json.RawMessage, error) {
	if v == nil {
		return nil, nil // value missing (optional)
	}
	s, ok := v.(string)
	if !ok {
		return nil, fmt.Errorf("unexpected type (got %T, want string)", v)
	}
	rawJSON := json.RawMessage(s)
	return &rawJSON, nil
}
