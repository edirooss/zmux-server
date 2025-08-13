package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	models "github.com/edirooss/zmux-server/pkg/models/channel"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

var ErrChannelNotFound = errors.New("channel not found")

const (
	channelKeyPrefix = "zmux:channel:"
	nextIDKey        = "zmux:channel:next_id"
)

// ChannelRepository handles Redis operations for channels
type ChannelRepository struct {
	client *Client
	logger *zap.Logger
}

// NewChannelRepository creates a new channel repository
func NewChannelRepository(log *zap.Logger) *ChannelRepository {
	return &ChannelRepository{
		client: NewClient("localhost:6379", 0, log),
		logger: log.Named("channel_repository"),
	}
}

func keyFor(id int64) string {
	return fmt.Sprintf("%s%d", channelKeyPrefix, id)
}

// GenerateID generates a new unique ID for a channel
func (r *ChannelRepository) GenerateID(ctx context.Context) (int64, error) {
	id, err := r.client.Incr(ctx, nextIDKey).Result()
	if err != nil {
		return 0, fmt.Errorf("incr: %w", err)
	}
	return id, nil
}

// Set saves a channel JSON blob to Redis
func (r *ChannelRepository) Set(ctx context.Context, channel *models.ZmuxChannel) error {
	key := keyFor(channel.ID)

	payload, err := json.Marshal(channel)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	// TTL 0 = persist forever;
	if err := r.client.Set(ctx, key, payload, 0).Err(); err != nil {
		return fmt.Errorf("set: %w", err)
	}
	return nil
}

// Get retrieves a channel by ID (returns redis.Nil if not found)
func (r *ChannelRepository) Get(ctx context.Context, id int64) (*models.ZmuxChannel, error) {
	key := keyFor(id)

	value, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrChannelNotFound
		}
		return nil, fmt.Errorf("get: %w", err)
	}

	var channel models.ZmuxChannel
	if err := json.Unmarshal(value, &channel); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	return &channel, nil
}

// Delete removes a single channel key. Returns ErrChannelNotFound if the key doesn't exist.
func (r *ChannelRepository) Delete(ctx context.Context, id int64) error {
	key := keyFor(id)
	n, err := r.client.Del(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("del: %w", err)
	}
	if n == 0 {
		return ErrChannelNotFound
	}
	return nil
}

// List retrieves all channels (no pagination).
// Uses SCAN to avoid blocking Redis, then MGET to fetch in one round-trip.
func (r *ChannelRepository) List(ctx context.Context) ([]*models.ZmuxChannel, error) {
	var keys []string

	iter := r.client.Scan(ctx, 0, channelKeyPrefix+"*", 1000).Iterator()
	for iter.Next(ctx) {
		k := iter.Val()
		// Skip the counter key just in case a broad pattern catches it
		if k == nextIDKey || strings.HasSuffix(k, "next_id") {
			continue
		}
		keys = append(keys, k)
	}
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}

	if len(keys) == 0 {
		// Consistent with "empty list" semantics
		return []*models.ZmuxChannel{}, nil
	}

	// Bulk fetch
	vals, err := r.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("mget: %w", err)
	}

	result := make([]*models.ZmuxChannel, 0, len(vals))
	for i, v := range vals {
		if v == nil {
			// Key disappeared between SCAN and MGET; skip
			continue
		}
		b, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("unexpected type for key %s at index %d", keys[i], i)
		}
		var ch models.ZmuxChannel
		if err := json.Unmarshal([]byte(b), &ch); err != nil {
			return nil, fmt.Errorf("unmarshal key %s: %w", keys[i], err)
		}
		result = append(result, &ch)
	}

	return result, nil
}
