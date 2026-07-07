package objpool

import (
	"bytes"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPanicOnNilNew(t *testing.T) {
	require.Panics(t, func() { New[int](nil) })
}

func TestGetMiss(t *testing.T) {
	p := New(func() *bytes.Buffer { return &bytes.Buffer{} })

	b := p.Get()
	require.NotNil(t, b)

	s := p.Stats()
	require.Equal(t, uint64(1), s.Gets)
	require.Equal(t, uint64(1), s.Misses, "first Get must miss")
	require.Equal(t, uint64(0), s.Puts)
	require.Equal(t, uint64(1), s.InUse)
}

func TestGetHit(t *testing.T) {
	p := New(func() *bytes.Buffer { return &bytes.Buffer{} })

	b := p.Get()
	p.Put(b) // recycle

	b2 := p.Get() // should hit (sync.Pool keeps the recently Put value)
	require.NotNil(t, b2)

	s := p.Stats()
	require.Equal(t, uint64(2), s.Gets)
	require.Equal(t, uint64(1), s.Puts)
	require.Equal(t, uint64(1), s.InUse)
	// sync.Pool does NOT guarantee a hit (the runtime may clear it on GC),
	// so the second Get may reuse the Put value (Misses stays 1) or miss
	// (Misses becomes 2). Assert only the deterministic counts; bound Misses.
	require.GreaterOrEqual(t, s.Misses, uint64(1))
	require.LessOrEqual(t, s.Misses, s.Gets)
}

func TestPutInUseDecrement(t *testing.T) {
	p := New(func() *bytes.Buffer { return &bytes.Buffer{} })

	b := p.Get()
	require.Equal(t, uint64(1), p.Stats().InUse)

	p.Put(b)
	s := p.Stats()
	require.Equal(t, uint64(1), s.Puts)
	require.Equal(t, uint64(0), s.InUse)
}

func TestResetHookApplied(t *testing.T) {
	p := New(
		func() *bytes.Buffer { return &bytes.Buffer{} },
		WithReset(func(b *bytes.Buffer) { b.Reset() }),
	)

	b := p.Get()
	b.WriteString("dirty")
	p.Put(b)

	// Sync.Pool tends to hand back the most recently Put value on the same
	// goroutine, so the reset hook should clear our "dirty" payload.
	b2 := p.Get()
	require.Equal(t, 0, b2.Len(), "reset hook should have cleared the buffer")
}

func TestResetHookNilSafe(t *testing.T) {
	// No WithReset: Get must still succeed and not panic on a nil reset.
	p := New(func() int { return 42 })
	require.Equal(t, 42, p.Get())
}

func TestStatsAllFields(t *testing.T) {
	p := New(func() *bytes.Buffer { return &bytes.Buffer{} })

	for i := range 3 {
		b := p.Get()
		b.WriteByte(byte('a' + i))
		p.Put(b)
	}
	s := p.Stats()
	require.Equal(t, uint64(3), s.Gets)
	require.Equal(t, uint64(3), s.Puts)
	require.Equal(t, uint64(0), s.InUse)
	require.Positive(t, s.Misses)
}

// TestStatsClampsNegativeInUse covers the defensive clamp in Stats. Under
// concurrency Gets - Puts can briefly go negative before the matching Put
// lands; the snapshot must never report a negative uint64. We force the branch
// deterministically by over-decrementing (two Puts per Get) — an abuse of the
// API, but it is the only way to reach the clamp without a flaky race.
func TestStatsClampsNegativeInUse(t *testing.T) {
	p := New(func() int { return 7 })

	x := p.Get() // inUse = 1
	p.Put(x)     // inUse = 0
	p.Put(x)     // inUse = -1 (over-decrement → exercises clamp)

	s := p.Stats()
	require.Equal(t, uint64(0), s.InUse, "negative inUse must be clamped to 0")
}

func TestWithResetOption(t *testing.T) {
	var calls int64
	p := New(
		func() []int { return make([]int, 0, 4) },
		WithReset(func(s []int) { atomic.AddInt64(&calls, 1) }),
	)
	_ = p.Get()
	_ = p.Get()
	require.Equal(t, int64(2), atomic.LoadInt64(&calls), "reset hook fires once per Get")
}

// TestConcurrentGetPut exercises Get/Put/Stats under the race detector. It
// primarily checks that the pool stays consistent and panic-free under heavy
// concurrency; exact counts are not asserted because sync.Pool may drop items
// and the runtime may call pool.New for warm-up.
func TestConcurrentGetPut(t *testing.T) {
	p := New(
		func() *bytes.Buffer { return &bytes.Buffer{} },
		WithReset(func(b *bytes.Buffer) { b.Reset() }),
	)

	const goroutines = 16
	const iters = 500

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			for range iters {
				b := p.Get()
				b.WriteString("x")
				p.Put(b)
			}
		}()
	}
	wg.Wait()

	s := p.Stats()
	require.Equal(t, uint64(goroutines*iters), s.Gets)
	require.Equal(t, uint64(goroutines*iters), s.Puts)
	require.Equal(t, uint64(0), s.InUse, "everything returned; InUse must be 0")
	// Sanity: at least some misses (the first Gets cannot be served from cache).
	require.Positive(t, s.Misses)
}
