package decimal

import (
	"errors"
	"math/big"
	"testing"
)

// newBigInt is a small test helper to build a *big.Int for struct-literal
// construction that bypasses New's scale guard.
func newBigInt(v int64) *big.Int { return big.NewInt(v) }

// r12_regression_test.go covers the four R12 findings. Each test targets an edge
// input that failed (panicked or returned the wrong type) on the pre-fix code,
// and asserts the correct post-fix behavior across negative/positive/mixed-sign
// inputs.

// --- F2: negative scale must not produce a value whose String() panics. ---

// TestNewNegativeScalePanics asserts New rejects a negative scale with a clear
// panic wrapping ErrNegativeScale (constructor idiom: New has no error return).
// On the OLD code New accepted scale<0 and the resulting String() panicked with
// a slice-out-of-range.
func TestNewNegativeScalePanics(t *testing.T) {
	cases := []struct {
		unscaled int64
		scale    int
	}{
		{123, -1},
		{-123, -1},
		{0, -5},
	}
	for _, tc := range cases {
		func() {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatalf("New(%d, %d): expected panic, got none", tc.unscaled, tc.scale)
				}
				if !errors.Is(r.(error), ErrNegativeScale) {
					t.Fatalf("New(%d, %d): panic = %v, want ErrNegativeScale", tc.unscaled, tc.scale, r)
				}
			}()
			_ = New(tc.unscaled, tc.scale)
		}()
	}
}

// TestParseNegativeScaleErrors asserts Parse returns an error wrapping
// ErrNegativeScale for negative scales (previously it constructed a value whose
// String() panicked, and strings.Repeat would also panic on a negative count).
func TestParseNegativeScaleErrors(t *testing.T) {
	for _, scale := range []int{-1, -2, -10} {
		_, err := Parse("1.23", scale)
		if !errors.Is(err, ErrNegativeScale) {
			t.Fatalf("Parse(%q, %d): err = %v, want ErrNegativeScale", "1.23", scale, err)
		}
	}
}

// TestRescaleNegativeIsNoOp asserts Rescale(-n) does not panic and returns d
// unchanged. On the OLD code Rescale(-1) built a Decimal with scale<0 whose
// String() then panicked with a slice-out-of-range.
func TestRescaleNegativeIsNoOp(t *testing.T) {
	cases := []struct {
		unscaled int64
		scale    int
		want     string
	}{
		{150, 2, "1.50"},
		{-150, 2, "-1.50"},
		{5, 0, "5"},
	}
	for _, tc := range cases {
		d := New(tc.unscaled, tc.scale)
		r := d.Rescale(-1)
		if got := r.String(); got != tc.want {
			t.Fatalf("Rescale(-1) of New(%d,%d): String = %q, want %q (should be a no-op)", tc.unscaled, tc.scale, got, tc.want)
		}
		if r.Scale() != tc.scale {
			t.Fatalf("Rescale(-1) changed scale: got %d, want %d", r.Scale(), tc.scale)
		}
	}
}

// TestStringIsPanicFreeOnAnyScale asserts the String() defensive guard: even a
// Decimal with a malformed negative scale (constructed by direct struct literal,
// bypassing New) renders without panicking.
func TestStringIsPanicFreeOnAnyScale(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("String() panicked on malformed Decimal: %v", r)
		}
	}()
	// Direct struct construction bypasses New's guard, exercising String()'s own
	// scale<=0 defensive branch.
	for _, d := range []Decimal{
		{value: newBigInt(123), scale: -1},
		{value: newBigInt(-123), scale: -5},
		{value: newBigInt(0), scale: -2},
	} {
		_ = d.String()
	}
}

// --- F4: zero-value Decimal{} arithmetic must not panic (treat as 0). ---

// TestZeroValueArithmeticNilSafe exercises every arithmetic method on a zero-value
// Decimal{} (d.value == nil). On the OLD code each of these nil-dereferenced.
func TestZeroValueArithmeticNilSafe(t *testing.T) {
	var z Decimal

	t.Run("Mul", func(t *testing.T) {
		if got := z.Mul(5).String(); got != "0" {
			t.Fatalf("z.Mul(5) = %q, want 0", got)
		}
		if got := z.Mul(-5).String(); got != "0" {
			t.Fatalf("z.Mul(-5) = %q, want 0", got)
		}
	})

	t.Run("Negate", func(t *testing.T) {
		if got := z.Negate().String(); got != "0" {
			t.Fatalf("z.Negate() = %q, want 0", got)
		}
	})

	t.Run("Abs", func(t *testing.T) {
		if got := z.Abs().String(); got != "0" {
			t.Fatalf("z.Abs() = %q, want 0", got)
		}
		if !z.Abs().IsZero() {
			t.Fatal("z.Abs() should be zero")
		}
	})

	t.Run("Rescale", func(t *testing.T) {
		if got := z.Rescale(3).String(); got != "0.000" {
			t.Fatalf("z.Rescale(3) = %q, want 0.000", got)
		}
	})

	t.Run("Add", func(t *testing.T) {
		// z has scale 0, so the operand must share scale 0.
		other := New(150, 0)
		sum, err := z.Add(other)
		if err != nil {
			t.Fatal(err)
		}
		if got := sum.String(); got != "150" {
			t.Fatalf("z + 150 = %q, want 150", got)
		}
		// other + z symmetric.
		sum2, _ := other.Add(z)
		if got := sum2.String(); got != "150" {
			t.Fatalf("150 + z = %q, want 150", got)
		}
		// z + z == 0.
		got, _ := z.Add(z)
		if got.String() != "0" {
			t.Fatalf("z + z = %q, want 0", got)
		}
		// Mixed-sign operand at scale 0.
		neg := New(-7, 0)
		d, _ := z.Add(neg)
		if d.String() != "-7" {
			t.Fatalf("z + (-7) = %q, want -7", d)
		}
	})

	t.Run("Sub", func(t *testing.T) {
		other := New(150, 0)
		// z - other == -other.
		diff, err := z.Sub(other)
		if err != nil {
			t.Fatal(err)
		}
		if got := diff.String(); got != "-150" {
			t.Fatalf("z - 150 = %q, want -150", got)
		}
		// other - z == other.
		diff2, _ := other.Sub(z)
		if got := diff2.String(); got != "150" {
			t.Fatalf("150 - z = %q, want 150", got)
		}
	})

	t.Run("MulDecimal", func(t *testing.T) {
		other := New(150, 0)
		p := z.MulDecimal(other)
		// z has scale 0, other scale 0 -> product scale 0 -> "0".
		if got := p.String(); got != "0" {
			t.Fatalf("z * 150 = %q, want 0 (scale %d)", got, p.Scale())
		}
		if !p.IsZero() {
			t.Fatal("z * other should be zero")
		}
	})

	t.Run("Div", func(t *testing.T) {
		q, err := z.Div(3)
		if err != nil {
			t.Fatal(err)
		}
		if got := q.String(); got != "0" {
			t.Fatalf("z / 3 = %q, want 0", got)
		}
	})

	t.Run("Cmp", func(t *testing.T) {
		other := New(150, 2)
		cmp, err := z.Rescale(2).Cmp(other)
		if err != nil {
			t.Fatal(err)
		}
		if cmp >= 0 {
			t.Fatalf("0 Cmp 1.50 = %d, want <0", cmp)
		}
	})
}

// --- F9: Unscaled() must return an independent copy (Immutable contract). ---

// TestUnscaledReturnsCopy asserts that mutating the big.Int returned by Unscaled()
// does not affect the original Decimal. On the OLD code Unscaled() returned the
// live internal pointer, so this mutation corrupted d.
func TestUnscaledReturnsCopy(t *testing.T) {
	cases := []struct {
		unscaled int64
		scale    int
		want     string
	}{
		{12345, 3, "12.345"},   // positive
		{-12345, 3, "-12.345"}, // negative
		{0, 2, "0.00"},         // zero
		{50, 0, "50"},          // whole
	}
	for _, tc := range cases {
		d := New(tc.unscaled, tc.scale)
		before := d.String()
		u := d.Unscaled()
		u.SetInt64(999) // mutate the returned big.Int

		if got := d.String(); got != before {
			t.Fatalf("New(%d,%d): after mutating Unscaled(), String = %q, want %q (Immutable contract violated)",
				tc.unscaled, tc.scale, got, before)
		}
		if got := d.String(); got != tc.want {
			t.Fatalf("New(%d,%d): String = %q, want %q", tc.unscaled, tc.scale, got, tc.want)
		}
	}
}

// --- F11: Div by zero returns a matchable ErrDivideByZero sentinel. ---

// TestDivByZeroSentinel asserts Div(0) returns an error matchable via errors.Is
// against the exported ErrDivideByZero. On the OLD code Div returned a bare
// errors.New string that could not be matched.
func TestDivByZeroSentinel(t *testing.T) {
	cases := []struct {
		unscaled int64
		scale    int
	}{
		{150, 2},  // positive
		{-150, 2}, // negative
		{0, 2},    // zero
	}
	for _, tc := range cases {
		d := New(tc.unscaled, tc.scale)
		_, err := d.Div(0)
		if err == nil {
			t.Fatalf("New(%d,%d).Div(0): expected error, got nil", tc.unscaled, tc.scale)
		}
		if !errors.Is(err, ErrDivideByZero) {
			t.Fatalf("New(%d,%d).Div(0): err = %v, want ErrDivideByZero", tc.unscaled, tc.scale, err)
		}
	}
}
