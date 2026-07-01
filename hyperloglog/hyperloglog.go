// Package hyperloglog estimates the number of distinct elements in a stream
// (cardinality) using a fixed, small amount of memory — the HyperLogLog
// algorithm.
//
// At the default precision 14 it uses ~16 KB of registers and estimates
// cardinality to ~0.8% relative error regardless of how many millions of
// elements are added. Estimates are probabilistic and slightly low-variance;
// duplicates do not move the estimate. Pure standard library.
//
// Ad-tech uses: unique-user / unique-auction / audience-size counts over streams
// where storing every ID is infeasible (a few bytes of register per ~10K
// distinct, vs the full ID each). Pair with bloom (membership) where you need a
// yes/no on a specific element.
package hyperloglog

import (
	"errors"
	"hash/fnv"
	"math"
	"math/bits"
)

// Precision bounds: 4..16 (m = 2^p registers; p=14 → m=16384, ~0.8% error).
const (
	MinPrecision = 4
	MaxPrecision = 16
)

// ErrPrecision is returned for precision outside [MinPrecision, MaxPrecision].
var ErrPrecision = errors.New("hyperloglog: precision must be in [4, 16]")

// ErrIncompatible is returned by Merge for differing precision.
var ErrIncompatible = errors.New("hyperloglog: incompatible precision")

// HyperLogLog is a cardinality estimator. Add is NOT internally synchronized
// (it is the hot path); Estimate and Merge are safe when no Add is in flight.
// For concurrent producers, give each a per-shard HyperLogLog and Merge them —
// the algorithm is designed for that (Merge takes the per-register max).
type HyperLogLog struct {
	p   uint8
	m   uint32
	reg []uint8 // m registers
}

// HashFunc produces a 64-bit hash. The default (used by Add) is FNV-1a 64
// finalized with a splitmix64 mix to decorrelate bits — pure stdlib, good
// enough for HLL's accuracy targets.
type HashFunc func(data []byte) uint64

// DefaultHash is FNV-1a 64 + splitmix64 finalizer.
//
// Benchmarked at 0 allocs/op on Go 1.26+: the hasher from fnv.New64a() is
// stack-allocated by escape analysis and never escapes, so no manual inlining or
// pooling is warranted (a hand-rolled inline FNV measured no faster and removed
// the stdlib's compiler optimisations — reverted).
func DefaultHash(data []byte) uint64 {
	h := fnv.New64a()
	_, _ = h.Write(data)
	return splitmix64(h.Sum64())
}

// splitmix64 mixes a 64-bit value for better bit distribution (xorshift-mix).
func splitmix64(x uint64) uint64 {
	x += 0x9E3779B97F4A7C15
	x = (x ^ (x >> 30)) * 0xBF58476D1CE4E5B9
	x = (x ^ (x >> 27)) * 0x94D049BB133111EB
	return x ^ (x >> 31)
}

// New builds a HyperLogLog with the given precision (register count = 2^p).
// Higher precision = lower error (~1.04/sqrt(2^p)) and more memory. Default
// choice: 14.
func New(precision uint8) (*HyperLogLog, error) {
	if precision < MinPrecision || precision > MaxPrecision {
		return nil, ErrPrecision
	}
	return &HyperLogLog{
		p:   precision,
		m:   1 << precision,
		reg: make([]uint8, 1<<precision),
	}, nil
}

// Add records data using DefaultHash.
func (h *HyperLogLog) Add(data []byte) { h.AddHashed(DefaultHash(data)) }

// AddString is a string convenience for Add.
func (h *HyperLogLog) AddString(s string) { h.Add([]byte(s)) }

// AddHashed records an element given its precomputed 64-bit hash. Use this when
// you already hash the element (e.g. a shared hash) to avoid re-hashing.
func (h *HyperLogLog) AddHashed(x uint64) {
	j := x >> (64 - h.p) // top p bits -> register index
	// Position of the leftmost 1 in the remaining (64-p) bits, +1.
	rho := uint8(bits.LeadingZeros64(x<<h.p)) + 1
	if max := uint8(64 - h.p + 1); rho > max {
		rho = max
	}
	if rho > h.reg[j] {
		h.reg[j] = rho
	}
}

// Estimate returns the approximate distinct count.
func (h *HyperLogLog) Estimate() float64 {
	m := float64(h.m)
	sum := 0.0
	zeros := 0
	for _, r := range h.reg {
		sum += math.Pow(2, -float64(r))
		if r == 0 {
			zeros++
		}
	}
	est := alpha(m) * m * m / sum
	// Small-range correction: linear counting when the estimate is low and some
	// registers are still empty (small cardinalities are biased high by raw HLL).
	if est <= 2.5*m && zeros > 0 {
		return m * math.Log(m/float64(zeros))
	}
	// Large-range correction for cardinalities approaching 2^32.
	if est > (1.0/30.0)*(1<<32) {
		return -float64(1<<32) * math.Log(1-est/float64(1<<32))
	}
	return est
}

// Precision returns the configured precision.
func (h *HyperLogLog) Precision() uint8 { return h.p }

// Reset clears all registers.
func (h *HyperLogLog) Reset() {
	for i := range h.reg {
		h.reg[i] = 0
	}
}

// Merge unions another sketch into this one (takes the per-register max).
// Requires identical precision.
func (h *HyperLogLog) Merge(other *HyperLogLog) error {
	if h.p != other.p {
		return ErrIncompatible
	}
	for i, r := range other.reg {
		if r > h.reg[i] {
			h.reg[i] = r
		}
	}
	return nil
}

// alpha is the HLL bias-correction constant for register count m.
func alpha(m float64) float64 {
	switch m {
	case 16:
		return 0.673
	case 32:
		return 0.697
	case 64:
		return 0.709
	default:
		return 0.7213 / (1 + 1.079/m)
	}
}
