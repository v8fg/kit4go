package objpool_test

import (
	"bytes"
	"fmt"

	"github.com/v8fg/kit4go/objpool"
)

// ExampleNew shows a *bytes.Buffer pool with a reset hook. The reset hook
// guarantees every Get returns a clean buffer, so callers never see leftover
// bytes from a previous use.
func ExampleNew() {
	pool := objpool.New(
		func() *bytes.Buffer { return new(bytes.Buffer) },
		objpool.WithReset(func(b *bytes.Buffer) { b.Reset() }),
	)

	b := pool.Get()
	b.WriteString("hello")
	pool.Put(b) // return the (now "dirty") buffer for reuse

	b2 := pool.Get() // reset hook has cleared it
	b2.WriteString("world")
	fmt.Println(b2.String())

	s := pool.Stats()
	fmt.Printf("gets=%d puts=%d inUse=%d\n", s.Gets, s.Puts, s.InUse)

	// Output:
	// world
	// gets=2 puts=1 inUse=1
}
