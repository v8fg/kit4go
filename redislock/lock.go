// Package redislock implements a simple, correct distributed lock on top of a
// Redis client (go-redis/v9 redis.Cmdable).
//
// Acquire uses SET key token NX PX ttl — atomic and single-roundtrip. Release
// and Refresh use Lua scripts so a holder only ever touches its own lock (the
// token guards against releasing a lock that has already expired and been
// re-acquired by someone else). An optional heartbeat goroutine renews the TTL
// while the critical section runs.
//
// This is a single-instance Redis lock (one Redis node). For environments that
// cannot tolerate a single Redis being a SPoF, run Redis in a failover setup
// (Sentinel/Cluster) and let the client route to the current master. Ad-tech
// uses: single-flight budget/pacing updates, leader election, dedup of
// concurrent bid requests for the same auction.
package redislock

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// ErrLockNotAcquired is returned by TryLock (or Lock on timeout) when the lock
// is held by someone else.
var ErrLockNotAcquired = errors.New("redislock: lock not acquired")

// ErrLockLost is reported via Lock.Lost when an auto-renew fails (the TTL
// expired before renewal succeeded — the lock can no longer be considered held).
var ErrLockLost = errors.New("redislock: lock lost (renewal failed)")

// Lua release: delete only if the stored value matches our token.
var releaseScript = goredis.NewScript(`
if redis.call('GET', KEYS[1]) == ARGV[1] then
	return redis.call('DEL', KEYS[1])
else
	return 0
end
`)

// Lua refresh: extend TTL only if the stored value matches our token.
var refreshScript = goredis.NewScript(`
if redis.call('GET', KEYS[1]) == ARGV[1] then
	return redis.call('PEXPIRE', KEYS[1], ARGV[2])
else
	return 0
end
`)

type options struct {
	ttl           time.Duration
	retryInterval time.Duration // for Lock (blocking)
	waitTimeout   time.Duration // 0 = until ctx done
	token         string        // empty = random
	autoRenew     bool
	renewInterval time.Duration // 0 = ttl/2
	onLost        func(error)   // invoked once when an auto-renew fails
}

// Option configures a Locker.
type Option func(*options)

// WithTTL sets the lock's time-to-live (default 10s). Choose a value safely
// above the worst-case critical-section duration unless auto-renew is on.
func WithTTL(d time.Duration) Option { return func(o *options) { o.ttl = d } }

// WithRetryInterval sets the delay between acquisition attempts in Lock
// (default 50ms).
func WithRetryInterval(d time.Duration) Option { return func(o *options) { o.retryInterval = d } }

// WithWaitTimeout sets the maximum total time Lock will spend retrying (default
// 0 = retry until ctx is done).
func WithWaitTimeout(d time.Duration) Option { return func(o *options) { o.waitTimeout = d } }

// WithToken sets an explicit owner token instead of a random one. Use only when
// you need a stable identity across re-acquisitions by the SAME holder. The
// token MUST be unique per active holder — sharing it across concurrent locks
// on the same key defeats the Lua ownership guard: a stale Release from holder
// A would pass the GET==token check and delete holder B's lock (split-brain).
// For distinct holders, use the default random token (crypto/rand 16 bytes).
func WithToken(token string) Option { return func(o *options) { o.token = token } }

// WithAutoRenew enables a heartbeat goroutine that extends the TTL while the
// lock is held. Renewal runs every renewInterval (default TTL/2). If a renewal
// fails, the lock is reported lost via Lost() and the onLost callback (if set).
func WithAutoRenew(on bool) Option { return func(o *options) { o.autoRenew = on } }

// WithRenewInterval overrides the auto-renew interval (default TTL/2).
func WithRenewInterval(d time.Duration) Option { return func(o *options) { o.renewInterval = d } }

// WithOnLost registers a callback invoked once when an auto-renew fails.
func WithOnLost(fn func(error)) Option { return func(o *options) { o.onLost = fn } }

// Locker acquires distributed locks against a Redis instance.
type Locker struct {
	client goredis.Cmdable
	opts   options
}

// New builds a Locker over the given Redis client (single-node, cluster, or the
// redis.Cmdable from the kit4go/redis wrapper).
func New(client goredis.Cmdable, opts ...Option) *Locker {
	o := options{
		ttl:           10 * time.Second,
		retryInterval: 50 * time.Millisecond,
	}
	for _, opt := range opts {
		opt(&o)
	}
	return &Locker{client: client, opts: o}
}

// TryLock makes a single attempt to acquire key. Returns ErrLockNotAcquired if
// the lock is currently held.
func (l *Locker) TryLock(ctx context.Context, key string) (*Lock, error) {
	return l.tryLock(ctx, key, l.opts.token)
}

// Lock blocks until the lock is acquired, ctx is done, or the wait timeout
// elapses. It retries every retryInterval.
func (l *Locker) Lock(ctx context.Context, key string) (*Lock, error) {
	deadline := time.Time{}
	if l.opts.waitTimeout > 0 {
		deadline = time.Now().Add(l.opts.waitTimeout)
	}
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
		}
		lk, err := l.tryLock(ctx, key, l.opts.token)
		if err == nil {
			return lk, nil
		}
		if !errors.Is(err, ErrLockNotAcquired) {
			return nil, err
		}
		if !deadline.IsZero() && !time.Now().Before(deadline) {
			return nil, ErrLockNotAcquired
		}
		// Wait retryInterval (respecting ctx) before the next attempt.
		interval := l.opts.retryInterval
		if interval <= 0 {
			interval = 50 * time.Millisecond
		}
		remaining := interval
		if !deadline.IsZero() {
			if r := time.Until(deadline); r < remaining {
				remaining = r
			}
		}
		timer.Reset(remaining)
	}
}

func (l *Locker) tryLock(ctx context.Context, key, token string) (*Lock, error) {
	if token == "" {
		var err error
		token, err = randomToken()
		if err != nil {
			return nil, fmt.Errorf("redislock: generate token: %w", err)
		}
	}
	ok, err := l.client.SetNX(ctx, key, token, l.opts.ttl).Result()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrLockNotAcquired
	}
	lk := &Lock{
		client:        l.client,
		key:           key,
		token:         token,
		ttl:           l.opts.ttl,
		autoRenew:     l.opts.autoRenew,
		renewInterval: l.opts.renewInterval,
		onLost:        l.opts.onLost,
		lost:          make(chan struct{}),
	}
	if lk.autoRenew {
		// Derive a context from the acquire ctx so its cancellation (request done,
		// shutdown, parent timeout) stops the renewer instead of leaking the lock
		// on context.Background().
		lk.acqCtx, lk.acqCancel = context.WithCancel(ctx)
		lk.startRenewer(lk.acqCtx)
	}
	return lk, nil
}

// Lock represents a held distributed lock.
type Lock struct {
	client        goredis.Cmdable
	key           string
	token         string
	ttl           time.Duration
	autoRenew     bool
	renewInterval time.Duration
	onLost        func(error)

	stop     chan struct{} // closed by Release to stop the renewer
	stopOnce sync.Once
	lost     chan struct{} // closed when an auto-renew fails
	lostOnce sync.Once

	acqCtx    context.Context    // acquire context; its cancellation stops the renewer (treated as a loss)
	acqCancel context.CancelFunc // cancelled by Release to abort an in-flight Refresh fast
}

// Key returns the locked key.
func (l *Lock) Key() string { return l.key }

// Token returns the owner token written into Redis.
func (l *Lock) Token() string { return l.token }

// Refresh extends the lock's TTL, but only if this holder still owns it.
func (l *Lock) Refresh(ctx context.Context) error {
	res, err := refreshScript.Run(ctx, l.client, []string{l.key}, l.token,
		l.ttl.Milliseconds()).Result()
	if err != nil {
		return err
	}
	if n, ok := res.(int64); !ok || n == 0 {
		return ErrLockNotAcquired
	}
	return nil
}

// Release gives up the lock atomically (no-op if ownership was already lost).
func (l *Lock) Release(ctx context.Context) error {
	l.stopOnce.Do(func() {
		if l.stop != nil {
			close(l.stop)
		}
		if l.acqCancel != nil {
			l.acqCancel() // abort any in-flight Refresh fast; renewer exits cleanly
		}
	})
	res, err := releaseScript.Run(ctx, l.client, []string{l.key}, l.token).Result()
	if err != nil {
		// If the error is nil reply (key gone), treat as already released.
		if errors.Is(err, goredis.Nil) {
			return nil
		}
		return err
	}
	if n, ok := res.(int64); !ok || n == 0 {
		return ErrLockNotAcquired
	}
	return nil
}

// Lost returns a channel that is closed when an auto-renew fails (the lock can
// no longer be considered held). It is never closed for non-auto-renewed locks.
func (l *Lock) Lost() <-chan struct{} { return l.lost }

// startRenewer runs a goroutine extending the TTL until Release, a renewal
// failure, or cancellation of the acquire context. On a real loss it closes Lost
// and invokes onLost once. ctx is the acquire context (not Background) so its
// cancellation stops the renewer instead of leaking the lock.
func (l *Lock) startRenewer(ctx context.Context) {
	interval := l.renewInterval
	if interval <= 0 {
		interval = l.ttl / 2
	}
	if interval <= 0 {
		interval = time.Second
	}
	l.stop = make(chan struct{})
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-l.stop:
				return
			case <-ctx.Done():
				// Acquire context cancelled without Release — the lock is
				// effectively abandoned (request done / shutdown / parent
				// timeout). Treat as a loss unless Release is stopping us.
				l.handleLoss(ctx.Err())
				return
			case <-ticker.C:
				if err := l.Refresh(ctx); err != nil {
					l.handleLoss(err)
					return
				}
			}
		}
	}()
}

// handleLoss reports a lost lock: invokes onLost then closes Lost() — UNLESS the
// renewer is being stopped by a clean Release, in which case the "loss" is just
// Release deleting the key under an in-flight Refresh (or cancelling the ctx),
// not a real loss, and must NOT fire onLost. Lost() is closed AFTER onLost (via
// defer) so the callback's effects are happens-before the close, AND it still
// closes if onLost panics (the recover ensures a panicking callback does not
// crash the host process — the renewer is a library-owned goroutine per the
// kit4go callback convention).
func (l *Lock) handleLoss(err error) {
	select {
	case <-l.stop:
		return // Release is in progress/done — clean shutdown, not a loss
	default:
	}
	defer l.lostOnce.Do(func() { close(l.lost) }) // closes even if onLost panics
	defer func() {
		if r := recover(); r != nil {
			// A panicking onLost must not crash the host. The lostOnce defer
			// above still runs (defers execute in LIFO order: recover first,
			// then close). The panic is swallowed — the renewer goroutine
			// exits cleanly after this function returns.
			_ = r
		}
	}()
	if l.onLost != nil {
		l.onLost(err) // runs before close: its effects are happens-before Lost()
	}
}

// randomToken returns a 16-byte hex token.
func randomToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
