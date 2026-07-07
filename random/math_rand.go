// Package random, the file refs the math/rand, it implements pseudo-random number generators unsuitable for
// security-sensitive work.
//
// All functions in this file are safe for concurrent use — they delegate to
// the math/rand/v2 package-level functions, which use a concurrent-safe
// global source. (The previous implementation used a shared *rand.Rand which
// is NOT safe for concurrent use and could panic.)
package random

import (
	crand "crypto/rand"
	"encoding/base64"
	"math/rand/v2"
)

// Seed is a no-op retained for backward compatibility. The global source in
// math/rand/v2 is auto-seeded and does not support manual seeding.
func Seed(_ int64) {}

// SeedReset is a no-op retained for backward compatibility.
func SeedReset() {}

// Float32 returns, as a float32, a pseudo-random number in [0.0,1.0).
func Float32() float32 { return rand.Float32() }

// Float32Between returns a pseudo-random float32 in [min,max).
func Float32Between(min, max float32) float32 { return rand.Float32()*(max-min) + min }

// Float64 returns a pseudo-random float64 in [0.0,1.0).
func Float64() float64 { return rand.Float64() }

// Float64Between returns a pseudo-random float64 in [min,max).
func Float64Between(min, max float64) float64 { return rand.Float64()*(max-min) + min }

// Int returns a non-negative pseudo-random int.
func Int() int { return rand.Int() }

// IntBetween returns a non-negative pseudo-random int in [min,max).
// It panics if max-min <= 0.
func IntBetween(min, max int) int { return rand.IntN(max-min) + min }

// Int31 returns a non-negative pseudo-random 31-bit integer as an int32.
func Int31() int32 { return rand.Int32() }

// Int31Between returns a non-negative pseudo-random 31-bit integer as an int32, in [min,max).
// It panics if max-min <= 0.
func Int31Between(min, max int32) int32 { return rand.Int32N(max-min) + min }

// Int63 returns a non-negative pseudo-random 63-bit integer as an int64.
func Int63() int64 { return rand.Int64() }

// Int63Between returns a non-negative pseudo-random 63-bit integer as an int64, in [min,max).
// It panics if max-min <= 0.
func Int63Between(min, max int64) int64 { return rand.Int64N(max-min) + min }

// Uint32 returns a pseudo-random 32-bit value as a uint32.
func Uint32() uint32 { return rand.Uint32() }

// Uint64 returns a pseudo-random 64-bit value as a uint64.
func Uint64() uint64 { return rand.Uint64() }

// Perm returns, as a slice of n ints, a pseudo-random permutation of [0,n).
func Perm(n int) []int { return rand.Perm(n) }

// PermBetween returns a pseudo-random permutation of [min,max).
func PermBetween(min, max int) []int {
	n := max - min
	if n <= 0 {
		return []int{}
	}
	m := make([]int, n)
	for i := range n {
		j := rand.IntN(i + 1)
		m[i] = m[j]
		m[j] = i + min
	}
	return m
}

// Shuffle pseudo-randomizes the order of elements.
// n is the number of elements. Shuffle panics if n < 0.
// swap swaps the elements with indexes i and j.
func Shuffle(n int, swap func(i, j int)) { rand.Shuffle(n, swap) }

// Read generates len(p) pseudo-random bytes and writes them into p.
// It always returns len(p) and a nil error.
func Read(p []byte) (n int, err error) { return crand.Read(p) }

// NormFloat64 returns a normally distributed float64 with standard normal
// distribution (mean = 0, stddev = 1).
func NormFloat64() float64 { return rand.NormFloat64() }

// ExpFloat64 returns an exponentially distributed float64 with rate parameter 1.
func ExpFloat64() float64 { return rand.ExpFloat64() }

// StringByRead returns the random string with the length == len(b).
func StringByRead(b []byte) string {
	_, _ = crand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

// Percent returns a non-negative pseudo-random percent number in [0,100.0].
func Percent() float64 {
	ret := Float64Between(0, 101)
	if ret >= 100 {
		ret = 100.0
	}
	return ret
}
