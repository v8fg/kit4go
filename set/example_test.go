package set_test

import (
	"fmt"

	"github.com/v8fg/kit4go/set"
)

// ExampleNew demonstrates building a Set, checking membership, and computing the
// intersection of two sets.
func ExampleNew() {
	tags := set.New("holiday", "sale", "clearance")
	fmt.Println("has sale:", tags.Contains("sale"))
	fmt.Println("has food:", tags.Contains("food"))

	allowed := set.New("holiday", "food")
	both := set.Intersect(tags, allowed)
	fmt.Println("intersect:", both.Len())

	// Output:
	// has sale: true
	// has food: false
	// intersect: 1
}

// ExampleUnion demonstrates merging multiple sets.
func ExampleUnion() {
	a := set.New(1, 2, 3)
	b := set.New(3, 4, 5)
	fmt.Println("union len:", set.Union(a, b).Len())

	// Output:
	// union len: 5
}

// ExampleDifference demonstrates set subtraction.
func ExampleDifference() {
	enabled := set.New("read", "write", "delete")
	revoked := set.New("delete")
	active := set.Difference(enabled, revoked)
	fmt.Println("active contains delete:", active.Contains("delete"))
	fmt.Println("active contains read:", active.Contains("read"))

	// Output:
	// active contains delete: false
	// active contains read: true
}
