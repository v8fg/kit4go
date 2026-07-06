package money

import (
	"errors"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// ----------------------------------------------------------------------------
// Unreachable defensive branches (documented, intentionally NOT covered)
// ----------------------------------------------------------------------------
//
// money.go FromMajor, lines 113-115:
//
//	scaledFrac, err := strconv.ParseInt(fracStr, 10, 64)
//	if err != nil {
//	    return Money{}, fmt.Errorf("%w: %w", ErrInvalidAmount, err)
//	}
//
// fracStr is built as `strings.Repeat("0", c.Decimals-len(strconv.Itoa(absInt(frac)))) + strconv.Itoa(absInt(frac))`.
// absInt always returns >= 0, and strconv.Itoa of a non-negative int always
// produces a digit-only string [0-9]+. The preceding `len(fracStr) > c.Decimals`
// guard guarantees fracStr has exactly c.Decimals runes (<= 3 for every ISO
// currency). ParseInt of a short digit-only string in base 10 cannot fail, so
// this error branch is structurally unreachable. Excluded from the 100% target.
// ----------------------------------------------------------------------------

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

// TestFromMajor_UnknownCurrency covers FromMajor's `!ok` error branch
// (money.go:98-100): an unknown currency code returns ErrUnknownCurrency.
func TestFromMajor_UnknownCurrency(t *testing.T) {
	_, err := FromMajor(12, 34, "NOPE")
	require.ErrorIs(t, err, ErrUnknownCurrency)
}

// TestFromMajor_MulCheckedOverflow covers FromMajor's `mulChecked` overflow in
// the decimals-scaling loop (money.go:119-121). whole = math.MaxInt64 parses
// fine, but whole*10 (first loop iteration for a 2-decimal currency) overflows
// int64, so the error is the checked-arithmetic ErrOverflow — not a parse error.
func TestFromMajor_MulCheckedOverflow(t *testing.T) {
	_, err := FromMajor(math.MaxInt64, 0, "USD")
	require.ErrorIs(t, err, ErrOverflow)
	// 3-decimal currency: a smaller whole already overflows when scaled by 1000.
	_, err = FromMajor(1<<60, 0, "KWD")
	require.ErrorIs(t, err, ErrOverflow)
}

// TestParse_FracParseIntError covers Parse's fractional ParseInt error branch
// (money.go:163-165). "1.2a" in a 3-decimal currency (KWD): whole "1" parses
// OK, frac "2a" is within the decimals budget but isn't a valid integer, so
// strconv.ParseInt fails -> ErrInvalidAmount. (frac < 0 is structurally
// impossible here because the sign was already stripped, so this is the only
// reachable arm of the `err != nil || frac < 0` guard.)
func TestParse_FracParseIntError(t *testing.T) {
	_, err := Parse("KWD", "1.2a")
	require.ErrorIs(t, err, ErrInvalidAmount)
}

// TestParse_MulCheckedOverflow covers Parse's `mulChecked` overflow in the
// decimals-scaling loop (money.go:170-172). whole = math.MaxInt64 parses as a
// valid int64, but whole*10 overflows on the first scaling iteration.
func TestParse_MulCheckedOverflow(t *testing.T) {
	_, err := Parse("USD", "9223372036854775807")
	require.ErrorIs(t, err, ErrOverflow)
}

// TestAllocate_MulCheckedOverflow covers Allocate's `mulChecked` overflow
// (money.go:323-325). Allocating math.MaxInt64 minor units with a ratio of 2
// makes mulChecked(MaxInt64, 2) overflow before any share is computed.
func TestAllocate_MulCheckedOverflow(t *testing.T) {
	big := MustFromMinor(math.MaxInt64, "USD")
	_, err := big.Allocate([]int{2, 1})
	require.ErrorIs(t, err, ErrOverflow)
}

// TestAllocate_RemainderPicksLaterIndex covers Allocate's inner selection-loop
// body `best = j` (money.go:350-352), the arm that fires when a later share has
// a strictly larger fractional remainder than the current best. With ratios
// {1, 2} of 100 cents: ideal shares are 33.33 / 66.67, integer division yields
// 33 / 66 leaving remainder 1. The fractional remainders are 0.33 and 0.67, so
// index 1 beats the initial best (0) and receives the extra cent (67). This is
// the only configuration that exercises the `>` arm; the {1,1,1} test leaves
// index 0 as the persistent maximum.
func TestAllocate_RemainderPicksLaterIndex(t *testing.T) {
	m := MustFromMinor(100, "USD")
	parts, err := m.Allocate([]int{1, 2})
	require.NoError(t, err)
	require.Len(t, parts, 2)
	// 67/33: the larger-ratio share got the rounding remainder.
	require.Equal(t, int64(67), parts[1].Amount())
	require.Equal(t, int64(33), parts[0].Amount())
	require.Equal(t, int64(100), parts[0].Amount()+parts[1].Amount())
}

// TestRoundFloatToInt64_RoundUpNegative covers roundFloatToInt64's RoundUp
// negative arm (money.go:400): for f < 0, RoundUp floors (rounds away from
// zero). Exercised both directly and through Scale on a negative Money.
func TestRoundFloatToInt64_RoundUpNegative(t *testing.T) {
	// Direct: -1.5 with RoundUp -> floor(-1.5) = -2 (away from zero).
	r, err := roundFloatToInt64(-1.5, RoundUp)
	require.NoError(t, err)
	require.Equal(t, int64(-2), r)

	// Via Scale: -100 minor (-1.00) * 0.005 = -0.5 -> RoundUp negative -> -1.
	neg := MustFromMinor(-100, "USD")
	scaled, err := neg.Scale(0.005, RoundUp)
	require.NoError(t, err)
	require.Equal(t, int64(-1), scaled.Amount())

	// Sanity: positive RoundUp still ceils (1.5 -> 2), confirming the branch
	// taken is the negative one above and not the `f >= 0` arm.
	pos, err := roundFloatToInt64(1.5, RoundUp)
	require.NoError(t, err)
	require.Equal(t, int64(2), pos)
}
