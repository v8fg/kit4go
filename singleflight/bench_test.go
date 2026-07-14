package singleflight_test

import (
	"sync"
	"testing"

	"github.com/v8fg/kit4go/singleflight"
)

func BenchmarkDoUncontended(b *testing.B) {
	// Leader path: unique key each iteration → fn always runs (no sharing).
	g := singleflight.New[int, int]()
	b.ResetTimer()
	for b.Loop() {
		g.Do(b.N, func() (int, error) { return 1, nil })
	}
}

func BenchmarkDoContended(b *testing.B) {
	// 16 goroutines racing over a 4-key space → exercises leader + shared-wait
	// paths under real mutex contention per iteration.
	g := singleflight.New[int, int]()
	b.ResetTimer()
	for b.Loop() {
		var wg sync.WaitGroup
		for i := range 16 {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				g.Do(i%4, func() (int, error) { return i, nil })
			}(i)
		}
		wg.Wait()
	}
}
