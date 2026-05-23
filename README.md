<div align="center">

<br/>

# 🚦 Distributed Rate Limiter Gateway

**production-grade API gateway with global rate limiting across horizontally scaled services.**

*zero-framework Go. atomic Lua scripts. Redis-backed distributed state.*

<br/>

[![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go)](https://go.dev)
[![Redis](https://img.shields.io/badge/Redis-7+-DC382D?logo=redis)](https://redis.io)
[![JWT](https://img.shields.io/badge/JWT-HS256%20%7C%20RS256-000000?logo=jsonwebtokens)](https://jwt.io)
[![License](https://img.shields.io/badge/License-MIT-green)](LICENSE)

</div>

---

## about

lightweight API gateway in pure Go that enforces **per-user rate limits globally** across any number of horizontally scaled replicas. all throttling state lives in Redis, mutated through atomic Lua scripts so there's no race conditions, no double-counting, no inter-process coordination needed.

two pluggable algorithms ship out of the box: **Token Bucket** for smooth burst absorption and **Sliding Window Log** for hard per-window caps. the gateway sits in front of any HTTP backend, validates JWT identity on every request, and only forwards what passes both auth and rate checks. the raw `Authorization` header gets stripped, replaced with a clean `X-User-ID` for the backend.

---

## Features

### JWT Auth
- validates `Bearer` tokens on every inbound request, drops unauthenticated traffic with `401`
- supports **HS256** (shared secret) and **RS256** (RSA public key), configurable via env var
- RSA public key is parsed once at startup so subsequent requests pay zero I/O cost
- verified `sub` claim gets injected into request context as the user identity downstream

### Rate Limiting Engine
- **pluggable strategy** — swap between Token Bucket and Sliding Window with a single env var, no code changes
- **atomic Lua execution** — both algorithms run as single Redis scripts; the entire check-and-mutate cycle is atomic
- **per-user isolation** — each `user_id` gets its own Redis key so one noisy user can't starve others
- returns `429` with a `Retry-After` header when the limit is exceeded

#### Token Bucket (`RATE_LIMIT_ALGO=token_bucket`)
- lazy refill on each request: tokens accrued since last access are computed and clamped to `MAX_TOKENS`
- state stored as an `HSET` with tokens + last_updated timestamp in milliseconds
- TTL is auto-set to refill-time + 60s buffer; idle keys expire on their own

#### Sliding Window Log (`RATE_LIMIT_ALGO=sliding_window`)
- sorted set of microsecond timestamps, stale entries outside the window get evicted on every call
- exact count of active requests within the rolling window, no approximation
- TTL is set to `2 × window_duration` so keys never outlive their data

### Reverse Proxy
- `httputil.ReverseProxy` with a custom Director pipeline
- strips the inbound `Authorization` header before forwarding
- injects `X-User-ID` (verified identity), `X-Real-IP`, and `X-Forwarded-For`
- tuned `http.Transport`: 200 max idle conns, 50 per host, 90s idle timeout
- returns `502` on upstream dial failure; handles client disconnects cleanly

### Graceful Shutdown
- listens for `SIGINT` / `SIGTERM` and drains in-flight requests with a 15-second deadline
- server timeouts: 10s read, 30s write, 120s idle

### Dev Utilities
- `cmd/echoserver` — minimal HTTP echo server on `:9000` that prints method, path, and all received headers. useful for verifying the header mutation pipeline
- `cmd/gentoken` — generates a signed HS256 JWT (`sub: user-123`, 24h expiry) for local testing

---

## Tech Stack

| layer | tool |
|---|---|
| language | [Go 1.24](https://go.dev) |
| JWT | [golang-jwt/jwt v5](https://github.com/golang-jwt/jwt) |
| Redis client | [redis/go-redis v9](https://github.com/redis/go-redis) |
| rate limit state | Redis (HSET + ZSET via atomic Lua) |
| reverse proxy | `net/http/httputil` (stdlib) |
| configuration | environment variables, no config files |
| structured logging | `log/slog` (stdlib, Go 1.21+) |

---

## Prerequisites

- Go 1.24+
- a running Redis instance (local or remote)
- `JWT_SECRET` for HS256 **or** an RSA public key PEM file for RS256

---

## Installation

```bash
git clone https://github.com/kashals/distributed-rate-limiter-gateway.git
cd distributed-rate-limiter-gateway
go mod download
```

---

## Configuration

everything is resolved from env vars at startup. the gateway fails fast with a clear error if anything required is missing or malformed.

### Required

| variable | description |
|---|---|
| `BACKEND_URL` | full URL of the upstream backend (e.g. `http://localhost:9000`) |
| `JWT_SECRET` | shared secret for HS256 signing *(required if `JWT_ALGORITHM=HS256`)* |
| `JWT_PUBLIC_KEY_PATH` | path to RSA public key PEM file *(required if `JWT_ALGORITHM=RS256`)* |

### Optional (with defaults)

| variable | default | description |
|---|---|---|
| `LISTEN_ADDR` | `:8080` | gateway listen address |
| `JWT_ALGORITHM` | `HS256` | `HS256` or `RS256` |
| `RATE_LIMIT_ALGO` | `token_bucket` | `token_bucket` or `sliding_window` |
| `REDIS_ADDR` | `localhost:6379` | Redis address |
| `REDIS_PASSWORD` | *(empty)* | Redis auth password |
| `REDIS_DB` | `0` | Redis logical database index |
| `REDIS_POOL_SIZE` | `10` | max connections in the pool |
| `REDIS_MIN_IDLE` | `2` | minimum idle connections to keep warm |
| `REDIS_DIAL_TIMEOUT_MS` | `500` | dial timeout in ms |
| `REDIS_READ_TIMEOUT_MS` | `300` | read timeout in ms |
| `REDIS_WRITE_TIMEOUT_MS` | `300` | write timeout in ms |
| `MAX_TOKENS` | `100` | token bucket max capacity |
| `REFILL_RATE` | `10.0` | token bucket refill rate (tokens/sec) |
| `WINDOW_SIZE_MS` | `60000` | sliding window duration in ms |
| `REQUEST_LIMIT` | `100` | sliding window max requests per window |

---

## Running Locally

**1. start Redis**
```bash
redis-server
```

**2. start the echo backend** (optional, for testing)
```bash
go run ./cmd/echoserver
# listening on :9000
```

**3. generate a test JWT**
```bash
JWT_SECRET=my-secret go run ./cmd/gentoken
# prints a signed token to stdout
```

**4. start the gateway**
```bash
BACKEND_URL=http://localhost:9000 \
JWT_SECRET=my-secret \
go run ./cmd/gateway
# gateway listening on :8080
```

**5. hit the gateway**
```bash
TOKEN=$(JWT_SECRET=my-secret go run ./cmd/gentoken)

curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/hello
```

the echo server will reflect back the method, path, and all forwarded headers, including the injected `X-User-ID` and `X-Real-IP`.

---

## Switching Algorithms

**Token Bucket** (default, smooth burst absorption):
```bash
RATE_LIMIT_ALGO=token_bucket MAX_TOKENS=50 REFILL_RATE=5 ...
```

**Sliding Window Log** (hard per-window cap):
```bash
RATE_LIMIT_ALGO=sliding_window WINDOW_SIZE_MS=60000 REQUEST_LIMIT=30 ...
```

---

## Request Flow

```
Client
  │
  │  POST /api/resource
  │  Authorization: Bearer <jwt>
  ▼
┌─────────────────────────────┐
│        Gateway :8080        │
│                             │
│  1. JWTMiddleware           │
│     · validate Bearer token │
│     · extract sub -> user_id│
│     · 401 if invalid        │
│                             │
│  2. RateLimitMiddleware     │
│     · run Lua script in     │
│       Redis (atomic)        │
│     · 429 if over limit     │
│                             │
│  3. ReverseProxy            │
│     · strip Authorization   │
│     · inject X-User-ID      │
│     · inject X-Real-IP /    │
│       X-Forwarded-For       │
│     · forward to backend    │
└─────────────────────────────┘
  │
  ▼
Backend Service
```

---

## Project Structure

```
distributed-rate-limiter-gateway/
├── cmd/
│   ├── gateway/
│   │   └── main.go           # entry point, wires config, Redis, limiter, proxy, server
│   ├── echoserver/
│   │   └── main.go           # dev utility, HTTP echo server on :9000
│   └── gentoken/
│       └── main.go           # dev utility, generates a signed HS256 JWT for testing
├── internal/
│   ├── config/
│   │   └── config.go         # env var resolution and validation
│   ├── gateway/
│   │   ├── middleware.go     # JWTMiddleware, RateLimitMiddleware, UserIDFromContext
│   │   └── proxy.go          # NewReverseProxy, Director pipeline, tuned Transport
│   ├── limiter/
│   │   ├── limiter.go        # RateLimiter interface
│   │   ├── tokenbucket.go    # TokenBucket, lazy refill via Lua
│   │   └── slidingwindow.go  # SlidingWindow, sorted set log via Lua
│   └── storage/
│       └── redis.go          # NewRedisClient, connection pool + startup ping
├── scripts/
│   ├── token_bucket.lua      # atomic token bucket script (HSET-based)
│   └── sliding_window.lua    # atomic sliding window script (ZSET-based)
├── go.mod
└── go.sum
```

---

## Why Lua Scripts?

both algorithms are implemented as atomic Lua scripts executed server-side in Redis. the entire **read -> compute -> write** cycle for each request is a single indivisible operation, no WATCH/MULTI/EXEC, no optimistic locking retries, no window for another replica to race in.

scripts are loaded from disk at startup and registered with Redis using SHA caching via `redis.NewScript`, so the full script body is only transferred once per connection.

---

## Extending

**add a new rate limiting algorithm:**
1. write a Lua script in `scripts/`
2. implement the `limiter.RateLimiter` interface in `internal/limiter/`
3. add a new `const` to `config/config.go` and wire it up in `cmd/gateway/main.go`

**change the upstream backend:**
- just set `BACKEND_URL`, no code changes needed

**use RS256 instead of HS256:**
```bash
JWT_ALGORITHM=RS256 JWT_PUBLIC_KEY_PATH=/path/to/public.pem ...
```

---

## Limitations

- single backend URL, no load balancing or upstream health checks yet
- no request body size limits enforced at the gateway layer
- rate limiting is per-`user_id` only, no per-route or per-IP granularity yet

---

## License

this project is licensed under the MIT License. see [LICENSE](LICENSE) for the full text.

---
