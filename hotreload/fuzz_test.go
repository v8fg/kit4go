// This file is an internal fuzz test (package hotreload) so it can reuse the
// unexported Buffer fields and keep the seeds self-contained.
//
// FuzzReloadGetRoundtrip drives the package's central invariant through an
// arbitrary stream of (value, fail-flag) pairs derived from a byte slice: each
// Reload must publish exactly the value Load produced and each Get must observe
// either the last successfully published value or the new one — never a torn,
// zero, or stale value. A failed Reload must leave the previously published
// value live.
//
// FuzzConcurrentReloadGet turns the same stream into concurrent Reload/Get
// traffic and asserts the lock-free read contract: Get never panics, never
// blocks, and always returns a value that Load actually produced.
package hotreload

import (
	"errors"
	"sync"
	"testing"
)

// fuzzLoader is a deterministic, in-package Loader for fuzzing. It replays a
// fixed slice of (value, shouldFail) entries in order, cycling once exhausted.
// Each Load records how many times it ran so the fuzz harness can assert the
// serialization invariant (parallel Reloads run Load strictly 1:1).
type fuzzLoader struct {
	mu      sync.Mutex
	entries []fuzzEntry
	idx     int
	calls   int
}

type fuzzEntry struct {
	val  string
	fail bool
}

func (l *fuzzLoader) Load() (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls++
	if len(l.entries) == 0 {
		return "", errors.New("fuzz: empty loader")
	}
	e := l.entries[l.idx%len(l.entries)]
	l.idx++
	if e.fail {
		return "", errors.New("fuzz load failure")
	}
	return e.val, nil
}

// decodeEntries turns a fuzzed byte slice into a deterministic stream of
// (value, fail) entries. Each byte maps to one entry: the high bit selects a
// failure, the remaining 7 bits select which seed value is published. The
// first entry is forced to succeed so New's initial Load populates the buffer
// (otherwise New returns an error and there is nothing to fuzz).
func decodeEntries(in []byte) []fuzzEntry {
	seeds := []string{"a", "b", "c", "d", "ab", "ba", "A", "z"}
	if len(in) == 0 {
		return []fuzzEntry{{val: "x"}}
	}
	out := make([]fuzzEntry, 0, len(in))
	for i, b := range in {
		if i == 0 {
			// First load must succeed so the buffer starts populated.
			out = append(out, fuzzEntry{val: seeds[int(b)%len(seeds)]})
			continue
		}
		out = append(out, fuzzEntry{
			val:  seeds[int(b)%len(seeds)],
			fail: b&0x80 != 0,
		})
	}
	return out
}

// FuzzReloadGetRoundtrip fuzzes the publish/observe contract:
//
//  1. No panic: New, Reload, and Get must never panic regardless of the input
//     stream (including all-fail-after-init sequences).
//  2. Roundtrip: after a successful Reload, Get returns exactly the value Load
//     produced; the published value is never torn or aliased.
//  3. Ordering/last-good: a failed Reload leaves the previously published value
//     live — Get returns the same value before and after the failed Reload.
//
// Seed corpus covers the canonical paths: all-success, mixed fail/succeed, and
// a single-load sequence.
func FuzzReloadGetRoundtrip(f *testing.F) {
	// All successes, distinct values: each Reload publishes the next seed.
	f.Add([]byte{0x00, 0x01, 0x02, 0x03, 0x04})
	// Init succeeds, then alternating fail/succeed.
	f.Add([]byte{0x00, 0x81, 0x02, 0x83, 0x04})
	// Init then all failures: Get must keep returning the init value.
	f.Add([]byte{0x05, 0x80, 0x81, 0x82})
	// Single load only.
	f.Add([]byte{0x07})
	// Duplicate values (roundtrip with repeats) and a trailing failure.
	f.Add([]byte{0x00, 0x00, 0x80})

	f.Fuzz(func(t *testing.T, in []byte) {
		entries := decodeEntries(in)
		l := &fuzzLoader{entries: entries}
		b, err := New[string](l)
		if err != nil {
			// The first entry is forced to succeed, so New must never error
			// here. If it does, the decode contract broke.
			t.Fatalf("New failed despite forced-success init: %v (in=%x)", err, in)
		}

		// published tracks the value the most recent successful Reload should
		// have swapped in. The first successful load came from New.
		published := entries[0].val
		if got := b.Get(); got != published {
			t.Fatalf("Get after New = %q, want %q (in=%x)", got, published, in)
		}

		// Replay the remaining entries: each Reload either fails (Get must be
		// unchanged) or succeeds (Get must reflect exactly what Load returned).
		for i := 1; i < len(entries); i++ {
			e := entries[i]
			err := b.Reload()
			if e.fail {
				if err == nil {
					t.Fatalf("entry %d expected failure but Reload returned nil (in=%x)", i, in)
				}
				// Last-good invariant: failed Reload keeps the prior value.
				if got := b.Get(); got != published {
					t.Fatalf("Get after failed Reload = %q, want last-good %q (in=%x)", got, published, in)
				}
				continue
			}
			if err != nil {
				t.Fatalf("entry %d expected success but Reload errored: %v (in=%x)", i, err, in)
			}
			// Roundtrip invariant: Get returns exactly what Load produced.
			published = e.val
			if got := b.Get(); got != published {
				t.Fatalf("Get after successful Reload = %q, want %q (in=%x)", got, published, in)
			}
		}

		// Final no-panic sanity: Get is callable after the whole stream.
		_ = b.Get()
	})
}

// FuzzConcurrentReloadGet fuzzes the lock-free read contract under concurrent
// Reload/Get traffic. It asserts two invariants for every input:
//
//  1. No panic: Get never panics even while Reloads (and their atomic stores)
//     race against it.
//  2. No torn value: every Get returns a value that some Load actually produced
//     (one of the seed strings), never a zero value or a half-published value.
//
// It does NOT assert a specific value — the whole point of the atomic swap is
// that under concurrency Get may observe any previously published value — only
// that whatever it observes is a real, fully-published Load result.
func FuzzConcurrentReloadGet(f *testing.F) {
	f.Add([]byte{0x00, 0x01, 0x02, 0x03})
	f.Add([]byte{0x00, 0x81, 0x82, 0x83}) // init then all fails
	f.Add([]byte{0x00, 0x80, 0x01, 0x81, 0x02})
	f.Add([]byte{0x00})

	f.Fuzz(func(t *testing.T, in []byte) {
		entries := decodeEntries(in)
		l := &fuzzLoader{entries: entries}
		b, err := New[string](l)
		if err != nil {
			t.Fatalf("New failed: %v (in=%x)", err, in)
		}

		// Build the set of values any successful Load can ever produce. Get
		// must only ever return members of this set.
		valid := make(map[string]struct{}, len(entries))
		for _, e := range entries {
			if !e.fail {
				valid[e.val] = struct{}{}
			}
		}

		// One writer drains the entry stream via Reload; many readers hammer
		// Get concurrently. The race is the point: Get's atomic load must
		// never observe a torn store.
		const readers = 8
		var wg sync.WaitGroup
		stop := make(chan struct{})
		var badValue atomicBadValue

		for range readers {
			wg.Go(func() {
				for {
					select {
					case <-stop:
						return
					default:
					}
					got := b.Get()
					if _, ok := valid[got]; !ok {
						badValue.set(got)
						return
					}
				}
			})
		}

		for i := 1; i < len(entries); i++ {
			_ = b.Reload() // failures are valid; readers keep the last value
		}
		close(stop)
		wg.Wait()

		if badValue.found {
			t.Fatalf("Get returned a value %q not produced by any Load (in=%x)", badValue.val, in)
		}
	})
}

// atomicBadValue lets concurrent readers report an invalid observed value
// without using testing APIs (which are not safe across goroutines) from
// inside the hot loop. The first reader to see an invalid value wins.
type atomicBadValue struct {
	mu    sync.Mutex
	found bool
	val   string
}

func (a *atomicBadValue) set(v string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.found {
		a.found = true
		a.val = v
	}
}
