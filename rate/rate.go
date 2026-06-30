// Package rate is a distributed, Redis-backed rate limiter using the Generic
// Cell Rate Algorithm (GCRA) — the same algorithm as redis-cell and
// go-redis/redis_rate.
//
// GCRA tracks a single "theoretical arrival time" (TAT) per key in Redis and
// updates it atomically via a Lua script, so a limit holds across every process
// sharing the Redis. It is the distributed sibling of the in-process limiter:
// use it when many instances must agree on a global rate (a shared bid QPS cap,
// a per-user API budget, a cross-pod postback throttle).
//
// Dependencies: github.com/redis/go-redis/v9. Pass any redis.Cmdable (single,
// cluster, or the kit4go/redis wrapper's Cmdable()).
package rate

import (
	"context"
	"errors"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// Limit is a rate (tokens per Period) with a Burst allowance.
type Limit struct {
	Rate   int           // tokens granted per Period
	Period time.Duration // the period over which Rate is measured
	Burst  int           // maximum tokens that may accumulate (burst)
}

// PerSecond is a convenience for a per-second rate.
func PerSecond(rate, burst int) Limit {
	return Limit{Rate: rate, Period: time.Second, Burst: burst}
}

// PerMinute is a convenience for a per-minute rate.
func PerMinute(rate, burst int) Limit {
	return Limit{Rate: rate, Period: time.Minute, Burst: burst}
}

// Result is the outcome of an Allow/AllowN decision.
type Result struct {
	Allowed    bool          // whether the request was permitted (and consumed a token)
	Remaining  int           // tokens remaining in the bucket after the decision
	RetryAfter time.Duration // when Allowed is false, how long until one token is free (0 when allowed)
}

// ErrLimitInvalid is returned for a non-positive Rate/Period/Burst or n<=0.
var ErrLimitInvalid = errors.New("rate: invalid limit")

// gcraScript atomically checks and advances the TAT for a key.
// Args (all microseconds, as strings): now, emission_interval, burst, cost, ttl_ms.
// Returns: {allowed(0/1), remaining, retry_after_us}.
var gcraScript = goredis.NewScript(`
local key     = KEYS[1]
local now     = tonumber(ARGV[1])
local emiss   = tonumber(ARGV[2])
local burst   = tonumber(ARGV[3])
local cost    = tonumber(ARGV[4])
local ttl_ms  = tonumber(ARGV[5])

local raw = redis.call('GET', key)
local tat
if raw == false or raw == nil then
  tat = now
else
  tat = tonumber(raw)
end
if tat < now then tat = now end

local allowed = false
if cost <= burst and (tat - now) <= emiss * (burst - cost) then
  allowed = true
end

local new_tat = tat
if allowed then
  new_tat = tat + emiss * cost
  redis.call('SET', key, tostring(new_tat), 'PX', ttl_ms)
end

local remaining = burst - math.ceil((new_tat - now) / emiss)
if remaining < 0 then remaining = 0 end
if remaining > burst then remaining = burst end

local retry_after = 0
if not allowed then
  retry_after = math.ceil(new_tat - now - emiss * (burst - cost))
  if retry_after < 0 then retry_after = 0 end
end
return {allowed and 1 or 0, remaining, retry_after}
`)

// Limiter applies GCRA limits against a Redis instance.
type Limiter struct {
	client goredis.Cmdable
	now    func() time.Time
}

// Option configures a Limiter.
type Option func(*Limiter)

// WithClock injects a clock (for tests). Defaults to time.Now.
func WithClock(f func() time.Time) Option { return func(l *Limiter) { l.now = f } }

// New builds a Limiter over the given Redis client.
func New(client goredis.Cmdable, opts ...Option) *Limiter {
	l := &Limiter{client: client, now: time.Now}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// Allow checks whether one token is available for key under limit, consuming it
// when it is.
func (l *Limiter) Allow(ctx context.Context, key string, limit Limit) (Result, error) {
	return l.AllowN(ctx, key, limit, 1)
}

// AllowN checks whether n tokens are available for key under limit, consuming
// them when they are. n must be >= 1.
func (l *Limiter) AllowN(ctx context.Context, key string, limit Limit, n int) (Result, error) {
	if err := validate(limit, n); err != nil {
		return Result{}, err
	}
	emiss := float64(limit.Period.Microseconds()) / float64(limit.Rate) // microseconds per token
	ttl := int64(limit.Period.Microseconds()) * int64(limit.Burst+1) / int64(limit.Rate)
	if ttl < int64(time.Second.Microseconds()) {
		ttl = int64(time.Second.Microseconds())
	}
	nowUs := l.now().UnixMicro()
	res, err := gcraScript.Run(ctx, l.client,
		[]string{key},
		nowUs, emiss, limit.Burst, n, ttl/1000, // ttl in ms
	).Slice()
	if err != nil {
		return Result{}, err
	}
	allowed := res[0].(int64) == 1
	remaining := int(res[1].(int64))
	retry := time.Duration(res[2].(int64)) * time.Microsecond
	return Result{Allowed: allowed, Remaining: remaining, RetryAfter: retry}, nil
}

func validate(limit Limit, n int) error {
	if limit.Rate <= 0 || limit.Period <= 0 || limit.Burst <= 0 {
		return ErrLimitInvalid
	}
	if n <= 0 {
		return ErrLimitInvalid
	}
	return nil
}
