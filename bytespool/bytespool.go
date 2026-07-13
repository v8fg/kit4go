// Package bytespool provides a sync.Pool of *bytes.Buffer bucketed by size
// class, so a caller requesting a buffer of size N gets one from the pool whose
// capacity is closest to N. This avoids the "one giant pool" problem where
// small requests get oversized buffers (wasting memory) and large requests get
// undersized buffers (triggering regrow).
//
// The pool is calibrated for hot-path serialization: log formatting, JSON
// encoding, HTTP response building — anywhere bytes.Buffer allocations dominate
// GC pressure.
//
// Pure standard library.
package bytespool

import (
	"bytes"
	"sync"
)

// minSize and maxSize define the range of size classes. Requests below minSize
// get the smallest class; above maxSize get the largest (then grow naturally).
const (
	minSize     = 64
	maxSize     = 65536 // 64 KiB
	sizeClasses = 20    // 64, 128, 256, ... 64K (powers of 2)
)

var pools [sizeClasses]sync.Pool

func init() {
	for i := range sizeClasses {
		size := classSize(i)
		pools[i].New = func() any {
			b := bytes.NewBuffer(make([]byte, 0, size))
			return b
		}
	}
}

func classSize(i int) int {
	return minSize << i // 64 << i
}

func classIndex(n int) int {
	if n <= minSize {
		return 0
	}
	if n > maxSize {
		return sizeClasses - 1
	}
	// log2(n / minSize)
	idx := 0
	v := n >> 6 // n / 64
	for v > 1 {
		v >>= 1
		idx++
	}
	if idx >= sizeClasses {
		idx = sizeClasses - 1
	}
	return idx
}

// Get returns a *bytes.Buffer with at least minCap bytes of capacity, pooled by
// the closest size class. The buffer is reset (Len=0) before return.
func Get(minCap int) *bytes.Buffer {
	idx := classIndex(minCap)
	b := pools[idx].Get().(*bytes.Buffer)
	b.Reset()
	return b
}

// Put returns a buffer to the pool for reuse. Pass nil to skip (no-op).
// Buffers larger than maxSize*2 are discarded (avoid retaining oversized buffers).
func Put(b *bytes.Buffer) {
	if b == nil {
		return
	}
	if b.Cap() > maxSize*2 {
		return // discard oversized
	}
	idx := classIndex(b.Cap())
	pools[idx].Put(b)
}

// WithBuffer calls fn with a pooled buffer and returns it after. This is the
// recommended pattern — the buffer is always returned, even on panic.
func WithBuffer(minCap int, fn func(*bytes.Buffer)) {
	b := Get(minCap)
	defer Put(b)
	fn(b)
}
