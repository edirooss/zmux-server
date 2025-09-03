package repo

import (
	"context"
	"fmt"
	"strconv"

	"go.uber.org/zap"
)

// b2bClntChnlsKey builds the Redis key for a client's channel set.
func b2bClntChnlsKey(clientID string) string { return "zmux:b2b_client:" + clientID + ":channels" }

// b2bClntChnlsScanPattern returns the SCAN pattern for all client channel sets.
func b2bClntChnlsScanPattern() string { return "zmux:b2b_client:*:channels" }

// B2BClntChnlsRepo persists client→channel bindings in a Redis SET per client.
type B2BClntChnlsRepo struct {
	log    *zap.Logger
	client *RedisClient
}

// newB2BClntChnlsRepo initializes a repository backed by Redis.
func newB2BClntChnlsRepo(log *zap.Logger, client *RedisClient) *B2BClntChnlsRepo {
	return &B2BClntChnlsRepo{
		log:    log.Named("b2b_client_channels"),
		client: client,
	}
}

// BindChannelID adds a single channel ID to the client's bound set.
func (r *B2BClntChnlsRepo) BindChannelID(ctx context.Context, clientID string, channelID int64) error {
	key := b2bClntChnlsKey(clientID)
	if err := r.client.SAdd(ctx, key, strconv.FormatInt(channelID, 10)).Err(); err != nil {
		return fmt.Errorf("sadd %s: %w", key, err)
	}
	return nil
}

// BindChannelIDs adds multiple channel IDs to the client's bound set in one call.
func (r *B2BClntChnlsRepo) BindChannelIDs(ctx context.Context, clientID string, channelIDs []int64) error {
	if len(channelIDs) == 0 {
		return nil
	}
	key := b2bClntChnlsKey(clientID)
	members := make([]interface{}, 0, len(channelIDs))
	for _, id := range channelIDs {
		members = append(members, strconv.FormatInt(id, 10))
	}
	if err := r.client.SAdd(ctx, key, members...).Err(); err != nil {
		return fmt.Errorf("sadd %s: %w", key, err)
	}
	return nil
}

// HasChannelID reports whether the given channel ID is bound to the client.
func (r *B2BClntChnlsRepo) HasChannelID(ctx context.Context, clientID string, channelID int64) (bool, error) {
	key := b2bClntChnlsKey(clientID)
	ok, err := r.client.SIsMember(ctx, key, strconv.FormatInt(channelID, 10)).Result()
	if err != nil {
		return false, fmt.Errorf("sismember %s: %w", key, err)
	}
	return ok, nil
}

// GetAll returns all channel IDs currently bound to the client.
func (r *B2BClntChnlsRepo) GetAll(ctx context.Context, clientID string) ([]int64, error) {
	key := b2bClntChnlsKey(clientID)
	raw, err := r.client.SMembers(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("smembers %s: %w", key, err)
	}
	out := make([]int64, 0, len(raw))
	for _, s := range raw {
		id, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			// Skip malformed member; log and continue.
			r.log.Warn("malformed channel id in set", zap.String("key", key), zap.String("member", s))
			continue
		}
		out = append(out, id)
	}
	return out, nil
}

// GetAllMap returns all channel IDs bound to the client as a set for O(1) lookups.
// Malformed members are skipped with a warning.
func (r *B2BClntChnlsRepo) GetAllMap(ctx context.Context, clientID string) (map[int64]struct{}, error) {
	key := b2bClntChnlsKey(clientID)
	raw, err := r.client.SMembers(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("smembers %s: %w", key, err)
	}
	out := make(map[int64]struct{}, len(raw))
	for _, s := range raw {
		id, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			r.log.Warn("malformed channel id in set", zap.String("key", key), zap.String("member", s))
			continue
		}
		out[id] = struct{}{}
	}
	return out, nil
}

// DeleteAllClients deletes every client's channel set (zmux:b2b_client:*:channels).
func (r *B2BClntChnlsRepo) DeleteAllClients(ctx context.Context) error {
	var cursor uint64
	pattern := b2bClntChnlsScanPattern()

	for {
		keys, next, err := r.client.Scan(ctx, cursor, pattern, 512).Result()
		if err != nil {
			return fmt.Errorf("scan %q: %w", pattern, err)
		}
		// DEV: batch DEL is fine at this scale; no need for pipelines for a few dozen keys.
		if len(keys) > 0 {
			if err := r.client.Del(ctx, keys...).Err(); err != nil {
				return fmt.Errorf("del: %w", err)
			}
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return nil
}
