package limiter

import (
	"context"
	"fmt"
	"time"

	"github.com/kashfmh/distributed-rate-limiter-gateway/internal/config"
	"github.com/redis/go-redis/v9"
)

// TokenBucket implements RateLimiter via a lazy-refill token bucket in Redis.
type TokenBucket struct {
	rdb    *redis.Client
	script *redis.Script
	cfg    *config.Config
}

// NewTokenBucket constructs a TokenBucket.
// luaScript is the raw content of token_bucket.lua, injected by the caller.
func NewTokenBucket(rdb *redis.Client, cfg *config.Config, luaScript string) *TokenBucket {
	return &TokenBucket{
		rdb:    rdb,
		script: redis.NewScript(luaScript),
		cfg:    cfg,
	}
}

// Allow runs the token bucket Lua script atomically against Redis.
func (tb *TokenBucket) Allow(ctx context.Context, userID string) (bool, error) {
	key := fmt.Sprintf("rl:tb:%s", userID)
	now := time.Now().UnixMilli()

	// ttl: time to refill an empty bucket from zero, plus a safety buffer
	ttl := int64(float64(tb.cfg.MaxTokens)/tb.cfg.RefillRate) + 60

	result, err := tb.script.Run(
		ctx, tb.rdb,
		[]string{key},
		now,
		tb.cfg.MaxTokens,
		tb.cfg.RefillRate,
		ttl,
	).Int()
	if err != nil {
		return false, fmt.Errorf("token bucket script: %w", err)
	}

	return result == 1, nil
}
