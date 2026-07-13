package syncutil_test

import (
	"context"
	"fmt"

	"github.com/v8fg/kit4go/syncutil"
)

func ExampleOrDone() {
	ctx := context.Background()
	ch := make(chan string, 2)
	ch <- "hello"
	ch <- "world"
	close(ch)

	for v := range syncutil.OrDone(ctx, ch) {
		fmt.Println(v)
	}

	// Output:
	// hello
	// world
}

func ExamplePromise() {
	p := syncutil.NewPromise[int]()
	go func() {
		// ... do some work ...
		p.Set(42)
	}()

	v, _ := p.Get(context.Background())
	fmt.Println("result:", v)

	// Output:
	// result: 42
}
