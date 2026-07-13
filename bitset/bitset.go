// Package bitset provides a compact set of non-negative integers using a bit
// array — 1/64 the memory of a map-based set for small integer ranges. O(1)
// Set/Clear/Test, O(n/64) for iteration.
//
// Pure standard library. Ad-tech uses: flag bitmasks, small-ID membership
// (creative IDs 0..10K), deduplication of bounded integer spaces, bloom-filter
// building blocks.
package bitset

import "math/bits"

// BitSet is a compact set of non-negative integers.
type BitSet struct {
	buf []uint64
	n   int // number of bits (max value + 1, grows as needed)
}

// New creates a BitSet with an initial capacity of n bits.
func New(n int) *BitSet {
	if n <= 0 {
		n = 64
	}
	return &BitSet{buf: make([]uint64, (n+63)/64), n: n}
}

func (b *BitSet) grow(i int) {
	wordsNeeded := (i + 64) / 64
	if wordsNeeded <= len(b.buf) {
		if i >= b.n {
			b.n = i + 1
		}
		return
	}
	newBuf := make([]uint64, wordsNeeded)
	copy(newBuf, b.buf)
	b.buf = newBuf
	b.n = i + 1
}

// Set marks bit i as present. Grows the buffer if i exceeds the current capacity.
func (b *BitSet) Set(i int) {
	if i < 0 {
		panic("bitset: negative index")
	}
	b.grow(i)
	b.buf[i/64] |= 1 << uint(i%64)
}

// Clear removes bit i.
func (b *BitSet) Clear(i int) {
	if i < 0 || i >= b.n {
		return
	}
	b.buf[i/64] &^= 1 << uint(i%64)
}

// Test reports whether bit i is present.
func (b *BitSet) Test(i int) bool {
	if i < 0 || i >= b.n {
		return false
	}
	return b.buf[i/64]&(1<<uint(i%64)) != 0
}

// Len returns the number of set bits.
func (b *BitSet) Len() int {
	count := 0
	for _, w := range b.buf {
		count += bits.OnesCount64(w)
	}
	return count
}

// IsEmpty reports whether no bits are set.
func (b *BitSet) IsEmpty() bool {
	for _, w := range b.buf {
		if w != 0 {
			return false
		}
	}
	return true
}

// ClearAll removes all bits.
func (b *BitSet) ClearAll() {
	clear(b.buf)
}

// Union sets all bits that are in other.
func (b *BitSet) Union(other *BitSet) {
	if other == nil {
		return
	}
	if len(other.buf) > len(b.buf) {
		b.grow(other.n - 1)
	}
	for i, w := range other.buf {
		if i < len(b.buf) {
			b.buf[i] |= w
		}
	}
}

// Intersect keeps only bits that are in both.
func (b *BitSet) Intersect(other *BitSet) {
	if other == nil {
		b.ClearAll()
		return
	}
	for i := range b.buf {
		if i < len(other.buf) {
			b.buf[i] &= other.buf[i]
		} else {
			b.buf[i] = 0
		}
	}
}

// ToSlice returns all set bit indices in ascending order.
func (b *BitSet) ToSlice() []int {
	var out []int
	for wordIdx, w := range b.buf {
		for w != 0 {
			t := bits.TrailingZeros64(w)
			out = append(out, wordIdx*64+int(t))
			w &= w - 1 // clear lowest set bit
		}
	}
	return out
}
