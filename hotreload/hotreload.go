// Package hotreload provides an atomic-swap double buffer for hot-reloading
// config or data without blocking readers. Pure standard library.
//
// A Loader builds the live value (from a file, a remote endpoint, an in-memory
// generator, etc.); a Buffer stores it behind an atomic pointer. Get is a
// single atomic load and never blocks on a reload, even while a slow Load is
// mid-flight — readers keep reading the previously swapped-in value until the
// new one is fully built and atomically published. Reload swaps the new value
// in a single atomic store.
//
// Concurrency: safe for concurrent use. Get is lock-free. Reload serializes
// Load calls via a mutex so a slow or expensive Load is never invoked twice in
// parallel (parallel Reloads would otherwise waste work and race on the swap).
// Start spawns a single goroutine that calls Reload on a ticker until stop() or
// ctx is cancelled; stop() is idempotent and joins the goroutine.
//
// Ad-tech uses: hot-reloading feature flags / pacing config / creative allow-
// lists / rate-limit thresholds read on every request, where blocking a request
// on a config reload (or reading a half-written value) is unacceptable.
package hotreload

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// ErrLoadFailed is returned by New when the initial load fails. A hot-reload
// buffer must start populated; callers should fix the source and retry rather
// than serve a zero value.
var ErrLoadFailed = errors.New("hotreload: initial load failed")

// Loader builds the live value from a source (file, remote, etc.). Load may be
// slow or fallible; the Buffer never invokes two Loads concurrently.
type Loader[T any] interface {
	Load() (T, error)
}

// Buffer is an atomic-swap double buffer for a value of type T. Get returns the
// current value via a single atomic load; Reload builds a fresh value through
// the Loader and atomically publishes it.
//
// The zero value is not usable: use New, which performs the initial load so the
// buffer is populated before it is handed out. Get on a Buffer whose initial
// Load has not yet completed returns the zero value of T.
type Buffer[T any] struct {
	value atomic.Pointer[T] // current value; lock-free read on Get
	mu    sync.Mutex        // serializes Reload so Load runs at most once at a time

	loader Loader[T]
}

// New builds a Buffer by calling loader.Load once. It returns an error if the
// initial load fails — a hot-reload buffer must start populated, so callers do
// not observe a zero value mid-flight. The returned Buffer is ready for Get.
func New[T any](loader Loader[T]) (*Buffer[T], error) {
	if loader == nil {
		return nil, ErrLoadFailed
	}
	b := &Buffer[T]{loader: loader}
	if err := b.Reload(); err != nil {
		return nil, err
	}
	return b, nil
}

// Get returns the current value. It is a single atomic load and never blocks,
// even while a Reload is mid-Load: readers keep seeing the previously published
// value until the new one is fully built and atomically swapped in. Before a
// successful initial Load, Get returns the zero value of T.
func (b *Buffer[T]) Get() T {
	// atomic.Pointer.Load returns nil before the first successful Reload; in
	// that case return the zero value of T (the documented pre-load behavior).
	// After New succeeds this branch is unreachable because New always loads.
	if p := b.value.Load(); p != nil {
		return *p
	}
	var zero T
	return zero
}

// Reload builds a fresh value via the Loader and atomically publishes it. The
// Load call is serialized: if two Reloads race, one wins the mutex and performs
// the Load while the other waits, so the (potentially slow or expensive) Load
// is never run twice in parallel. A failed Load leaves the previously published
// value intact and returns the error.
func (b *Buffer[T]) Reload() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	v, err := b.loader.Load()
	if err != nil {
		return err
	}
	// Box the value so atomic.Pointer can hold it; Load returned by value, so
	// each Reload publishes its own copy (no shared mutable aliasing).
	b.value.Store(&v)
	return nil
}

// Start spawns a goroutine that calls Reload every interval until stop() is
// called or ctx is cancelled. The returned stop() is idempotent: calling it
// more than once is a no-op. stop() blocks until the reload goroutine has
// exited, so a caller that invokes stop() and then returns is guaranteed no
// goroutine leak. If the context is already cancelled, Start still returns a
// valid stop() (the goroutine observes the cancellation and exits promptly).
//
// A Reload failure inside the loop is ignored (the previously published value
// remains live); callers needing failure visibility should call Reload directly
// or wrap the Loader.
func (b *Buffer[T]) Start(ctx context.Context, interval time.Duration) (stop func()) {
	// Each Start/stop cycle uses LOCAL bookkeeping (stopCh/stopOnce/wg) so a
	// second Start cannot overwrite the first's channels and orphan its reload
	// goroutine. stop() closes this cycle's stopCh and waits on this cycle's wg.
	stopCh := make(chan struct{})
	var stopOnce sync.Once
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Ignore the error: a failed reload leaves the last good value
				// live, which is the desired fail-open behavior for hot config.
				_ = b.Reload()
			}
		}
	}()
	return func() {
		stopOnce.Do(func() { close(stopCh) })
		wg.Wait()
	}
}
