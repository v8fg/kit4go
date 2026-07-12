package decimal

import (
	"errors"
	"testing"
)

func TestParseAndString(t *testing.T) {
	cases := []struct {
		input string
		scale int
		want  string
	}{
		{"12.345", 3, "12.345"},
		{"0.05", 3, "0.050"},
		{"100", 2, "100.00"},
		{"-3.14", 4, "-3.1400"},
		{"0", 2, "0.00"},
		{"9999999999.99", 2, "9999999999.99"},
	}
	for _, tc := range cases {
		d, err := Parse(tc.input, tc.scale)
		if err != nil {
			t.Fatalf("Parse(%q, %d): %v", tc.input, tc.scale, err)
		}
		if got := d.String(); got != tc.want {
			t.Fatalf("Parse(%q).String() = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestParseTooManyDigits(t *testing.T) {
	_, err := Parse("1.234", 2)
	if !errors.Is(err, ErrParse) {
		t.Fatalf("expected ErrParse, got %v", err)
	}
}

func TestAdd(t *testing.T) {
	a := MustParse("12.34", 2)
	b := MustParse("0.06", 2)
	sum, err := a.Add(b)
	if err != nil {
		t.Fatal(err)
	}
	if sum.String() != "12.40" {
		t.Fatalf("sum = %s, want 12.40", sum)
	}
}

func TestAddScaleMismatch(t *testing.T) {
	a := MustParse("1.0", 1)
	b := MustParse("1.00", 2)
	_, err := a.Add(b)
	if !errors.Is(err, ErrScaleMismatch) {
		t.Fatalf("expected ErrScaleMismatch, got %v", err)
	}
}

func TestSub(t *testing.T) {
	a := MustParse("10.00", 2)
	b := MustParse("3.50", 2)
	diff, _ := a.Sub(b)
	if diff.String() != "6.50" {
		t.Fatalf("diff = %s, want 6.50", diff)
	}
}

func TestMul(t *testing.T) {
	a := MustParse("12.50", 2)
	product := a.Mul(3)
	if product.String() != "37.50" {
		t.Fatalf("product = %s, want 37.50", product)
	}
}

func TestMulDecimal(t *testing.T) {
	// 12.50 * 0.08 (tax rate) → scale 2+2=4 → 1.0000
	a := MustParse("12.50", 2)
	b := MustParse("0.08", 2)
	product := a.MulDecimal(b)
	if product.String() != "1.0000" {
		t.Fatalf("product = %s, want 1.0000", product)
	}
	// Rescale back to 2: 1.00
	r := product.Rescale(2)
	if r.String() != "1.00" {
		t.Fatalf("rescaled = %s, want 1.00", r)
	}
}

func TestDiv(t *testing.T) {
	a := MustParse("100.00", 2)
	q, err := a.Div(3)
	if err != nil {
		t.Fatal(err)
	}
	if q.String() != "33.33" {
		t.Fatalf("div = %s, want 33.33", q)
	}
}

func TestDivByZero(t *testing.T) {
	a := MustParse("1.00", 2)
	_, err := a.Div(0)
	if err == nil {
		t.Fatal("expected divide-by-zero error")
	}
}

func TestCmp(t *testing.T) {
	a := MustParse("1.50", 2)
	b := MustParse("2.00", 2)
	c := MustParse("1.50", 2)
	if cmp, _ := a.Cmp(b); cmp >= 0 {
		t.Fatal("1.50 < 2.00")
	}
	if cmp, _ := a.Cmp(c); cmp != 0 {
		t.Fatal("1.50 == 1.50")
	}
	if cmp, _ := b.Cmp(a); cmp <= 0 {
		t.Fatal("2.00 > 1.50")
	}
}

func TestNegate(t *testing.T) {
	a := MustParse("3.14", 2)
	n := a.Negate()
	if n.String() != "-3.14" {
		t.Fatalf("negate = %s, want -3.14", n)
	}
	if n.Negate().String() != "3.14" {
		t.Fatalf("double negate failed")
	}
}

func TestAbs(t *testing.T) {
	a := MustParse("-5.00", 2)
	if a.Abs().String() != "5.00" {
		t.Fatalf("abs(-5.00) = %s, want 5.00", a.Abs())
	}
	b := MustParse("5.00", 2)
	if b.Abs().String() != "5.00" {
		t.Fatalf("abs(5.00) = %s, want 5.00", b.Abs())
	}
}

func TestRescale(t *testing.T) {
	a := MustParse("1.50", 2)
	up := a.Rescale(4)
	if up.String() != "1.5000" {
		t.Fatalf("rescale up = %s, want 1.5000", up)
	}
	down := up.Rescale(2)
	if down.String() != "1.50" {
		t.Fatalf("rescale down = %s, want 1.50", down)
	}
	// Truncation: 1.99 rescaled to scale 1 → 1.9
	b := MustParse("1.99", 2)
	trunc := b.Rescale(1)
	if trunc.String() != "1.9" {
		t.Fatalf("rescale truncate = %s, want 1.9", trunc)
	}
}

func TestLargeValues(t *testing.T) {
	// 999999999999 * 999999999999 → should not overflow
	a := MustParse("999999999999.00", 2)
	b := MustParse("999999999999.00", 2)
	p := a.MulDecimal(b)
	if p.value.Sign() <= 0 {
		t.Fatal("large multiplication should be positive")
	}
}

func TestZeroValue(t *testing.T) {
	var d Decimal // zero value
	if d.String() != "0" {
		t.Fatalf("zero-value String() = %q, want 0", d.String())
	}
	if d.Sign() != 0 {
		t.Fatalf("zero-value Sign() = %d, want 0", d.Sign())
	}
	if !d.IsZero() {
		t.Fatal("zero-value IsZero() should be true")
	}
	if d.Unscaled().Sign() != 0 {
		t.Fatal("zero-value Unscaled() should be big.Int(0)")
	}
}

func TestAccessors(t *testing.T) {
	d := New(12345, 3)
	if d.Scale() != 3 {
		t.Fatalf("Scale() = %d, want 3", d.Scale())
	}
	if d.Unscaled().Int64() != 12345 {
		t.Fatalf("Unscaled() = %d, want 12345", d.Unscaled())
	}
	if d.Sign() != 1 {
		t.Fatalf("Sign() = %d, want 1", d.Sign())
	}
	if d.IsZero() {
		t.Fatal("IsZero() should be false")
	}
	fi := FromInt(42)
	if fi.Scale() != 0 || fi.Unscaled().Int64() != 42 {
		t.Fatalf("FromInt(42) wrong: scale=%d unscaled=%d", fi.Scale(), fi.Unscaled())
	}
}

func TestParsePlusPrefix(t *testing.T) {
	d, err := Parse("+1.50", 2)
	if err != nil {
		t.Fatal(err)
	}
	if d.String() != "1.50" {
		t.Fatalf("+prefix: %s", d)
	}
}

func TestParseEmpty(t *testing.T) {
	_, err := Parse("", 2)
	if err == nil {
		t.Fatal("empty string should error")
	}
}

func TestRescaleLargeScale(t *testing.T) {
	// Scale > 20 hits the pow10 fallback path.
	d := New(1, 0)
	r := d.Rescale(25)
	if r.Scale() != 25 {
		t.Fatalf("scale = %d, want 25", r.Scale())
	}
}

func TestParseRejectsDoubleSign(t *testing.T) {
	// Doubled/mismatched signs used to slip through (big.Int.SetString accepts a
	// leading sign): "+-5" parsed as -5.00, "--5" cancelled to +5.00.
	for _, bad := range []string{"++5", "--5", "-+5", "+-5", "+", "-"} {
		if _, err := Parse(bad, 2); err == nil {
			t.Errorf("Parse(%q) should error", bad)
		}
	}
}
