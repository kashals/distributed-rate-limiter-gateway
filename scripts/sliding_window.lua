-- sliding_window.lua: sorted set sliding window log
-- KEYS[1] sorted set key
-- ARGV[1] current timestamp (microseconds)
-- ARGV[2] window size (microseconds)
-- ARGV[3] max requests per window (integer)
-- ARGV[4] ttl (seconds)

local key       = KEYS[1]
local now       = tonumber(ARGV[1])
local window    = tonumber(ARGV[2])
local max_limit = tonumber(ARGV[3])
local ttl       = tonumber(ARGV[4])

local boundary  = now - window

-- evict stale entries outside the current window
redis.call('ZREMRANGEBYSCORE', key, '-inf', boundary)

-- count requests active within the window
local count = redis.call('ZCARD', key)

if count < max_limit then
    -- record this request; member == score == microsecond timestamp for uniqueness
    redis.call('ZADD', key, now, now)
    redis.call('EXPIRE', key, ttl)
    return 1
end

-- window saturated
return 0
