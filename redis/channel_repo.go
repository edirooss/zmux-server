package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/edirooss/zmux-server/internal/domain/channel"
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
	log    *zap.Logger
}

// NewChannelRepository creates a new channel repository
func NewChannelRepository(log *zap.Logger) *ChannelRepository {
	log = log.Named("channel_repo")
	return &ChannelRepository{
		client: NewClient("localhost:6379", 0, log),
		log:    log,
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

const channelIDSetKey = "zmux:channels" // SET of string ids: {"1","2",...}

func (r *ChannelRepository) Set(ctx context.Context, channel *channel.ZmuxChannel) error {
	key := keyFor(channel.ID)

	payload, err := json.Marshal(channel)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	pipe := r.client.TxPipeline()
	pipe.Set(ctx, key, payload, 0)
	pipe.SAdd(ctx, channelIDSetKey, strconv.FormatInt(channel.ID, 10))
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("set+sadd: %w", err)
	}
	return nil
}

// Get retrieves a channel by ID (returns redis.Nil if not found)
func (r *ChannelRepository) Get(ctx context.Context, id int64) (*channel.ZmuxChannel, error) {
	key := keyFor(id)

	value, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrChannelNotFound
		}
		return nil, fmt.Errorf("get: %w", err)
	}

	var channel channel.ZmuxChannel
	if err := json.Unmarshal(value, &channel); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	return &channel, nil
}

// Delete removes a single channel key. Returns ErrChannelNotFound if the key doesn't exist.
func (r *ChannelRepository) Delete(ctx context.Context, id int64) error {
	key := keyFor(id)
	pipe := r.client.TxPipeline()
	del := pipe.Del(ctx, key)
	pipe.SRem(ctx, channelIDSetKey, strconv.FormatInt(id, 10))
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("del+srem: %w", err)
	}
	if n := del.Val(); n == 0 {
		return ErrChannelNotFound
	}
	return nil
}

// List retrieves all channels by using the maintained SET of IDs.
func (r *ChannelRepository) List(ctx context.Context) ([]*channel.ZmuxChannel, error) {
	ids, err := r.client.SMembers(ctx, channelIDSetKey).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("smembers: %w", err)
	}
	if len(ids) == 0 {
		return nil, nil
	}

	keys := make([]string, 0, len(ids))
	for _, idStr := range ids {
		// guard against accidental junk in the set
		if strings.TrimSpace(idStr) == "" {
			continue
		}
		keys = append(keys, fmt.Sprintf("%s%s", channelKeyPrefix, idStr))
	}

	vals, err := r.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("mget: %w", err)
	}

	result := make([]*channel.ZmuxChannel, 0, len(vals))
	for i, v := range vals {
		if v == nil {
			continue // key missing (possible if set drifted); harmless
		}
		var b []byte
		switch t := v.(type) {
		case string:
			b = []byte(t)
		case []byte:
			b = t
		default:
			return nil, fmt.Errorf("unexpected type for key %s at index %d", keys[i], i)
		}
		var ch channel.ZmuxChannel
		if err := json.Unmarshal(b, &ch); err != nil {
			return nil, fmt.Errorf("unmarshal key %s: %w", keys[i], err)
		}
		result = append(result, &ch)
	}
	return result, nil
}
