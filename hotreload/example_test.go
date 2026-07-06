package hotreload_test

import (
	"fmt"
	"sync/atomic"

	"github.com/v8fg/kit4go/hotreload"
)

// countLoader is a deterministic Loader for the example: each Load returns the
// next integer (boxed in a value type). It stands in for any real source — a
// config file, a feature-flag service, a remote endpoint — without I/O.
type countLoader struct {
	n atomic.Int64
}

func (c *countLoader) Load() (int, error) {
	return int(c.n.Add(1)), nil
}

// ExampleNew shows a hot-reload buffer built from a mock loader. The initial
// Load populates the buffer (Get returns 1); Reload atomically swaps in the
// next value (Get returns 2). Readers never block on a reload.
func ExampleNew() {
	b, err := hotreload.New[int](&countLoader{})
	if err != nil {
		fmt.Println("init error:", err)
		return
	}
	fmt.Println("initial:", b.Get()) // populated by New's first Load

	if err := b.Reload(); err != nil {
		fmt.Println("reload error:", err)
		return
	}
	fmt.Println("reloaded:", b.Get()) // atomically swapped in

	// Output:
	// initial: 1
	// reloaded: 2
}
