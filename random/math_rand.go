// Package random, the file refs the math/rand, it implements pseudo-random number generators unsuitable for
// security-sensitive work.
package random

import (
	"encoding/base64"
	"math/rand"
	"time"
)

// Init localRand once will work well for you, if you need init multiple times, pls careful call the Seed function.
var localRand = rand.New(rand.NewSource(time.Now().UnixNano()))

// Seed uses the provided seed value to initialize the generator to a deterministic state.
// Seed should not be called concurrently with any other Rand method.
func Seed(seed int64) {
	localRand.Seed(seed)
}

// SeedReset reset the rand seed with the unix nanoseconds timestamp.
// SeedReset should not be called concurrently with any other Rand method.
func SeedReset() {
	localRand.Seed(time.Now().UnixNano())
}

// Float32 returns, as a float32, a pseudo-random number in the half-open interval [0.0,1.0)
// from the default Source.
func Float32() float32 { return localRand.Float32() }

// Float32Between returns, as a float32, a pseudo-random number in the half-open interval [min,max)
// from the default Source.
func Float32Between(min, max float32) float32 { return localRand.Float32()*(max-min) + min }

// Float64 returns, as a float64, a pseudo-random number in the half-open interval [0.0,1.0)
// from the default Source.
func Float64() float64 { return localRand.Float64() }

// Float64Between returns, as a float64, a pseudo-random number in the half-open interval [min,max)
// from the default Source.
func Float64Between(min, max float64) float64 { return localRand.Float64()*(max-min) + min }

// Int returns a non-negative pseudo-random int from the default Source.
func Int() int { return localRand.Int() }

// IntBetween returns a non-negative pseudo-random int in the half-open interval [min,max)
// from the default Source.
// It panics if max-min <= 0.
func IntBetween(min, max int) int { return localRand.Intn(max-min) + min }

// Int31 returns a non-negative pseudo-random 31-bit integer as an int32 from the default Source.
func Int31() int32 { return localRand.Int31() }

// Int31Between returns a non-negative pseudo-random 31-bit integer as an int32, in the half-open interval [min,max)
// from the default Source.
// It panics if max-min <= 0.
func Int31Between(min, max int32) int32 { return localRand.Int31n(max-min) + min }

// Int63 returns a non-negative pseudo-random 31-bit integer as an int64 from the default Source.
func Int63() int32 { return localRand.Int31() }

// Int63Between returns a non-negative pseudo-random 63-bit integer as an int64, in the half-open interval [min,max)
// from the default Source.
// It panics if max-min <= 0.
func Int63Between(min, max int64) int64 { return localRand.Int63n(max-min) + min }

// Uint32 returns a pseudo-random 32-bit value as a uint32 from the default Source.
func Uint32() uint32 { return localRand.Uint32() }

// Uint64 returns a pseudo-random 64-bit value as a uint64 from the default Source.
func Uint64() uint64 { return localRand.Uint64() }

// Perm returns, as a slice of n ints, a pseudo-random permutation of the integers
// in the half-open interval [0,n).
func Perm(n int) []int { return localRand.Perm(n) }

// PermBetween returns, as a slice of n ints, a pseudo-random permutation of the integers
// in the half-open interval [min,max).
func PermBetween(min, max int) []int {
	n := max - min
	if n <= 0 {
		return []int{}
	}

	m := make([]int, n)
	for i := 0; i < n; i++ {
		j := localRand.Intn(i + 1)
		m[i] = m[j]
		m[j] = i + min
	}
	return m
}

// Shuffle pseudo-randomizes the order of elements.
// n is the number of elements. Shuffle panics if n < 0.
// swap swaps the elements with indexes i and j.
func Shuffle(n int, swap func(i, j int)) { localRand.Shuffle(n, swap) }

// Read generates len(p) random bytes and writes them into p. It
// always returns len(p) and a nil error.
// Read should not be called concurrently with any other Rand method.
func Read(p []byte) (n int, err error) { return localRand.Read(p) }

// NormFloat64 returns a normally distributed float64 in
// the range -math.MaxFloat64 through +math.MaxFloat64 inclusive,
// with standard normal distribution (mean = 0, stddev = 1).
// To produce a different normal distribution, callers can
// adjust the output using:
//
//	sample = NormFloat64() * desiredStdDev + desiredMean
func NormFloat64() float64 { return localRand.NormFloat64() }

// ExpFloat64 returns an exponentially distributed float64 in the range
// (0, +math.MaxFloat64] with an exponential distribution whose rate parameter
// (lambda) is 1 and whose mean is 1/lambda (1) from the default Source.
// To produce a distribution with a different rate parameter,
// callers can adjust the output using:
//
//	sample = ExpFloat64() / desiredRateParameter
func ExpFloat64() float64 { return localRand.ExpFloat64() }

// StringByRead returns the random string with the length == len(b).
func StringByRead(b []byte) string {
	_, _ = rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

// Percent returns a non-negative pseudo-random percent number in the half-open interval [0,100.0)
func Percent() float64 {
	ret := Float64Between(0, 101)
	if ret >= 100 {
		ret = 100.0
	}
	return ret
}
