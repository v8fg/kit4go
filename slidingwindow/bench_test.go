package slidingwindow_test

import (
	"testing"
	"time"

	"github.com/v8fg/kit4go/slidingwindow"
)

func BenchmarkWindowPush(b *testing.B) {
	w := slidingwindow.New(1000)
	b.ResetTimer()
	for b.Loop() {
		w.Push(1.5)
	}
}

func BenchmarkWindowSum(b *testing.B) {
	w := slidingwindow.New(1000)
	for i := range 1000 {
		w.Push(float64(i))
	}
	b.ResetTimer()
	for b.Loop() {
		w.Sum()
	}
}

func BenchmarkTimeWindowPush(b *testing.B) {
	tw := slidingwindow.NewTimeWindow(60*time.Second, 10000)
	base := time.Unix(1000, 0)
	b.ResetTimer()
	for b.Loop() {
		tw.Push(1.0, base)
	}
}
