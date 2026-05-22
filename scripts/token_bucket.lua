-- token_bucket.lua: lazy refill token bucket
-- KEYS[1] bucket key
-- ARGV[1] current unix timestamp (seconds)
-- ARGV[2] max_tokens (integer)
-- ARGV[3] refill_rate (tokens/second, float)
-- ARGV[4] ttl (seconds)

local key        = KEYS[1]
local now        = tonumber(ARGV[1])
local max_tokens = tonumber(ARGV[2])
local rate       = tonumber(ARGV[3])
local ttl        = tonumber(ARGV[4])

local data       = redis.call('HMGET', key, 'tokens', 'last_updated')
local tokens     = tonumber(data[1])
local last_upd   = tonumber(data[2])

if tokens == nil then
    -- first request: bucket does not exist; initialize and consume one token
    redis.call('HMSET', key, 'tokens', max_tokens - 1, 'last_updated', now)
    redis.call('EXPIRE', key, ttl)
    return 1
end

-- lazy refill calculation
local delta     = now - last_upd
local generated = delta * rate
tokens = math.min(max_tokens, tokens + generated)

if tokens >= 1 then
    -- consume one token
    redis.call('HMSET', key, 'tokens', tokens - 1, 'last_updated', now)
    redis.call('EXPIRE', key, ttl)
    return 1
end

-- bucket empty
return 0
