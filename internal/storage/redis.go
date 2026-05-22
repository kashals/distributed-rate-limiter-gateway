package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/kashfmh/distributed-rate-limiter-gateway/internal/config"
	"github.com/redis/go-redis/v9"
)

// NewRedisClient initializes a connection pool and validates reachability.
func NewRedisClient(cfg *config.Config) (*redis.Client, error) {
	opts := &redis.Options{
		Addr:         cfg.RedisAddr,
		Password:     cfg.RedisPassword,
		DB:           cfg.RedisDB,
		PoolSize:     cfg.RedisPoolSize,
		MinIdleConns: cfg.RedisMinIdle,
		DialTimeout:  cfg.RedisDialTimeout,
		ReadTimeout:  cfg.RedisReadTimeout,
		WriteTimeout: cfg.RedisWriteTimeout,
	}

	client := redis.NewClient(opts)

	// validate connectivity at startup; fail fast
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}

	return client, nil
}
