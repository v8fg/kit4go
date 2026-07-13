package syncutil_test

import (
	"context"
	"testing"

	"github.com/v8fg/kit4go/syncutil"
)

func BenchmarkOrDone(b *testing.B) {
	ctx := context.Background()
	src := make(chan int, 1)
	src <- 1
	b.ResetTimer()
	for b.Loop() {
		<-syncutil.OrDone(ctx, src)
		src <- 1 // refill
	}
}

func BenchmarkPromiseGet(b *testing.B) {
	p := syncutil.NewPromise[int]()
	p.Set(42)
	ctx := context.Background()
	b.ResetTimer()
	for b.Loop() {
		p.Get(ctx)
	}
}
