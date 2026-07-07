// Package semaphore is a weighted counting semaphore for concurrency limiting.
// Pure standard library.
//
// A semaphore with capacity N allows at most N concurrent "permits" to be held
// at once. Acquire blocks until a permit is available; Release returns it.
// Weighted acquires (Acquire(n)) let large operations consume multiple permits.
//
// The unit-permit fast path (Acquire(ctx, 1) / TryAcquire(1) / Release(1)) is
// implemented with a buffered channel: it allocates nothing and spawns no
// goroutine per call. The previous design spawned a goroutine (and a watch
// channel) on every Acquire to observe ctx.Done(); for a per-request limiter on
// the 100k+/s hot path that was avoidable churn.
//
// Ad-tech uses: limit concurrent outbound calls to a specific SSP (each bid
// request consumes 1 permit; the SSP can handle at most 100 concurrent), limit
// concurrent DB connections per pool, or cap goroutine fan-out per worker.
package semaphore

import (
	"context"
	"errors"
	"sync"
)

// ErrClosed is returned by Acquire after Close.
var ErrClosed = errors.New("semaphore: closed")

// Semaphore is a weighted counting semaphore.
//
// Permits are modeled as tokens in a buffered channel of capacity = limit,
// pre-filled at construction. The unit-permit (n==1) fast path is a pure
// channel send/recv: zero allocations and zero goroutines per Acquire.
// Weighted (n>1) operations serialize on wmu to keep multi-token takes atomic.
type Semaphore struct {
	ch     chan struct{} // token buffer; len(ch) == available permits
	closed chan struct{} // closed on Close(); never sends, only closed
	once   sync.Once

	// wmu guards weighted (n>1) takes so a multi-token acquire is atomic.
	// The n==1 fast path never touches it.
	wmu sync.Mutex
}

// New builds a semaphore with the given capacity (must be > 0). Panics otherwise.
func New(capacity int) *Semaphore {
	if capacity <= 0 {
		panic("semaphore: capacity must be > 0")
	}
	s := &Semaphore{
		ch:     make(chan struct{}, capacity),
		closed: make(chan struct{}),
	}
	for range capacity {
		s.ch <- struct{}{}
	}
	return s
}

// Acquire blocks until n permits are available, then holds them. n must be > 0
// and <= capacity (n <= 0 is treated as 1). Returns ErrClosed if the semaphore
// is closed while waiting, and ctx.Err() if the context is cancelled.
//
// The n==1 fast path allocates nothing and starts no goroutine.
func (s *Semaphore) Acquire(ctx context.Context, n int) error {
	if n <= 0 {
		n = 1
	}
	if n > cap(s.ch) {
		return errors.New("semaphore: requested permits exceed capacity")
	}

	// Fast path: unit permit, pure channel select, zero alloc / zero goroutine.
	if n == 1 {
		select {
		case <-s.ch:
			return nil
		case <-s.closed:
			return ErrClosed
		default:
		}
		select {
		case <-s.ch:
			return nil
		case <-s.closed:
			return ErrClosed
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// Weighted path: take n tokens atomically under wmu, but give up the lock
	// while waiting so unit-permit traffic is not blocked by a large weighted
	// acquire that is itself waiting for tokens.
	done := ctx.Done()
	for {
		s.wmu.Lock()
		if len(s.ch) >= n {
			s.takeLocked(n)
			s.wmu.Unlock()
			return nil
		}
		s.wmu.Unlock()

		// No goroutine, no allocation: block on any token returning, Close, or
		// ctx until something changes, then re-check under the lock.
		select {
		case <-s.ch:
			// A token is available again; put it straight back and re-evaluate
			// atomically (we may still not have enough for n).
			s.ch <- struct{}{}
			continue
		case <-s.closed:
			return ErrClosed
		case <-done:
			return ctx.Err()
		}
	}
}

// takeLocked removes n tokens from the channel. Caller holds wmu.
func (s *Semaphore) takeLocked(n int) {
	for range n {
		<-s.ch
	}
}

// TryAcquire attempts to acquire n permits without blocking (n <= 0 is treated
// as 1). Returns false if unavailable or closed.
func (s *Semaphore) TryAcquire(n int) bool {
	if n <= 0 {
		n = 1
	}
	if n > cap(s.ch) {
		return false
	}

	// Fast path: unit permit, non-blocking channel recv.
	if n == 1 {
		select {
		case <-s.ch:
			return true
		default:
			return false
		}
	}

	// Weighted path: atomic check-and-take under wmu.
	s.wmu.Lock()
	defer s.wmu.Unlock()
	if len(s.ch) < n {
		return false
	}
	s.takeLocked(n)
	return true
}

// Release returns n permits (n <= 0 is treated as 1). It panics if more permits
// are released than were acquired (i.e. it would exceed capacity), mirroring the
// original underflow guarantee. After Close, Release is a no-op so deferred
// Releases on the shutdown path do not panic.
func (s *Semaphore) Release(n int) {
	if n <= 0 {
		n = 1
	}

	// If already closed, tokens are being drained to unblock Release callers;
	// dropping a late Release is the safe, non-panicking choice.
	select {
	case <-s.closed:
		return
	default:
	}

	for range n {
		select {
		case s.ch <- struct{}{}:
		default:
			panic("semaphore: released more permits than acquired")
		}
	}
}

// Available returns the number of free permits.
func (s *Semaphore) Available() int64 {
	return int64(len(s.ch))
}

// Cap returns the configured capacity.
func (s *Semaphore) Cap() int { return cap(s.ch) }

// Close shuts down the semaphore. Pending Acquire callers are woken and receive
// ErrClosed. Idempotent. After Close, Release is a no-op (deferred Releases on
// the shutdown path do not panic).
func (s *Semaphore) Close() {
	s.once.Do(func() {
		close(s.closed)
	})
}
