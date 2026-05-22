package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// RateLimitAlgo selects the throttling strategy.
type RateLimitAlgo string

const (
	AlgoTokenBucket   RateLimitAlgo = "token_bucket"
	AlgoSlidingWindow RateLimitAlgo = "sliding_window"
)

// Config holds all runtime configuration resolved from environment.
type Config struct {
	// server
	ListenAddr string

	// upstream backend
	BackendURL string

	// redis
	RedisAddr         string
	RedisPassword     string
	RedisDB           int
	RedisPoolSize     int
	RedisMinIdle      int
	RedisDialTimeout  time.Duration
	RedisReadTimeout  time.Duration
	RedisWriteTimeout time.Duration

	// jwt
	// Algorithm: "HS256" | "RS256"
	JWTAlgorithm     string
	JWTSecret        string // HS256 shared secret
	JWTPublicKeyPath string // RS256 PEM public key path

	// rate limiting
	RateLimitAlgo RateLimitAlgo

	// token bucket params
	MaxTokens  int
	RefillRate float64 // tokens per second

	// sliding window params
	WindowSizeMs int64 // window duration in milliseconds
	RequestLimit int   // max requests per window
}

// Load resolves all config values from environment variables.
// Returns an error if any required variable is absent or malformed.
func Load() (*Config, error) {
	cfg := &Config{}
	var err error

	// server
	cfg.ListenAddr = envStr("LISTEN_ADDR", ":8080")
	if cfg.BackendURL, err = requireStr("BACKEND_URL"); err != nil {
		return nil, err
	}

	// redis
	cfg.RedisAddr = envStr("REDIS_ADDR", "localhost:6379")
	cfg.RedisPassword = envStr("REDIS_PASSWORD", "")
	if cfg.RedisDB, err = envInt("REDIS_DB", 0); err != nil {
		return nil, err
	}
	if cfg.RedisPoolSize, err = envInt("REDIS_POOL_SIZE", 10); err != nil {
		return nil, err
	}
	if cfg.RedisMinIdle, err = envInt("REDIS_MIN_IDLE", 2); err != nil {
		return nil, err
	}
	if cfg.RedisDialTimeout, err = envDuration("REDIS_DIAL_TIMEOUT_MS", 500); err != nil {
		return nil, err
	}
	if cfg.RedisReadTimeout, err = envDuration("REDIS_READ_TIMEOUT_MS", 300); err != nil {
		return nil, err
	}
	if cfg.RedisWriteTimeout, err = envDuration("REDIS_WRITE_TIMEOUT_MS", 300); err != nil {
		return nil, err
	}

	// jwt
	cfg.JWTAlgorithm = envStr("JWT_ALGORITHM", "HS256")
	cfg.JWTSecret = envStr("JWT_SECRET", "")
	cfg.JWTPublicKeyPath = envStr("JWT_PUBLIC_KEY_PATH", "")

	// validate jwt config
	switch cfg.JWTAlgorithm {
	case "HS256":
		if cfg.JWTSecret == "" {
			return nil, fmt.Errorf("JWT_SECRET required for HS256")
		}
	case "RS256":
		if cfg.JWTPublicKeyPath == "" {
			return nil, fmt.Errorf("JWT_PUBLIC_KEY_PATH required for RS256")
		}
	default:
		return nil, fmt.Errorf("unsupported JWT_ALGORITHM: %s", cfg.JWTAlgorithm)
	}

	// rate limiting
	algo := envStr("RATE_LIMIT_ALGO", string(AlgoTokenBucket))
	switch RateLimitAlgo(algo) {
	case AlgoTokenBucket, AlgoSlidingWindow:
		cfg.RateLimitAlgo = RateLimitAlgo(algo)
	default:
		return nil, fmt.Errorf("unsupported RATE_LIMIT_ALGO: %s", algo)
	}

	// token bucket
	if cfg.MaxTokens, err = envInt("MAX_TOKENS", 100); err != nil {
		return nil, err
	}
	if cfg.RefillRate, err = envFloat("REFILL_RATE", 10.0); err != nil {
		return nil, err
	}

	// sliding window
	if cfg.WindowSizeMs, err = envInt64("WINDOW_SIZE_MS", 60000); err != nil {
		return nil, err
	}
	if cfg.RequestLimit, err = envInt("REQUEST_LIMIT", 100); err != nil {
		return nil, err
	}

	return cfg, nil
}

// -- helpers --

func requireStr(key string) (string, error) {
	v := os.Getenv(key)
	if v == "" {
		return "", fmt.Errorf("required env var %s is not set", key)
	}
	return v, nil
}

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("invalid int for %s: %w", key, err)
	}
	return n, nil
}

func envInt64(key string, def int64) (int64, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid int64 for %s: %w", key, err)
	}
	return n, nil
}

func envFloat(key string, def float64) (float64, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid float for %s: %w", key, err)
	}
	return f, nil
}

// envDuration reads a millisecond integer env var and returns a time.Duration.
func envDuration(key string, defMs int64) (time.Duration, error) {
	ms, err := envInt64(key, defMs)
	if err != nil {
		return 0, err
	}
	return time.Duration(ms) * time.Millisecond, nil
}
