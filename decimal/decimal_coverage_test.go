package decimal

import (
	"errors"
	"testing"
)

// TestMustParse_Panic covers the panic branch of MustParse (parse error).
func TestMustParse_Panic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("MustParse with invalid input should panic")
		}
	}()
	_ = MustParse("not-a-number", 2)
}

// TestMustParse_OK covers the happy path of MustParse returning a value.
func TestMustParse_OK(t *testing.T) {
	d := MustParse("1.50", 2)
	if d.String() != "1.50" {
		t.Fatalf("got %s", d)
	}
}

// TestParseInvalidNumber covers the big.Int SetString failure branch.
func TestParseInvalidNumber(t *testing.T) {
	_, err := Parse("abc", 2)
	if !errors.Is(err, ErrParse) {
		t.Fatalf("expected ErrParse, got %v", err)
	}
}

// TestSubScaleMismatch covers the error branch of Sub.
func TestSubScaleMismatch(t *testing.T) {
	a := MustParse("1.0", 1)
	b := MustParse("1.00", 2)
	_, err := a.Sub(b)
	if !errors.Is(err, ErrScaleMismatch) {
		t.Fatalf("expected ErrScaleMismatch, got %v", err)
	}
}

// TestCmpScaleMismatch covers the error branch of Cmp.
func TestCmpScaleMismatch(t *testing.T) {
	a := MustParse("1.0", 1)
	b := MustParse("1.00", 2)
	_, err := a.Cmp(b)
	if !errors.Is(err, ErrScaleMismatch) {
		t.Fatalf("expected ErrScaleMismatch, got %v", err)
	}
}

// TestStringScaleZero covers the d.scale == 0 branches of String (positive and
// negative).
func TestStringScaleZero(t *testing.T) {
	pos := New(123, 0)
	if got := pos.String(); got != "123" {
		t.Fatalf("positive scale-0 String = %q, want 123", got)
	}
	neg := New(-123, 0)
	if got := neg.String(); got != "-123" {
		t.Fatalf("negative scale-0 String = %q, want -123", got)
	}
}

// TestStringNegativeFractional covers the negative number path through the
// fractional-digit formatting branch of String.
func TestStringNegativeFractional(t *testing.T) {
	d := New(-5, 5) // -0.00005 -> unscaled -5, scale 5
	if got := d.String(); got != "-0.00005" {
		t.Fatalf("got %q, want -0.00005", got)
	}
}

// TestStringSmallValuePadding covers the leading-zero padding loop in String
// (abs value shorter than scale+1 digits).
func TestStringSmallValuePadding(t *testing.T) {
	// unscaled 7, scale 4 -> 0.0007 (pad loop runs since "7" <= 4).
	d := New(7, 4)
	if got := d.String(); got != "0.0007" {
		t.Fatalf("got %q, want 0.0007", got)
	}
}

// TestRescaleSameScale covers the early-return branch of Rescale.
func TestRescaleSameScale(t *testing.T) {
	a := MustParse("1.50", 2)
	r := a.Rescale(2)
	if r.String() != "1.50" {
		t.Fatalf("got %s", r)
	}
}

// TestParseNegativeWholeOnly covers a negative number without a fractional
// part being padded up to scale.
func TestParseNegativeWholeOnly(t *testing.T) {
	d, err := Parse("-100", 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := d.String(); got != "-100.00" {
		t.Fatalf("got %q, want -100.00", got)
	}
}

// TestParsePositiveWholeOnly covers the positive-prefix path with padding.
func TestParsePositiveWholeOnly(t *testing.T) {
	d, err := Parse("+42", 3)
	if err != nil {
		t.Fatal(err)
	}
	if got := d.String(); got != "42.000" {
		t.Fatalf("got %q, want 42.000", got)
	}
}
