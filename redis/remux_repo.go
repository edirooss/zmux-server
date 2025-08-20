package redis

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/edirooss/zmux-server/pkg/models/channelmodel"
	"go.uber.org/zap"
)

func remuxStatusKey(id int64) string  { return fmt.Sprintf("remux:%d:status", id) }
func remuxIfmtKey(id int64) string    { return fmt.Sprintf("remux:%d:ifmt", id) }
func remuxMetricsKey(id int64) string { return fmt.Sprintf("remux:%d:metrics", id) }

// RemuxRepository deals with monitoring keys remux:<id>:*
type RemuxRepository struct {
	client *Client
	log    *zap.Logger
}

func NewRemuxRepository(log *zap.Logger) *RemuxRepository {
	return &RemuxRepository{
		client: NewClient("localhost:6379", 0, log),
		log:    log.Named("remux_repository"),
	}
}

// BulkStatus fetches remux:<id>:status for all ids in one MGET. Missing keys are ignored.
func (r *RemuxRepository) BulkStatus(ctx context.Context, ids []int64) (map[int64]*channelmodel.RemuxStatus, error) {
	out := make(map[int64]*channelmodel.RemuxStatus, len(ids))
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
		var st channelmodel.RemuxStatus
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
