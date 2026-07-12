package str_test

import (
	"testing"

	"github.com/v8fg/kit4go/str"
)

// FuzzCamelToSnake encodes CamelToSnake's invariants on arbitrary input: it must
// not panic, must be deterministic (same input → same output), and its output is
// snake_case (no ASCII uppercase — every camel word-boundary becomes a lowercase
// segment). The manual-loop regression that turned "URLPath" into "ur_lpath"
// broke delimiter placement; this target guards the determinism + lowercase
// contract broadly (the specific url_path case has its own table test). E10
// invariant-encoding fuzz target.
func FuzzCamelToSnake(f *testing.F) {
	f.Add("URLPath")
	f.Add("getUserID")
	f.Add("")
	f.Add("A")
	f.Add("already_snake")
	f.Add("HTTPServer2")
	f.Add("MixedCASEString")
	f.Fuzz(func(t *testing.T, s string) {
		out := str.CamelToSnake(s)
		// Deterministic.
		if str.CamelToSnake(s) != out {
			t.Errorf("CamelToSnake(%q) not deterministic", s)
		}
		// Output is snake_case: no ASCII uppercase survives.
		for _, r := range out {
			if r >= 'A' && r <= 'Z' {
				t.Errorf("CamelToSnake(%q) = %q contains uppercase %q (not snake_lower)", s, out, r)
				break
			}
		}
	})
}
