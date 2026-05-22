package limiter

import "context"

// RateLimiter is the common interface for all throttling strategies.
type RateLimiter interface {
	// Allow returns true if the request is within the rate limit.
	// userID is the authenticated consumer identifier extracted from the JWT.
	Allow(ctx context.Context, userID string) (bool, error)
}
