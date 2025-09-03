package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/edirooss/zmux-server/internal/domain/principal"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

var (
	// ErrPrincipalNotFound indicates no principal is mapped to the token.
	ErrPrincipalNotFound = errors.New("principal not found")

	authBearerKeyPrefix = "zmux:auth:bearer:" // JSON value per token
)

// bearerKey builds the Redis key for a bearer token mapping.
func bearerKey(token string) string { return authBearerKeyPrefix + token }
func bearerScanPattern() string     { return authBearerKeyPrefix + "*" }

// PrincipalRepository provides Redis-backed CRUD for plaintext bearer tokens.
type PrincipalRepository struct {
	log    *zap.Logger
	client *RedisClient
}

// newPrincipalRepository constructs a new PrincipalRepository.
func newPrincipalRepository(log *zap.Logger, client *RedisClient) *PrincipalRepository {
	return &PrincipalRepository{
		log:    log.Named("principals"),
		client: client,
	}
}

// Upsert stores/updates the principal JSON at zmux:auth:bearer:<token>.
func (r *PrincipalRepository) Upsert(ctx context.Context, token string, p *principal.Principal) error {
	if p == nil || p.ID == "" {
		return fmt.Errorf("invalid principal")
	}
	payload, err := encodePrincipal(p)
	if err != nil {
		return fmt.Errorf("encode: %w", err)
	}
	key := bearerKey(token)
	if err := r.client.Set(ctx, key, payload, 0).Err(); err != nil {
		return fmt.Errorf("set %s: %w", key, err)
	}
	return nil
}

// GetByToken fetches and decodes the principal stored at zmux:auth:bearer:<token>.
func (r *PrincipalRepository) GetByToken(ctx context.Context, token string) (*principal.Principal, error) {
	key := bearerKey(token)
	raw, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrPrincipalNotFound
		}
		return nil, fmt.Errorf("get %s: %w", key, err)
	}
	pr, err := decodePrincipal(raw)
	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return pr, nil
}

// DeleteAll removes all bearer token mappings under zmux:auth:bearer:* (idempotent).
func (r *PrincipalRepository) DeleteAll(ctx context.Context) error {
	var cursor uint64
	pattern := bearerScanPattern()
	for {
		keys, next, err := r.client.Scan(ctx, cursor, pattern, 1024).Result()
		if err != nil {
			return fmt.Errorf("scan: %w", err)
		}
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

// encodePrincipal serializes a Principal to JSON.
func encodePrincipal(p *principal.Principal) ([]byte, error) {
	return json.Marshal(p)
}

// decodePrincipal deserializes JSON into a Principal.
func decodePrincipal(b []byte) (*principal.Principal, error) {
	var pr principal.Principal
	if err := json.Unmarshal(b, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}
