// Package bloom implements a classic Bloom filter: a space-efficient,
// probabilistic set membership test with one-sided error.
//
// A negative answer is certain (the element was definitely never added); a
// positive answer is probable (the element was likely added, with a tunable
// false-positive rate). The filter uses the Kirsch-Mitzenmacher double-hashing
// scheme (two base hashes derive all k indices), so it needs no hash-function
// slice and stays cache-friendly.
//
// Ad-tech uses: per-user / per-auction dedup at scale (a few bytes per element
// instead of the full key), bot / repeat-impression suppression, and
// "have I already bid on this" guards where a rare false positive is acceptable
// (it just means treating a new item as seen).
package bloom

import (
	"errors"
	"hash/fnv"
	"math"
	"sync"
)

// Filter is a Bloom filter. Safe for concurrent use.
type Filter struct {
	mu    sync.RWMutex
	bits  []uint64  // packed bit array (len = ceil(m/64))
	m     uint64    // number of bits
	k     uint64    // number of hash functions
	n     uint64    // number of Add calls
	ipool sync.Pool // recycles the k-length index slice so the hot path is 0-alloc
}

// New builds a filter sized for expectedN elements at the desired false-positive
// rate fp (0 < fp < 1). It computes m (bits) and k (hashes) from the standard
// formulas:
//
//	m = ceil(-n * ln(fp) / (ln2)^2)
//	k = max(1, round((m/n) * ln2))
//
// Panics if fp is out of range or expectedN is non-positive.
func New(expectedN int, fp float64) *Filter {
	if expectedN <= 0 {
		panic("bloom: expectedN must be > 0")
	}
	if fp <= 0 || fp >= 1 {
		panic("bloom: false-positive rate must be in (0,1)")
	}
	n := float64(expectedN)
	m := math.Ceil(-n * math.Log(fp) / (math.Ln2 * math.Ln2))
	k := math.Round((m / n) * math.Ln2)
	if k < 1 {
		k = 1
	}
	return NewFromParams(uint(m), uint(k))
}

// NewFromParams builds a filter with an explicit bit count m and hash count k.
// Use this when you control sizing directly (e.g. a fixed memory budget).
func NewFromParams(m, k uint) *Filter {
	if m == 0 {
		panic("bloom: m must be > 0")
	}
	if k == 0 {
		k = 1
	}
	words := (m + 63) / 64
	k64 := uint64(k)
	f := &Filter{
		bits: make([]uint64, words),
		m:    uint64(m),
		k:    k64,
	}
	// Pool the index slice so Add/Test/TestAndAdd allocate zero on the hot
	// path. Each Filter owns its pool; k is fixed per Filter, so every Get
	// returns a slice already sized to k and no cross-Filter reslicing is
	// needed.
	//
	// The pool stores *[]uint64, not []uint64, deliberately: a slice does not
	// fit in an interface word, so storing []uint64 would heap-allocate a
	// 24-byte header copy on every Put. A pointer fits in a word and boxes
	// for free, which is what makes the hot path truly 0-alloc.
	f.ipool.New = func() any {
		s := make([]uint64, k64)
		return &s
	}
	return f
}

// Add inserts data into the filter.
func (f *Filter) Add(data []byte) {
	idxp := f.indices(data)
	f.mu.Lock()
	f.addLocked(*idxp)
	f.mu.Unlock()
	f.ipool.Put(idxp)
}

// AddString is a string convenience wrapper around Add.
func (f *Filter) AddString(s string) { f.Add([]byte(s)) }

// Test reports whether data may be in the filter. A false result is certain
// (data was never added); a true result is probable.
func (f *Filter) Test(data []byte) bool {
	idxp := f.indices(data)
	f.mu.RLock()
	present := f.testLocked(*idxp)
	f.mu.RUnlock()
	f.ipool.Put(idxp)
	return present
}

// TestString is a string convenience wrapper around Test.
func (f *Filter) TestString(s string) bool { return f.Test([]byte(s)) }

// TestAndAdd reports the pre-add Test result, then inserts data. Useful for
// "return whether this is a duplicate, and record it" in one call.
func (f *Filter) TestAndAdd(data []byte) bool {
	idxp := f.indices(data)
	f.mu.Lock()
	present := f.testLocked(*idxp)
	f.addLocked(*idxp)
	f.mu.Unlock()
	f.ipool.Put(idxp)
	return present
}

// testLocked reports membership assuming a held lock (read or write).
func (f *Filter) testLocked(idx []uint64) bool {
	for _, i := range idx {
		if f.bits[i>>6]&(1<<(i&63)) == 0 {
			return false
		}
	}
	return true
}

func (f *Filter) addLocked(idx []uint64) {
	for _, i := range idx {
		f.bits[i>>6] |= 1 << (i & 63)
	}
	f.n++
}

// indices derives the k bit indices for data via double hashing.
// g_i = (h1 + i*h2) mod m, with h2 forced non-zero so all k indices can differ.
//
// It returns a pointer to a slice borrowed from f.ipool. The caller MUST
// return the pointer via f.ipool.Put once done; never alias the slice beyond
// that point. Because k is fixed per Filter, every pooled slice is already
// exactly len k, so no reslicing or bounds fixing is required.
//
// The pool stores *[]uint64 (not []uint64): a slice does not fit in an
// interface word and would heap-allocate a 24-byte header copy on every Put,
// whereas a pointer boxes for free — which is what keeps the hot path 0-alloc.
func (f *Filter) indices(data []byte) *[]uint64 {
	h1, h2 := doubleHash(data)
	if h2 == 0 {
		h2 = 1
	}
	out := f.ipool.Get().(*[]uint64)
	for i := uint64(0); i < f.k; i++ {
		(*out)[i] = (h1 + i*h2) % f.m
	}
	return out
}

// doubleHash returns two 64-bit hashes of data using FNV-1a and FNV-1.
func doubleHash(data []byte) (uint64, uint64) {
	a := fnv.New64a()
	_, _ = a.Write(data)
	b := fnv.New64()
	_, _ = b.Write(data)
	return a.Sum64(), b.Sum64()
}

// N returns the number of Add/TestAndAdd calls (the approximate set size).
func (f *Filter) N() uint64 {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.n
}

// M returns the bit count. K returns the hash count.
func (f *Filter) M() uint64 { return f.m }

// K returns the hash count.
func (f *Filter) K() uint64 { return f.k }

// EstimatedFalsePositiveRate returns the current FPR given n items inserted:
// p ≈ (1 - e^(-kn/m))^k.
func (f *Filter) EstimatedFalsePositiveRate(n int) float64 {
	if n <= 0 {
		return 0
	}
	exponent := -float64(f.k) * float64(n) / float64(f.m)
	return math.Pow(1-math.Exp(exponent), float64(f.k))
}

// ErrIncompatible is returned by Merge when the filters have different M or K.
var ErrIncompatible = errors.New("bloom: filters have incompatible m or k")

// Merge unions another filter into this one (both must have the same m and k).
// Counts are summed.
func (f *Filter) Merge(other *Filter) error {
	if f.m != other.m || f.k != other.k {
		return ErrIncompatible
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	other.mu.RLock()
	defer other.mu.RUnlock()
	for i := range f.bits {
		f.bits[i] |= other.bits[i]
	}
	f.n += other.n
	return nil
}

// Reset clears all bits and resets the counter.
func (f *Filter) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.bits {
		f.bits[i] = 0
	}
	f.n = 0
}
