// Package semaphore is a weighted counting semaphore for concurrency limiting.
// Pure standard library.
//
// A semaphore with capacity N allows at most N concurrent "permits" to be held
// at once. Acquire blocks until a permit is available; Release returns it.
// Weighted acquires (Acquire(n)) let large operations consume multiple permits.
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
type Semaphore struct {
	mu     sync.Mutex
	cond   *sync.Cond
	count  int64 // currently held permits
	cap    int64 // max permits
	closed bool
}

// New builds a semaphore with the given capacity (must be > 0). Panics otherwise.
func New(capacity int) *Semaphore {
	if capacity <= 0 {
		panic("semaphore: capacity must be > 0")
	}
	s := &Semaphore{cap: int64(capacity)}
	s.cond = sync.NewCond(&s.mu)
	return s
}

// Acquire blocks until n permits are available, then holds them. n must be > 0
// and <= capacity. Returns ErrClosed if the semaphore is closed.
func (s *Semaphore) Acquire(ctx context.Context, n int) error {
	if n <= 0 {
		n = 1
	}
	if int64(n) > s.cap {
		return errors.New("semaphore: requested permits exceed capacity")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	// Set up context cancellation.
	done := ctx.Done()
	if done != nil {
		stop := make(chan struct{})
		defer close(stop)
		go func() {
			select {
			case <-done:
				s.mu.Lock()
				s.cond.Broadcast() // wake the waiter
				s.mu.Unlock()
			case <-stop:
			}
		}()
	}

	for s.count+int64(n) > s.cap && !s.closed && ctx.Err() == nil {
		s.cond.Wait()
	}
	if s.closed {
		return ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.count += int64(n)
	return nil
}

// TryAcquire attempts to acquire n permits without blocking. Returns false if
// unavailable or closed.
func (s *Semaphore) TryAcquire(n int) bool {
	if n <= 0 {
		n = 1
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.count+int64(n) > s.cap {
		return false
	}
	s.count += int64(n)
	return true
}

// Release returns n permits. n must be > 0. Panics if it would underflow.
func (s *Semaphore) Release(n int) {
	if n <= 0 {
		n = 1
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.count -= int64(n)
	if s.count < 0 {
		panic("semaphore: released more permits than acquired")
	}
	s.cond.Broadcast()
}

// Available returns the number of free permits.
func (s *Semaphore) Available() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cap - s.count
}

// Cap returns the configured capacity.
func (s *Semaphore) Cap() int { return int(s.cap) }

// Close shuts down the semaphore. Pending Acquire callers are woken and receive
// ErrClosed. Idempotent.
func (s *Semaphore) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	s.cond.Broadcast()
}
