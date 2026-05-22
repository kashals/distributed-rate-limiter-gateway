package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kashfmh/distributed-rate-limiter-gateway/internal/config"
	"github.com/kashfmh/distributed-rate-limiter-gateway/internal/gateway"
	"github.com/kashfmh/distributed-rate-limiter-gateway/internal/limiter"
	"github.com/kashfmh/distributed-rate-limiter-gateway/internal/storage"
)

func main() {
	// load and validate all env config
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	// init redis connection pool; fail fast on unreachable host
	rdb, err := storage.NewRedisClient(cfg)
	if err != nil {
		log.Fatalf("redis: %v", err)
	}
	defer rdb.Close()

	// load lua scripts from disk; injected into limiters to avoid embed path constraints
	tbScript, err := os.ReadFile("scripts/token_bucket.lua")
	if err != nil {
		log.Fatalf("read token_bucket.lua: %v", err)
	}
	swScript, err := os.ReadFile("scripts/sliding_window.lua")
	if err != nil {
		log.Fatalf("read sliding_window.lua: %v", err)
	}

	// select rate limiting strategy from config
	var rl limiter.RateLimiter
	switch cfg.RateLimitAlgo {
	case config.AlgoTokenBucket:
		rl = limiter.NewTokenBucket(rdb, cfg, string(tbScript))
	case config.AlgoSlidingWindow:
		rl = limiter.NewSlidingWindow(rdb, cfg, string(swScript))
	}

	// init reverse proxy
	proxy, err := gateway.NewReverseProxy(cfg.BackendURL)
	if err != nil {
		log.Fatalf("proxy: %v", err)
	}

	// middleware chain: JWT auth -> rate limit -> proxy
	handler := gateway.JWTMiddleware(cfg,
		gateway.RateLimitMiddleware(rl, proxy),
	)

	srv := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// start server in background goroutine
	go func() {
		slog.Info("gateway listening",
			"addr", cfg.ListenAddr,
			"algo", cfg.RateLimitAlgo,
			"backend", cfg.BackendURL,
		)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	// block until SIGINT or SIGTERM
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutdown signal received")

	// graceful drain with 15s deadline
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("server shutdown: %v", err)
	}

	slog.Info("gateway stopped")
}
