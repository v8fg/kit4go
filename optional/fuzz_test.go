package optional_test

import (
	"testing"

	"github.com/v8fg/kit4go/optional"
)

// FuzzMapIdentity encodes the invariant: Map(Some(v), f) == Some(f(v)), and
// Map(None[T](), f) == None[U](). E10 invariant-encoding fuzz target.
func FuzzMapIdentity(f *testing.F) {
	f.Add(42)
	f.Add(0)
	f.Add(-1)
	f.Fuzz(func(t *testing.T, v int) {
		// Some → Map → Some(f(v))
		s := optional.Some(v)
		mapped := optional.Map(s, func(x int) int { return x + 1 })
		if !mapped.IsSome() {
			t.Fatalf("Map(Some(%d)) returned None", v)
		}
		if got := mapped.Unwrap(); got != v+1 {
			t.Errorf("Map(Some(%d), x+1) = Some(%d), want Some(%d)", v, got, v+1)
		}
		// None → Map → None
		n := optional.None[int]()
		mappedNone := optional.Map(n, func(x int) int { return x + 1 })
		if !mappedNone.IsNone() {
			t.Errorf("Map(None) returned Some")
		}
	})
}
