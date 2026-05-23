package gateway

import (
	"context"
	"crypto/rsa"
	"net/http"
	"os"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/kashals/distributed-rate-limiter-gateway/internal/config"
	"github.com/kashals/distributed-rate-limiter-gateway/internal/limiter"
)

// ctxKey is a private type to prevent context key collisions.
type ctxKey string

const ctxUserID ctxKey = "user_id"

// JWTMiddleware verifies the Bearer token and injects the verified user_id into context.
// Supports HS256 and RS256; drops unauthenticated traffic with 401.
func JWTMiddleware(cfg *config.Config, next http.Handler) http.Handler {
	// parse RS256 public key once at init; panic early on bad config
	var rsaPub *rsa.PublicKey
	if cfg.JWTAlgorithm == "RS256" {
		pemBytes, err := os.ReadFile(cfg.JWTPublicKeyPath)
		if err != nil {
			panic("read jwt public key: " + err.Error())
		}
		rsaPub, err = jwt.ParseRSAPublicKeyFromPEM(pemBytes)
		if err != nil {
			panic("parse jwt public key: " + err.Error())
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// extract bearer token
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

		// verify jwt signature
		var claims jwt.MapClaims
		token, err := jwt.ParseWithClaims(tokenStr, &claims, func(t *jwt.Token) (interface{}, error) {
			switch cfg.JWTAlgorithm {
			case "HS256":
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, jwt.ErrSignatureInvalid
				}
				return []byte(cfg.JWTSecret), nil
			case "RS256":
				if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
					return nil, jwt.ErrSignatureInvalid
				}
				return rsaPub, nil
			default:
				return nil, jwt.ErrSignatureInvalid
			}
		})
		if err != nil || !token.Valid {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// extract subject claim as user identifier
		userID, ok := claims["sub"].(string)
		if !ok || userID == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// inject user_id downstream
		ctx := context.WithValue(r.Context(), ctxUserID, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RateLimitMiddleware gates requests through the configured RateLimiter.
// Returns 429 with a Retry-After header when the limit is exceeded.
func RateLimitMiddleware(rl limiter.RateLimiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// user_id must already be present; JWTMiddleware runs first
		userID, ok := r.Context().Value(ctxUserID).(string)
		if !ok || userID == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		allowed, err := rl.Allow(r.Context(), userID)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		if !allowed {
			w.Header().Set("Retry-After", "1")
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// UserIDFromContext retrieves the authenticated user_id from context.
// Used by the proxy layer if it needs to forward the identity header.
func UserIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(ctxUserID).(string)
	return id, ok
}
