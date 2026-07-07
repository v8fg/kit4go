package objpool

import (
	"bytes"
	"testing"
)

// newBufferPool builds a *bytes.Buffer pool, optionally with a reset hook. Kept
// here so each benchmark sets up its own Pool without leaking state.
func newBufferPool(reset bool) *Pool[*bytes.Buffer] {
	newFn := func() *bytes.Buffer { return &bytes.Buffer{} }
	if !reset {
		return New(newFn)
	}
	return New(newFn, WithReset(func(b *bytes.Buffer) { b.Reset() }))
}

// BenchmarkNew measures pool construction cost (constructor registration + the
// sync.Pool warm-up the runtime may trigger). Out of the hot path but useful as
// a baseline for the per-Pool setup overhead.
func BenchmarkNew(b *testing.B) {
	newFn := func() *bytes.Buffer { return &bytes.Buffer{} }
	reset := WithReset(func(buf *bytes.Buffer) { buf.Reset() })
	b.ReportAllocs()

	for b.Loop() {
		_ = New(newFn, reset)
	}
}

// BenchmarkGetNoResetCold stresses the miss path: every Get invokes the
// constructor because nothing is Put back. The b.N separate buffers allocated
// here are never returned, so each iteration is a cache miss plus an alloc.
func BenchmarkGetNoResetCold(b *testing.B) {
	p := newBufferPool(false)
	b.ReportAllocs()

	for b.Loop() {
		_ = p.Get()
	}
}

// BenchmarkGetPutNoReset is the steady-state hot path: Get then immediately Put
// on the same goroutine, no reset hook. Once the pool is warm almost every Get
// is a hit, so this isolates sync.Pool round-trip + atomic-counter cost.
func BenchmarkGetPutNoReset(b *testing.B) {
	p := newBufferPool(false)
	b.ReportAllocs()

	for b.Loop() {
		x := p.Get()
		p.Put(x)
	}
}

// BenchmarkGetPutWithReset mirrors BenchmarkGetPutNoReset but installs the
// reset hook, so the per-Get Reset call cost is included. Comparing the two
// shows the reset-hook overhead on the hot path.
func BenchmarkGetPutWithReset(b *testing.B) {
	p := newBufferPool(true)
	b.ReportAllocs()

	for b.Loop() {
		x := p.Get()
		p.Put(x)
	}
}

// BenchmarkStats measures the snapshot cost: four atomic loads plus the clamp.
// Stats is meant to be cheap enough to call from metrics scrapers.
func BenchmarkStats(b *testing.B) {
	p := newBufferPool(false)
	x := p.Get()
	p.Put(x)
	b.ReportAllocs()

	for b.Loop() {
		_ = p.Stats()
	}
}
