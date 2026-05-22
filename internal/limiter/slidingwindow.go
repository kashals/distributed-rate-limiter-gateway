package limiter

import (
	"context"
	"fmt"
	"time"

	"github.com/kashfmh/distributed-rate-limiter-gateway/internal/config"
	"github.com/redis/go-redis/v9"
)

// SlidingWindow implements RateLimiter via a Redis sorted set sliding window log.
type SlidingWindow struct {
	rdb    *redis.Client
	script *redis.Script
	cfg    *config.Config
}

// NewSlidingWindow constructs a SlidingWindow limiter.
// luaScript is the raw content of sliding_window.lua, injected by the caller.
func NewSlidingWindow(rdb *redis.Client, cfg *config.Config, luaScript string) *SlidingWindow {
	return &SlidingWindow{
		rdb:    rdb,
		script: redis.NewScript(luaScript),
		cfg:    cfg,
	}
}

// Allow runs the sliding window Lua script atomically against Redis.
func (sw *SlidingWindow) Allow(ctx context.Context, userID string) (bool, error) {
	key := fmt.Sprintf("rl:sw:%s", userID)
	now := time.Now().UnixMicro()

	// window in microseconds
	windowUs := sw.cfg.WindowSizeMs * 1000

	// ttl: window duration in seconds plus a one-period buffer
	ttl := (sw.cfg.WindowSizeMs / 1000) + (sw.cfg.WindowSizeMs / 1000)

	result, err := sw.script.Run(
		ctx, sw.rdb,
		[]string{key},
		now,
		windowUs,
		sw.cfg.RequestLimit,
		ttl,
	).Int()
	if err != nil {
		return false, fmt.Errorf("sliding window script: %w", err)
	}

	return result == 1, nil
}
