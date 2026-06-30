// Package countmin is a Count-Min Sketch: a fixed-space data structure that
// estimates the frequency of elements in a stream. Estimates are always
// over-approximations (never under-count).
//
// It uses d independent hash rows, each over w counters; Add increments all d
// positions, Estimate returns their min. The minimum across rows cancels most
// hash collisions, so a frequent element's estimate stays close to its true
// count while rare elements may over-count slightly. Pure standard library.
//
// Ad-tech uses: heavy-hitter detection (top SSPs / creatives by volume), traffic
// share, approximate frequency where exact counting is too costly, and as a
// cheaper frequency signal than per-key storage. Pair with freqcap for exact
// per-entity caps.
package countmin

import (
	"errors"
	"hash/fnv"
	"math"
)

// ErrIncompatible is returned by Merge for sketches of differing shape.
var ErrIncompatible = errors.New("countmin: incompatible width/depth")

// CountMinSketch estimates element frequencies. Add is NOT internally
// synchronized (hot path); use per-shard sketches + Merge for concurrency.
type CountMinSketch struct {
	w      uint32     // counters per row
	d      uint32     // number of rows
	counts [][]uint64 // d rows × w counters
	total  uint64     // total increments (for merge sanity / heaviness)
}

// New builds a sketch with the given width (counters per row) and depth (rows).
// More depth = lower probability of a large over-estimate; more width = smaller
// expected over-estimate.
func New(width, depth uint) *CountMinSketch {
	if width == 0 {
		width = 2048
	}
	if depth == 0 {
		depth = 5
	}
	c := &CountMinSketch{
		w:      uint32(width),
		d:      uint32(depth),
		counts: make([][]uint64, depth),
	}
	for i := range c.counts {
		c.counts[i] = make([]uint64, width)
	}
	return c
}

// NewForError builds a sketch sized for the desired error bound: the estimate
// over-counts by at most width*epsilon with probability >= 1-delta. (width here
// is the additive error in count, epsilon the relative slack; standard CMS
// sizing: w = ceil(e/epsilon), d = ceil(ln(1/delta)).)
func NewForError(epsilon, delta float64) *CountMinSketch {
	if epsilon <= 0 {
		epsilon = 0.001
	}
	if delta <= 0 || delta >= 1 {
		delta = 0.001
	}
	w := uint(math.Ceil(math.E / epsilon))
	d := uint(math.Ceil(math.Log(1.0 / delta)))
	return New(w, d)
}

// Add increments the counters for data by count (use 1 for a single event).
func (c *CountMinSketch) Add(data []byte, count uint64) {
	h1, h2 := doubleHash(data)
	for i := uint32(0); i < c.d; i++ {
		idx := (h1 + uint64(i)*h2) % uint64(c.w)
		c.counts[i][idx] += count
	}
	c.total += count
}

// AddString is a string convenience for Add(data, 1).
func (c *CountMinSketch) AddString(s string) { c.Add([]byte(s), 1) }

// Estimate returns the approximate frequency of data (>= the true count).
func (c *CountMinSketch) Estimate(data []byte) uint64 {
	h1, h2 := doubleHash(data)
	var min uint64 = ^uint64(0)
	for i := uint32(0); i < c.d; i++ {
		idx := (h1 + uint64(i)*h2) % uint64(c.w)
		v := c.counts[i][idx]
		if v < min {
			min = v
		}
	}
	return min
}

// EstimateString is a string convenience for Estimate.
func (c *CountMinSketch) EstimateString(s string) uint64 { return c.Estimate([]byte(s)) }

// Total returns the sum of all Add counts (the stream length), useful for
// computing a heavy-hitter's share.
func (c *CountMinSketch) Total() uint64 { return c.total }

// Width returns counters per row; Depth returns the row count.
func (c *CountMinSketch) Width() uint32 { return c.w }

// Depth returns the row count.
func (c *CountMinSketch) Depth() uint32 { return c.d }

// Reset zeroes all counters.
func (c *CountMinSketch) Reset() {
	for _, row := range c.counts {
		for j := range row {
			row[j] = 0
		}
	}
	c.total = 0
}

// Merge adds another sketch's counts into this one (sum). Requires identical
// width and depth.
func (c *CountMinSketch) Merge(other *CountMinSketch) error {
	if c.w != other.w || c.d != other.d {
		return ErrIncompatible
	}
	for i, row := range other.counts {
		for j, v := range row {
			c.counts[i][j] += v
		}
	}
	c.total += other.total
	return nil
}

// doubleHash returns two 64-bit hashes of data for the d-row layout (Kirsch-
// Mitzenmacher: g_i = h1 + i*h2). h2 is forced non-zero.
func doubleHash(data []byte) (uint64, uint64) {
	a := fnv.New64a()
	_, _ = a.Write(data)
	h1 := splitmix64(a.Sum64())
	b := fnv.New64()
	_, _ = b.Write(data)
	h2 := splitmix64(b.Sum64())
	if h2 == 0 {
		h2 = 1
	}
	return h1, h2
}

// splitmix64 finalizer to decorrelate FNV output.
func splitmix64(x uint64) uint64 {
	x += 0x9E3779B97F4A7C15
	x = (x ^ (x >> 30)) * 0xBF58476D1CE4E5B9
	x = (x ^ (x >> 27)) * 0x94D049BB133111EB
	return x ^ (x >> 31)
}
