package bloom_test

import (
	"fmt"

	"github.com/v8fg/kit4go/bloom"
)

func ExampleFilter() {
	f := bloom.New(1000, 0.01) // ~1000 expected items, 1% false-positive rate
	f.AddString("user-42")
	fmt.Println(f.TestString("user-42")) // always true — bloom has no false negatives
	// Output: true
}
