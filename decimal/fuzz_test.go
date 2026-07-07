package decimal

import (
	"testing"
)

// FuzzDecimalArithmetic fuzzes two decimal strings: parsing them and then
// exercising Add/Sub/Mul/Div must never panic. Parse errors (unparseable
// input, fractional digits exceeding the fixed scale) are skipped — those
// branches are covered by unit tests; the fuzz target here is that valid
// inputs never crash the arithmetic core. A single fixed scale is used so
// Add/Sub/Cmp succeed; scale-mismatch is itself unit-tested elsewhere.
func FuzzDecimalArithmetic(f *testing.F) {
	// Seed corpus: typical financial-looking decimal strings.
	seeds := []string{"0", "1", "-1", "12.34", "-0.05", "0.00", "100", "9999999999.99"}
	for _, a := range seeds {
		for _, b := range seeds {
			f.Add(a, b)
		}
	}

	const scale = 2

	f.Fuzz(func(t *testing.T, a, b string) {
		da, err := Parse(a, scale)
		if err != nil {
			t.Skip() // unparseable at this scale — covered by unit tests
		}
		db, err := Parse(b, scale)
		if err != nil {
			t.Skip()
		}

		// Add and Sub share the scale, so they must succeed and not panic.
		if _, err := da.Add(db); err != nil {
			t.Fatalf("Add(%q, %q) error: %v", a, b, err)
		}
		if _, err := da.Sub(db); err != nil {
			t.Fatalf("Sub(%q, %q) error: %v", a, b, err)
		}

		// MulDecimal (two-decimal multiply) must not panic regardless of input.
		_ = da.MulDecimal(db)

		// Div divides by an int64 divisor. Guard zero — the implementation
		// returns an error rather than panicking, but we skip zero to keep the
		// target focused on the non-error arithmetic path. Negative divisors
		// and large unscaled values exercise Quo truncation toward zero.
		divisor := db.Unscaled().Int64()
		if divisor == 0 {
			t.Skip()
		}
		if _, err := da.Div(divisor); err != nil {
			t.Fatalf("Div(%q, %d) error: %v", a, divisor, err)
		}
	})
}

// FuzzDecimalCmp fuzzes two decimal strings and asserts that Cmp never panics
// and returns a value consistent with the sign of (a - b): the sign of the
// numeric difference must agree with the comparison result.
func FuzzDecimalCmp(f *testing.F) {
	seeds := []string{"0", "1", "-1", "12.34", "-0.05", "0.00", "100", "9999999999.99"}
	for _, a := range seeds {
		for _, b := range seeds {
			f.Add(a, b)
		}
	}

	const scale = 2

	f.Fuzz(func(t *testing.T, a, b string) {
		da, err := Parse(a, scale)
		if err != nil {
			t.Skip()
		}
		db, err := Parse(b, scale)
		if err != nil {
			t.Skip()
		}

		cmp, err := da.Cmp(db)
		if err != nil {
			t.Fatalf("Cmp(%q, %q) error: %v", a, b, err)
		}

		// diff = a - b; both share the scale so Sub cannot error here.
		diff, err := da.Sub(db)
		if err != nil {
			t.Fatalf("Sub(%q, %q) error: %v", a, b, err)
		}
		sign := diff.Sign()

		// Cmp must agree with sign(diff) in classification (<0, 0, >0).
		if (cmp < 0) != (sign < 0) || (cmp > 0) != (sign > 0) {
			t.Fatalf("Cmp(%q, %q) = %d but sign(a-b) = %d", a, b, cmp, sign)
		}
	})
}
