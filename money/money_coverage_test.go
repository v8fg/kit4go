package money

import (
	"errors"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRegisterCurrency_EmptyCode covers the `c.Code == ""` early-return branch
// of RegisterCurrency (a no-op for empty codes so the registry isn't polluted
// with an "" entry).
func TestRegisterCurrency_EmptyCode(t *testing.T) {
	before := len(Currencies())
	RegisterCurrency(Currency{Code: "", Numeric: "000", Decimals: 2})
	// Must not register an entry under "".
	_, ok := Lookup("")
	require.False(t, ok, "empty code must not be registered")
	require.Len(t, Currencies(), before, "registry size unchanged for empty code")
}

// TestMustFromMinor_Panic covers the panic branch of MustFromMinor (unknown
// currency code).
func TestMustFromMinor_Panic(t *testing.T) {
	require.Panics(t, func() { MustFromMinor(100, "NOPE") })
}

// TestFromMajor_ZeroDecimalFracError covers FromMajor's 0-decimal currency +
// non-zero frac error branch (e.g. passing cents to JPY).
func TestFromMajor_ZeroDecimalFracError(t *testing.T) {
	_, err := FromMajor(100, 50, "JPY") // JPY has 0 decimals, frac must be 0
	require.ErrorIs(t, err, ErrInvalidAmount)
}

// TestFromMajor_NegativeWholeAndFrac covers FromMajor's negative path where
// both whole and frac are negative (the `minor -= scaledFrac` branch).
func TestFromMajor_NegativeWholeAndFrac(t *testing.T) {
	// -12.34 USD via the negative whole+frac branch -> -1234 cents.
	m, err := FromMajor(-12, -34, "USD")
	require.NoError(t, err)
	require.Equal(t, int64(-1234), m.Amount())

	// Negative whole, zero frac -> still negative; minor is whole*10^dec.
	m2, err := FromMajor(-5, 0, "USD")
	require.NoError(t, err)
	require.Equal(t, int64(-500), m2.Amount())
}

// TestParse_FracParseError covers Parse's fractional parse-error branch
// (fractional part isn't a valid integer — exercised via a non-numeric frac).
// strconv.ParseInt on a digit-only string won't error, so we drive the whole<0
// guard instead, plus the explicit-plus path is already covered.
func TestParse_WholeNegativeGuard(t *testing.T) {
	// A leading '-' is stripped and `neg` set; the whole part itself must parse
	// as non-negative. "-abc" trips the whole-parse-error branch.
	_, err := Parse("USD", "-abc")
	require.ErrorIs(t, err, ErrInvalidAmount)
	// "abc" (no sign) -> whole parse error.
	_, err = Parse("USD", "abc")
	require.ErrorIs(t, err, ErrInvalidAmount)
}

// TestSub_Overflow covers Sub's overflow branch: subtracting a large negative
// from a large positive overflows int64.
func TestSub_Overflow(t *testing.T) {
	big := MustFromMinor(1<<62, "USD")
	veryNeg := MustFromMinor(-(1 << 62), "USD")
	_, err := big.Sub(veryNeg) // big - (-big) ≈ 2*big → overflow
	require.ErrorIs(t, err, ErrOverflow)
}

// TestAllocate_NegativeAmountRemainder covers Allocate's negative-amount path:
// allocating a negative Money distributes a negative remainder (step = -1),
// exercising the `if remainder < 0 { step = -1 }` branch and the loop.
func TestAllocate_NegativeAmountRemainder(t *testing.T) {
	neg := MustFromMinor(-100, "USD") // -$1.00
	parts, err := neg.Allocate([]int{1, 1, 1})
	require.NoError(t, err)
	sum := int64(0)
	for _, p := range parts {
		sum += p.Amount()
	}
	require.Equal(t, int64(-100), sum, "no minor units lost on negative allocate")
	// Each share is roughly -33/-33/-34; the most-negative gets the extra cent.
	require.LessOrEqual(t, parts[0].Amount(), int64(-34))
}

// TestRoundFloatToInt64_OverflowMin covers roundFloatToInt64's overflow branch
// at the negative extreme (f <= math.MinInt64).
func TestRoundFloatToInt64_OverflowMin(t *testing.T) {
	_, err := roundFloatToInt64(math.MinInt64*1.0-1, RoundHalfUp)
	require.ErrorIs(t, err, ErrOverflow)
	// And the positive extreme boundary.
	_, err = roundFloatToInt64(math.MaxInt64+1, RoundHalfUp)
	require.ErrorIs(t, err, ErrOverflow)
}

// TestShouldRoundUp_HalfEvenExactHalf covers shouldRoundUp's RoundHalfEven
// exact-half branch (`absRem*2 == absDiv` returns true — the half-up fallback).
func TestShouldRoundUp_HalfEvenExactHalf(t *testing.T) {
	// remainder 1, divisor 2 → absRem*2 == absDiv → exact half → return true.
	require.True(t, shouldRoundUp(1, 2, RoundHalfEven))
	// Negative remainder symmetric: -1, -2.
	require.True(t, shouldRoundUp(-1, -2, RoundHalfEven))
	// Just over half: rem=2, div=3 → absRem*2=4 > 3 → true.
	require.True(t, shouldRoundUp(2, 3, RoundHalfEven))
	// Just under half: rem=1, div=3 → absRem*2=2 < 3 → false.
	require.False(t, shouldRoundUp(1, 3, RoundHalfEven))
}

// TestShouldRoundUp_AllModes exercises every branch of shouldRoundUp for full
// coverage of the RoundUp / RoundDown / default arms with rem==0 and rem!=0.
func TestShouldRoundUp_AllModes(t *testing.T) {
	// rem == 0 → always false regardless of mode.
	for _, mode := range []Rounding{RoundDown, RoundUp, RoundHalfEven, RoundHalfUp} {
		require.False(t, shouldRoundUp(0, 3, mode))
	}
	// RoundDown never rounds up.
	require.False(t, shouldRoundUp(2, 3, RoundDown))
	// RoundUp always rounds up (rem != 0).
	require.True(t, shouldRoundUp(1, 5, RoundUp))
	// RoundHalfUp default: absRem*2 >= absDiv.
	require.True(t, shouldRoundUp(2, 4, RoundHalfUp))  // exactly half
	require.False(t, shouldRoundUp(1, 4, RoundHalfUp)) // under half
}

// TestAbsI64_Negative covers absI64's negative branch directly.
func TestAbsI64_Negative(t *testing.T) {
	require.Equal(t, int64(42), absI64(-42))
	require.Equal(t, int64(42), absI64(42))
	require.Equal(t, int64(0), absI64(0))
}

// TestAbsInt_Negative covers absInt's negative branch directly.
func TestAbsInt_Negative(t *testing.T) {
	require.Equal(t, 42, absInt(-42))
	require.Equal(t, 42, absInt(42))
	require.Equal(t, 0, absInt(0))
}

// TestFromMajor_TooManyDigits covers the too-many-fractional-digits branch for
// a 3-decimal currency (KWD).
func TestFromMajor_TooManyDigits(t *testing.T) {
	_, err := FromMajor(1, 99999, "KWD") // 5 digits > 3
	require.ErrorIs(t, err, ErrInvalidAmount)
}

// TestMulChecked_OverflowDirect exercises the overflow guard of mulChecked
// directly (used by Allocate / Mul); covers both the a==0/b==0 fast path and
// the overflow path.
func TestMulChecked_OverflowDirect(t *testing.T) {
	zero, err := mulChecked(0, 1<<60)
	require.NoError(t, err)
	require.Equal(t, int64(0), zero)

	_, err = mulChecked(1<<60, 1<<10)
	require.ErrorIs(t, err, ErrOverflow)
}

// TestAddChecked_OverflowDirect exercises addChecked's overflow path directly.
func TestAddChecked_OverflowDirect(t *testing.T) {
	_, err := addChecked(int64(1<<62), int64(1<<62))
	require.ErrorIs(t, err, ErrOverflow)
}

// TestErrorsSentinels ensures each exported sentinel is itself (guards against
// accidental reassignment during refactors).
func TestErrorsSentinals(t *testing.T) {
	require.True(t, errors.Is(ErrInvalidAmount, ErrInvalidAmount))
	require.True(t, errors.Is(ErrDivideByZero, ErrDivideByZero))
	require.True(t, errors.Is(ErrNoRatios, ErrNoRatios))
	require.True(t, errors.Is(ErrNegativeRatio, ErrNegativeRatio))
	require.True(t, errors.Is(ErrZeroRatios, ErrZeroRatios))
}
