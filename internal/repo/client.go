package repo

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// RedisClient wraps the Redis client with additional functionality
type RedisClient struct {
	*redis.Client
	log *zap.Logger
}

// newRedisClient creates a new Redis client with configuration
func newRedisClient(addr string, db int, log *zap.Logger) *RedisClient {
	opts := &redis.Options{
		Addr:         addr,
		DB:           db,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     10,
		MinIdleConns: 5,
		MaxRetries:   3,
	}

	client := &RedisClient{
		Client: redis.NewClient(opts),
		log:    log.Named("Redis"),
	}

	client.Ping(context.TODO())

	return client
}

// Close closes the Redis client connection
func (c *RedisClient) Close() error {
	return c.Client.Close()
}

// Ping uses opTimeout and logs connection diagnostics.
func (c *RedisClient) Ping(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()

	opts := c.Options()
	log := c.log.With(
		zap.String("addr", opts.Addr),
		zap.Int("db", opts.DB),
		zap.Int("max_retries", opts.MaxRetries),
	)

	start := time.Now()
	err := c.Client.Ping(ctx).Err()
	elapsed := time.Since(start)

	if err != nil {
		log.Warn("connection failed", zap.Error(err), zap.Duration("ping_rtt", elapsed))
	} else {
		log.Info("connection established", zap.Duration("ping_rtt", elapsed))
	}
}
