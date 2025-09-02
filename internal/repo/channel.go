package repo

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

func channelKeyInt(id int64) string  { return channelKeyPrefix + strconv.FormatInt(id, 10) }
func channelKeyStr(id string) string { return channelKeyPrefix + id }

// ChannelRepository provides Redis-backed persistence for ZmuxChannel entities.
type ChannelRepository struct {
	client *RedisClient
	log    *zap.Logger
}

// newChannelRepository initializes a new ChannelRepository instance.
func newChannelRepository(log *zap.Logger, client *RedisClient) *ChannelRepository {
	log = log.Named("channels")

	return &ChannelRepository{
		log:    log,
		client: client,
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

// Upsert persists a ZmuxChannel and adds its ID to the Redis index set.
func (r *ChannelRepository) Upsert(ctx context.Context, ch *channel.ZmuxChannel) error {
	key := channelKeyInt(ch.ID)

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

// Delete removes a channel by ID.
// Returns ErrChannelNotFound if the channel key was not present in Redis.
// Logs a warning if the channel record and index set are inconsistent.
func (r *ChannelRepository) Delete(ctx context.Context, id int64) error {
	key := channelKeyInt(id)
	idStr := strconv.FormatInt(id, 10)

	pipe := r.client.TxPipeline()
	delRes := pipe.Del(ctx, key)
	sremRes := pipe.SRem(ctx, channelIDsKey, idStr)

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("exec: %w", err)
	}

	delCount := delRes.Val()
	sremCount := sremRes.Val()

	// If both returned 0, nothing existed
	if delCount == 0 && sremCount == 0 {
		return ErrChannelNotFound
	}

	// If they differ, log it — data/index mismatch
	if delCount != sremCount {
		r.log.Warn(
			"channel delete mismatch",
			zap.String("key", key),
			zap.String("id", idStr),
			zap.Int64("del_count", delCount),
			zap.Int64("srem_count", sremCount),
		)
	}

	return nil
}

// HasID returns true if a channel with the given ID exists.
func (r *ChannelRepository) HasID(ctx context.Context, id int64) (bool, error) {
	ok, err := r.client.SIsMember(ctx, channelIDsKey, strconv.FormatInt(id, 10)).Result()
	if err != nil {
		return false, fmt.Errorf("ismember: %w", err)
	}
	return ok, nil
}

// GetByID fetches a channel by its ID.
// Returns ErrChannelNotFound if the key does not exist.
func (r *ChannelRepository) GetByID(ctx context.Context, id int64) (*channel.ZmuxChannel, error) {
	key := channelKeyInt(id)

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
		return []*channel.ZmuxChannel{}, nil
	}

	keys := channelKeysInt(ids)
	vals, err := r.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("mget: %w", err)
	}

	return r.parseMGetResult(keys, vals)
}

// GetAll returns all ZmuxChannels currently indexed in Redis.
//
// Note: This operation is **not strongly consistent**. It issues two separate calls:
//  1. SMEMBERS to collect the set of channel IDs.
//  2. MGET to fetch the channel payloads.
//
// If channels are created or deleted between those two calls, the result may
// contain transient inconsistencies (e.g. an ID with no value, or a value not
// yet indexed). Callers should treat the result as **an eventually consistent**
// snapshot, not a transactional view.
//
// If we require atomic semantics (point-in-time snapshot), we must implement
// this as a Lua script or handle versioning at the application layer.
func (r *ChannelRepository) GetAll(ctx context.Context) ([]*channel.ZmuxChannel, error) {
	ids, err := r.client.SMembers(ctx, channelIDsKey).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("smembers: %w", err)
	}
	if len(ids) == 0 {
		return nil, nil
	}

	keys := channelKeysStr(ids)
	vals, err := r.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("mget: %w", err)
	}

	return r.parseMGetResult(keys, vals)
}

// channelKeysInt builds Redis keys for multiple int64 channel IDs.
func channelKeysInt(ids []int64) []string {
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = channelKeyInt(id)
	}
	return keys
}

// channelKeysStr builds Redis keys for multiple string channel IDs.
func channelKeysStr(ids []string) []string {
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = channelKeyStr(id)
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

// parseMGetResult converts Redis MGET results to ZmuxChannel structs.
// It logs warnings for missing keys and errors for unexpected payload types.
// Callers should treat missing keys as eventual-consistency artifacts, not hard failures.
func (r *ChannelRepository) parseMGetResult(keys []string, vals []interface{}) ([]*channel.ZmuxChannel, error) {
	out := make([]*channel.ZmuxChannel, 0, len(vals))

	for i, v := range vals {
		if v == nil {
			r.log.Warn(
				"channel missing during MGET",
				zap.String("key", keys[i]),
				zap.Int("index", i),
			)
			continue
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
