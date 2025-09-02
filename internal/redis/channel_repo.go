package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/edirooss/zmux-server/internal/domain/channel"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

var (
	ErrChannelNotFound = errors.New("channel not found")

	channelKeyPrefix = "zmux:channel:"
	nextIDKey        = "zmux:channel:next_id"
	channelIDsKey    = "zmux:channels" // SET of string IDs: {"1", "2", ...}
)

// ChannelRepository provides Redis-backed persistence for ZmuxChannel entities.
type ChannelRepository struct {
	client *Client
	log    *zap.Logger
}

// NewChannelRepository initializes a new ChannelRepository instance.
func NewChannelRepository(log *zap.Logger) *ChannelRepository {
	log = log.Named("channel_repo")

	return &ChannelRepository{
		log:    log,
		client: NewClient("localhost:6379", 0, log),
	}
}

// GenerateID increments and returns the next unique channel ID.
func (r *ChannelRepository) GenerateID(ctx context.Context) (int64, error) {
	id, err := r.client.Incr(ctx, nextIDKey).Result()
	if err != nil {
		return 0, fmt.Errorf("incr: %w", err)
	}
	return id, nil
}

// HasID returns true if a channel with the given ID exists.
func (r *ChannelRepository) HasID(ctx context.Context, id int64) (bool, error) {
	ok, err := r.client.SIsMember(ctx, channelIDsKey, strconv.FormatInt(id, 10)).Result()
	if err != nil {
		return false, fmt.Errorf("set is member: %w", err)
	}
	return ok, nil
}

// Upsert persists a ZmuxChannel and adds its ID to the Redis index set.
func (r *ChannelRepository) Upsert(ctx context.Context, ch *channel.ZmuxChannel) error {
	key := channelKey(ch.ID)

	payload, err := encodeChannel(ch)
	if err != nil {
		return fmt.Errorf("encode: %w", err)
	}

	pipe := r.client.TxPipeline()
	pipe.Set(ctx, key, payload, 0)
	pipe.SAdd(ctx, channelIDsKey, strconv.FormatInt(ch.ID, 10))

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("exec: %w", err)
	}
	return nil
}

// GetByID fetches a channel by its ID.
// Returns ErrChannelNotFound if the key does not exist.
func (r *ChannelRepository) GetByID(ctx context.Context, id int64) (*channel.ZmuxChannel, error) {
	key := channelKey(id)

	value, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrChannelNotFound
		}
		return nil, fmt.Errorf("get: %w", err)
	}

	ch, err := decodeChannel(value)
	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return ch, nil
}

// GetByIDs retrieves multiple channels by ID.
func (r *ChannelRepository) GetByIDs(ctx context.Context, ids []int64) ([]*channel.ZmuxChannel, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	keys := channelKeys(ids)
	vals, err := r.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("mget: %w", err)
	}

	return parseMGetValues(r.log, keys, vals)
}

// GetAll returns all known ZmuxChannels stored in Redis.
func (r *ChannelRepository) GetAll(ctx context.Context) ([]*channel.ZmuxChannel, error) {
	ids, err := r.client.SMembers(ctx, channelIDsKey).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("set members: %w", err)
	}
	if len(ids) == 0 {
		return nil, nil
	}

	keys := channelKeys(ids)
	vals, err := r.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("mget: %w", err)
	}

	return parseMGetValues(r.log, keys, vals)
}

// Delete removes a channel by ID. Returns ErrChannelNotFound if the key was not present.
func (r *ChannelRepository) Delete(ctx context.Context, id int64) error {
	key := channelKey(id)

	pipe := r.client.TxPipeline()
	del := pipe.Del(ctx, key)
	pipe.SRem(ctx, channelIDsKey, strconv.FormatInt(id, 10))

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("exec: %w", err)
	}
	if n := del.Val(); n == 0 {
		return ErrChannelNotFound
	}
	return nil
}

// channelKey constructs the Redis key for a channel ID.
func channelKey[T int64 | string](id T) string {
	switch v := any(id).(type) {
	case int64:
		return fmt.Sprintf("%s%d", channelKeyPrefix, v)
	case string:
		return fmt.Sprintf("%s%s", channelKeyPrefix, v)
	default:
		panic("unsupported type") // programming fault
	}
}

// channelKeys constructs the Redis keys for multiple channel IDs.
func channelKeys[T int64 | string](ids []T) []string {
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = channelKey(id)
	}
	return keys
}

// encodeChannel serializes a ZmuxChannel to JSON.
func encodeChannel(ch *channel.ZmuxChannel) ([]byte, error) {
	return json.Marshal(ch)
}

// decodeChannel deserializes a JSON payload into a ZmuxChannel.
func decodeChannel(raw []byte) (*channel.ZmuxChannel, error) {
	var ch channel.ZmuxChannel
	if err := json.Unmarshal(raw, &ch); err != nil {
		return nil, err
	}
	return &ch, nil
}

// parseMGetValues converts Redis MGET results to ZmuxChannel structs.
func parseMGetValues(_ *zap.Logger, keys []string, vals []interface{}) ([]*channel.ZmuxChannel, error) {
	out := make([]*channel.ZmuxChannel, 0, len(vals))

	for i, v := range vals {
		if v == nil {
			return nil, fmt.Errorf("key %s at index %d: %w", keys[i], i, ErrChannelNotFound)
		}

		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("key %s at index %d: unexpected type (got %T, want string)", keys[i], i, v)
		}
		ch, err := decodeChannel([]byte(s))
		if err != nil {
			return nil, fmt.Errorf("key %s at index %d: decode channel: %w", keys[i], i, err)
		}
		out = append(out, ch)
	}
	return out, nil
}
