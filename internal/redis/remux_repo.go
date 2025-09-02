package redis

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

// RemuxRepository deals with monitoring keys remux:<id>:*
type RemuxRepository struct {
	client *Client
	log    *zap.Logger
}

func newRemuxRepository(log *zap.Logger, client *Client) *RemuxRepository {
	log = log.Named("remux")
	return &RemuxRepository{
		log:    log,
		client: client,
	}
}

// RemuxStatus mirrors the JSON stored at remux:<id>:status
// Example stored value (string):
//
//	{
//	  "liveness": "Dead" | "Live",
//	  "metadata": "...",
//	  "timestamp": 0
//	}
//
// Keep field names/json tags aligned with stored JSON to avoid re-mapping.
type RemuxStatus struct {
	Liveness  string `json:"liveness"`
	Metadata  string `json:"metadata"`
	Timestamp int64  `json:"timestamp"`
}

// BulkStatus fetches remux:<id>:status for all ids in one MGET. Missing keys are ignored.
func (r *RemuxRepository) BulkStatus(ctx context.Context, ids []int64) (map[int64]*RemuxStatus, error) {
	out := make(map[int64]*RemuxStatus, len(ids))
	if len(ids) == 0 {
		return out, nil
	}

	keys := make([]string, 0, len(ids))
	for _, id := range ids {
		keys = append(keys, remuxStatusKey(id))
	}

	vals, err := r.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("mget status: %w", err)
	}

	for i, v := range vals {
		if v == nil {
			continue // key missing
		}
		var raw string
		switch t := v.(type) {
		case string:
			raw = t
		case []byte:
			raw = string(t)
		default:
			r.log.Warn("unexpected redis type for status", zap.Any("type", t))
			continue
		}
		var st RemuxStatus
		if err := json.Unmarshal([]byte(raw), &st); err != nil {
			r.log.Warn("bad status json", zap.String("key", keys[i]), zap.Error(err))
			continue
		}
		out[ids[i]] = &st
	}
	return out, nil
}

// LiveExtra bundles optional ifmt/metrics JSON blobs for live channels.
type LiveExtra struct {
	Ifmt    json.RawMessage
	Metrics json.RawMessage
}

// BulkIfmtMetrics fetches both ifmt and metrics for the given ids via a single MGET.
func (r *RemuxRepository) BulkIfmtMetrics(ctx context.Context, ids []int64) (map[int64]LiveExtra, error) {
	out := make(map[int64]LiveExtra, len(ids))
	if len(ids) == 0 {
		return out, nil
	}

	// Interleave keys: ifmt0, metrics0, ifmt1, metrics1, ...
	keys := make([]string, 0, len(ids)*2)
	for _, id := range ids {
		keys = append(keys, remuxIfmtKey(id), remuxMetricsKey(id))
	}

	vals, err := r.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("mget ifmt/metrics: %w", err)
	}

	// Walk results in steps of 2 (ifmt, metrics)
	for i, idx := 0, 0; i < len(vals); i, idx = i+2, idx+1 {
		var ifmtRaw, metricsRaw json.RawMessage

		if v := vals[i]; v != nil {
			ifmtRaw = asRawJSON(r.log, keys[i], v)
		}
		if v := vals[i+1]; v != nil {
			metricsRaw = asRawJSON(r.log, keys[i+1], v)
		}
		if ifmtRaw != nil || metricsRaw != nil {
			out[ids[idx]] = LiveExtra{Ifmt: ifmtRaw, Metrics: metricsRaw}
		}
	}
	return out, nil
}

func asRawJSON(log *zap.Logger, key string, v interface{}) json.RawMessage {
	switch t := v.(type) {
	case string:
		return json.RawMessage(t)
	case []byte:
		return json.RawMessage(t)
	default:
		log.Warn("unexpected redis type for json blob", zap.String("key", key), zap.Any("type", t))
		return nil
	}
}
