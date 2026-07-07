package batcher_test

import (
	"fmt"
	"time"

	"github.com/v8fg/kit4go/batcher"
)

// ExampleBatcher coalesces items and flushes them via a caller-supplied
// callback. With the time trigger disabled (interval <= 0), a batch flushes
// either when maxSize items accumulate or Flush/Close is called. Flush is
// synchronous, so the example output is deterministic.
func ExampleBatcher() {
	var all [][]int
	flush := func(batch []int) {
		// Copy: the batcher reuses its buffer.
		cp := append([]int(nil), batch...)
		all = append(all, cp)
	}

	// Flush at 3 items; no time trigger (interval <= 0 disables it).
	b := batcher.New(3, -1*time.Second, flush)
	b.Add(1)
	b.Add(2)
	b.Add(3) // reaches maxSize -> flushes [1 2 3]
	b.Add(4)
	b.Flush() // flushes the buffered remainder [4]
	b.Close()

	fmt.Println(all)
	// Output: [[1 2 3] [4]]
}
