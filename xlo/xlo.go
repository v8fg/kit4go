package xlo

import (
	"github.com/samber/lo"
	lop "github.com/samber/lo/parallel"
)

// Maximum average load of a bucket that triggers growth is 6.5.
// Represent as loadFactorNum/loadFactorDen, to allow integer math.
// loadFactorDen = 2
// defined in runtime./map.go
const loadFactorNum = 13

// Uniq returns a duplicate-free version of an array, in which only the first occurrence of each element is kept.
// The order of result values is determined by the order they occur in the array.
//
// Some optimizations for small capacity temp map, if capacity less than loadFactorNum.
func Uniq[T comparable](collection []T) []T {
	size := len(collection)
	result := make([]T, 0, size)
	var temp map[T]struct{}
	if size < loadFactorNum {
		temp = map[T]struct{}{}
	} else {
		temp = make(map[T]struct{}, size)
	}

	for _, item := range collection {
		if _, ok := temp[item]; !ok {
			temp[item] = struct{}{}
			result = append(result, item)
		}
	}
	return result
}

// LoUniq returns a duplicate-free version of an array, in which only the first occurrence of each element is kept.
// The order of result values is determined by the order they occur in the array.
func LoUniq[T comparable](collection []T) []T {
	return lo.Uniq(collection)
}

// LoMap manipulates a slice and transforms it to a slice of another type.
func LoMap[T any, R any](collection []T, iteratee func(item T, index int) R) []R {
	return lo.Map(collection, iteratee)
}

// LopMap manipulates a slice and transforms it to a slice of another type.
// `iteratee` is call in parallel. Result keep the same order.
func LopMap[T any, R any](collection []T, iteratee func(item T, index int) R) []R {
	return lop.Map(collection, iteratee)
}
