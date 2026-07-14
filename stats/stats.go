// Package stats provides basic descriptive statistics over float64 slices:
// Mean, Median, Mode, Variance, StdDev, Percentile, Min, Max, Sum, Range.
//
// All functions return NaN for empty input, except Sum which returns 0 (the
// additive identity over no elements). Pure standard library.
package stats

import (
	"math"
	"slices"
)

// Sum returns the sum of all values.
func Sum(s []float64) float64 {
	var sum float64
	for _, v := range s {
		sum += v
	}
	return sum
}

// Mean returns the arithmetic mean. NaN if empty.
func Mean(s []float64) float64 {
	if len(s) == 0 {
		return math.NaN()
	}
	return Sum(s) / float64(len(s))
}

// Median returns the middle value (or average of the two middle values). NaN if
// empty. Does not modify the input (sorts a copy).
func Median(s []float64) float64 {
	n := len(s)
	if n == 0 {
		return math.NaN()
	}
	sorted := slices.Clone(s)
	slices.Sort(sorted)
	mid := n / 2
	if n%2 == 0 {
		return (sorted[mid-1] + sorted[mid]) / 2
	}
	return sorted[mid]
}

// Mode returns the most frequent value. If there are ties, the smallest value
// among the modes is returned. NaN if empty.
func Mode(s []float64) float64 {
	if len(s) == 0 {
		return math.NaN()
	}
	freq := make(map[float64]int)
	for _, v := range s {
		freq[v]++
	}
	bestVal := s[0]
	bestCount := 0
	for v, c := range freq {
		if c > bestCount || (c == bestCount && v < bestVal) {
			bestVal = v
			bestCount = c
		}
	}
	return bestVal
}

// Variance returns the population variance (divide by N). NaN if empty.
func Variance(s []float64) float64 {
	n := len(s)
	if n == 0 {
		return math.NaN()
	}
	m := Mean(s)
	var sumSq float64
	for _, v := range s {
		d := v - m
		sumSq += d * d
	}
	return sumSq / float64(n)
}

// StdDev returns the population standard deviation. NaN if empty.
func StdDev(s []float64) float64 {
	v := Variance(s)
	if math.IsNaN(v) {
		return v
	}
	return math.Sqrt(v)
}

// Percentile returns the p-th percentile (0 <= p <= 100) using linear
// interpolation. NaN if empty or p out of range.
func Percentile(s []float64, p float64) float64 {
	if len(s) == 0 || p < 0 || p > 100 {
		return math.NaN()
	}
	sorted := slices.Clone(s)
	slices.Sort(sorted)
	if len(sorted) == 1 {
		return sorted[0]
	}
	rank := p / 100 * float64(len(sorted)-1)
	lo := int(math.Floor(rank))
	hi := int(math.Ceil(rank))
	if lo == hi {
		return sorted[lo]
	}
	frac := rank - float64(lo)
	return sorted[lo] + frac*(sorted[hi]-sorted[lo])
}

// Min returns the minimum value. NaN if empty.
func Min(s []float64) float64 {
	if len(s) == 0 {
		return math.NaN()
	}
	m := s[0]
	for _, v := range s[1:] {
		if v < m {
			m = v
		}
	}
	return m
}

// Max returns the maximum value. NaN if empty.
func Max(s []float64) float64 {
	if len(s) == 0 {
		return math.NaN()
	}
	m := s[0]
	for _, v := range s[1:] {
		if v > m {
			m = v
		}
	}
	return m
}

// Range returns Max - Min. NaN if empty.
func Range(s []float64) float64 {
	if len(s) == 0 {
		return math.NaN()
	}
	return Max(s) - Min(s)
}
