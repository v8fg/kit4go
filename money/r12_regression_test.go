package money

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// This file holds the regression tests for the R12 money findings (finance
// correctness). Each test asserts the CORRECT finance value for the edge input
// and FAILS against the pre-fix code:
//
//   - TestR12_Div_NegativeQuotientDirection (F1): Div rounded toward zero for
//     negative dividends instead of away from zero.
//   - TestR12_Div_RoundHalfEven_BankersRounding (F8): Div fell back to half-up
//     at exact halves instead of rounding to the nearest even quotient.
//   - TestR12_Sub_MinInt64_Overflow (F5): Sub missed overflow when the
//     subtrahend was MinInt64 (-MinInt64 wraps to MinInt64).
//   - TestR12_Constructors_RejectMinInt64 (F6): MinInt64 was accepted at
//     construction, breaking Abs/Negate/String downstream.
//   - TestR12_FromMajor_MixedSign (F7): FromMajor silently produced a wrong
//     magnitude for whole/frac of opposite signs.
//   - TestR12_roundDir_HalfEvenParity: unit-level cover for the parity-aware
//     half-even decision now that roundDir owns it.

// TestR12_Div_NegativeQuotientDirection asserts that Div rounds AWAY FROM ZERO
// for negative results under RoundUp and RoundHalfUp. Pre-fix the apply step did
// `q++` unconditionally; for a negative quotient that moved the result toward
// zero (the opposite of the documented direction). RoundDown (toward zero) is
// correct for both signs and is asserted here for parity.
func TestR12_Div_NegativeQuotientDirection(t *testing.T) {
	type tc struct {
		name                     string
		amount, divisor          int64
		mode                     Rounding
		want                     int64
		note                     string
		alsoCheckPositiveEquival bool
	}
	cases := []tc{
		// -7 / 2 = -3.5 → RoundHalfUp/Up away from zero = -4 (pre-fix returned -3).
		{"-7/2 RoundHalfUp", -7, 2, RoundHalfUp, -4, "-3.5 away from zero is -4", true},
		{"-7/2 RoundUp", -7, 2, RoundUp, -4, "-3.5 away from zero is -4", true},
		// -5 / 2 = -2.5 → RoundHalfUp away from zero = -3 (pre-fix returned -2).
		{"-5/2 RoundHalfUp", -5, 2, RoundHalfUp, -3, "-2.5 away from zero is -3", true},
		// Divisor negative: 10 / -3 = -3.33. RoundHalfUp rounds to NEAREST, and
		// -3.33 is closer to -3 than -4, so the correct result is -3. (This case
		// guards that the sign-aware apply doesn't spuriously round away from
		// zero for under-half remainders.)
		{"10/-3 RoundHalfUp", 10, -3, RoundHalfUp, -3, "-3.33 nearest is -3", true},
		// RoundDown truncates toward zero for negatives too: -10/3 = -3.33 → -3.
		{"-10/3 RoundDown", -10, 3, RoundDown, -3, "toward zero", true},
		// -10 / -3 = 3.33 → RoundHalfUp nearest = 3.
		{"-10/-3 RoundHalfUp", -10, -3, RoundHalfUp, 3, "3.33 nearest is 3", true},
		// Whole-number division is unaffected.
		{"-8/2 exact", -8, 2, RoundHalfUp, -4, "exact, no rounding", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := MustFromMinor(c.amount, "USD")
			got, err := m.Div(c.divisor, c.mode)
			require.NoError(t, err)
			require.Equalf(t, c.want, got.Amount(),
				"%s: want %d (%s)", c.name, c.want, c.note)
		})
	}

	// Positive equivalents: confirm the positive direction is unchanged by the
	// sign-aware apply (away from zero for RoundUp/RoundHalfUp, toward zero for
	// RoundDown). These mirror the negative cases above.
	pos := []struct {
		name            string
		amount, divisor int64
		mode            Rounding
		want            int64
	}{
		{"7/2 RoundHalfUp", 7, 2, RoundHalfUp, 4},
		{"7/2 RoundUp", 7, 2, RoundUp, 4},
		{"5/2 RoundHalfUp", 5, 2, RoundHalfUp, 3},
		{"10/3 RoundDown", 10, 3, RoundDown, 3},
	}
	for _, c := range pos {
		t.Run(c.name, func(t *testing.T) {
			m := MustFromMinor(c.amount, "USD")
			got, err := m.Div(c.divisor, c.mode)
			require.NoError(t, err)
			require.Equal(t, c.want, got.Amount())
		})
	}
}

// TestR12_Div_RoundHalfEven_BankersRounding asserts true round-half-even
// (banker's rounding): at an exact half the result rounds to the nearest EVEN
// quotient. Pre-fix Div fell back to half-up at exact halves, so Div(501,2,...)
// returned 251 instead of 250.
//
// 501/2 = 250.5 → truncated q=250 (even) → stays 250.
// 503/2 = 251.5 → truncated q=251 (odd)  → rounds away to 252.
// Negative halves are symmetric about zero (the parity of the truncated q,
// which is toward zero, decides).
func TestR12_Div_RoundHalfEven_BankersRounding(t *testing.T) {
	cases := []struct {
		name            string
		amount, divisor int64
		want            int64
	}{
		{"501/2 -> 250 (even)", 501, 2, 250},
		{"503/2 -> 252 (even)", 503, 2, 252},
		{"5/2 -> 2 (even)", 5, 2, 2},
		{"7/2 -> 4 (even)", 7, 2, 4},
		// Negative halves: truncation is toward zero, so the parity of |q| decides.
		{"-501/2 -> -250 (even)", -501, 2, -250},
		{"-503/2 -> -252 (even)", -503, 2, -252},
		{"-5/2 -> -2 (even)", -5, 2, -2},
		{"-7/2 -> -4 (even)", -7, 2, -4},
		// Non-exact halves still follow the >half rule.
		{"100/3 -> 33 (closer)", 100, 3, 33},
		{"200/3 -> 67 (closer)", 200, 3, 67},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := MustFromMinor(c.amount, "USD")
			got, err := m.Div(c.divisor, RoundHalfEven)
			require.NoError(t, err)
			require.Equalf(t, c.want, got.Amount(),
				"Div(%d,%d,RoundHalfEven) banker's rounding: want %d", c.amount, c.divisor, c.want)
		})
	}
}

// TestR12_Sub_MinInt64_Overflow asserts that subtracting exactly MinInt64 is
// detected as overflow rather than wrapping silently. -MinInt64 overflows to
// MinInt64, so the pre-fix addChecked(m, -other) computed addChecked(m,
// MinInt64) and MISSED the overflow for MaxInt64 - MinInt64 (which should be
// ~2*MaxInt64, far out of range, yet old code returned a wrapped value).
//
// Because F6 now rejects MinInt64 at construction, the exact-MinInt64 Money is
// built via a same-package struct literal (the only way to reach the Sub path
// with amount == MinInt64). This is the precise case the old code got wrong.
func TestR12_Sub_MinInt64_Overflow(t *testing.T) {
	usd := MustCurrency("USD")
	maxM := Money{amount: math.MaxInt64, cur: usd}
	minM := Money{amount: math.MinInt64, cur: usd} // reachable only via literal; constructors reject it

	// MaxInt64 - MinInt64: true result is ~2*MaxInt64 → MUST overflow. Old code
	// did addChecked(MaxInt64, -MinInt64) == addChecked(MaxInt64, MinInt64) == -1
	// (wrapped) and returned no error — the bug. New subChecked detects it.
	_, err := maxM.Sub(minM)
	require.ErrorIs(t, err, ErrOverflow, "Max - Min must overflow, not wrap to -1")

	// MinInt64 - MaxInt64: true result is ~2*MinInt64 → overflow.
	_, err = minM.Sub(maxM)
	require.ErrorIs(t, err, ErrOverflow, "Min - Max must overflow")

	// Normal subtraction still works (regression guard for the new subChecked).
	a := MustFromMinor(1000, "USD")
	b := MustFromMinor(300, "USD")
	diff, err := a.Sub(b)
	require.NoError(t, err)
	require.Equal(t, int64(700), diff.Amount())

	// Negative result via Sub still works.
	neg, err := b.Sub(a)
	require.NoError(t, err)
	require.Equal(t, int64(-700), neg.Amount())

	// Subtraction at the boundaries that does NOT overflow still works:
	// MinInt64+1 - 1 == MinInt64 is fine, and 0 - MinInt64 overflows.
	oneAboveMin := Money{amount: math.MinInt64 + 1, cur: usd}
	one := Money{amount: 1, cur: usd}
	ok, err := oneAboveMin.Sub(one)
	require.NoError(t, err)
	require.Equal(t, int64(math.MinInt64), ok.Amount(), "(Min+1) - 1 == Min is in range")
	zero := Money{amount: 0, cur: usd}
	_, err = zero.Sub(minM)
	require.ErrorIs(t, err, ErrOverflow, "0 - Min overflows (would need +Min, which doesn't exist)")
}

// TestR12_subChecked_Direct is a focused unit test for the new overflow detector
// at the exact MinInt64 boundary. It fails against the old addChecked-based Sub
// (which wrapped for the MinInt64 subtrahend).
func TestR12_subChecked_Direct(t *testing.T) {
	// The canonical wrap: MaxInt64 - MinInt64. Old code returned -1, no error.
	_, err := subChecked(math.MaxInt64, math.MinInt64)
	require.ErrorIs(t, err, ErrOverflow)

	// 0 - MinInt64 overflows (no positive counterpart of MinInt64 exists).
	_, err = subChecked(0, math.MinInt64)
	require.ErrorIs(t, err, ErrOverflow)

	// In-range subtractions are exact.
	got, err := subChecked(5, 3)
	require.NoError(t, err)
	require.Equal(t, int64(2), got)
	got, err = subChecked(-5, -3)
	require.NoError(t, err)
	require.Equal(t, int64(-2), got)
	got, err = subChecked(math.MinInt64+1, 1) // (Min+1) - 1 == Min, in range
	require.NoError(t, err)
	require.Equal(t, int64(math.MinInt64), got)
}

// TestR12_Constructors_RejectMinInt64 asserts that all three constructors reject
// math.MinInt64. The value has no symmetric positive, so Abs/Negate/String all
// misbehave on it; rejecting at construction is the single clean guard.
func TestR12_Constructors_RejectMinInt64(t *testing.T) {
	// FromMinor.
	_, err := FromMinor(math.MinInt64, "USD")
	require.ErrorIs(t, err, ErrOverflow, "FromMinor(MinInt64) must be rejected")

	// MustFromMinor panics on MinInt64 (it panics on any FromMinor error).
	require.Panics(t, func() { MustFromMinor(math.MinInt64, "USD") })

	// Parse cannot construct MinInt64: the magnitude "9223372036854775808"
	// (MinInt64's absolute value) overflows int64 in strconv.ParseInt, so the
	// whole-parse step rejects it; and the negation path computes minor>=0 then
	// negates, which can never equal MinInt64 (-MinInt64 overflows). The
	// MinInt64 guard inside Parse is therefore pure defense-in-depth. We assert
	// the reachable overflow rejection instead.
	_, err = Parse("JPY", "-9223372036854775808")
	require.Error(t, err, "magnitude overflow must be rejected by Parse")

	// FromMajor (0-decimal currency, whole = MinInt64): int64(whole) == MinInt64.
	_, err = FromMajor(math.MinInt64, 0, "JPY")
	require.ErrorIs(t, err, ErrOverflow, "FromMajor(MinInt64,0,JPY) must be rejected")

	// Existing MinInt64-free paths are unaffected.
	m, err := FromMinor(math.MaxInt64, "USD")
	require.NoError(t, err)
	require.Equal(t, int64(math.MaxInt64), m.Amount())

	m2, err := FromMinor(math.MinInt64+1, "USD")
	require.NoError(t, err)
	require.Equal(t, int64(math.MinInt64+1), m2.Amount())
}

// TestR12_FromMajor_MixedSign asserts that whole and frac of opposite signs are
// rejected. Pre-fix FromMajor(12,-34) silently returned 1166 (1200-34) instead
// of erroring.
func TestR12_FromMajor_MixedSign(t *testing.T) {
	// Mixed signs → error.
	_, err := FromMajor(12, -34, "USD")
	require.ErrorIs(t, err, ErrInvalidAmount, "12,-34: opposite signs must error")
	_, err = FromMajor(-12, 34, "USD")
	require.ErrorIs(t, err, ErrInvalidAmount, "-12,34: opposite signs must error")

	// Same-sign and sign-neutral (zero) combinations still work.
	pos, err := FromMajor(12, 34, "USD")
	require.NoError(t, err)
	require.Equal(t, int64(1234), pos.Amount(), "12,34 -> 1234")

	neg, err := FromMajor(-12, -34, "USD")
	require.NoError(t, err)
	require.Equal(t, int64(-1234), neg.Amount(), "-12,-34 -> -1234")

	// frac == 0 is sign-neutral: positive whole + zero frac is fine.
	posZeroFrac, err := FromMajor(12, 0, "USD")
	require.NoError(t, err)
	require.Equal(t, int64(1200), posZeroFrac.Amount())

	// whole == 0 is sign-neutral: negative frac with zero whole is fine (e.g. -0.34).
	negZeroWhole, err := FromMajor(0, -34, "USD")
	require.NoError(t, err)
	require.Equal(t, int64(-34), negZeroWhole.Amount())
}

// TestR12_roundDir_HalfEvenParity covers the parity-aware half-even branch of
// roundDir at unit level: at an exact half, round away from zero iff the
// truncated quotient is odd (so the result becomes even).
func TestR12_roundDir_HalfEvenParity(t *testing.T) {
	// Exact halves (absRem*2 == absDiv): decision is driven solely by q parity.
	// q odd → roundAway; q even → roundStay.
	require.Equal(t, roundAway, roundDir(1, 2, RoundHalfEven, 251), "odd q rounds to even")
	require.Equal(t, roundStay, roundDir(1, 2, RoundHalfEven, 250), "even q stays")
	require.Equal(t, roundAway, roundDir(-1, -2, RoundHalfEven, 251), "sign of rem/div irrelevant to decision")
	require.Equal(t, roundStay, roundDir(-1, -2, RoundHalfEven, 252), "even q stays regardless of rem sign")

	// Just over half → roundAway regardless of parity.
	require.Equal(t, roundAway, roundDir(2, 3, RoundHalfEven, 0))
	// Just under half → roundStay.
	require.Equal(t, roundStay, roundDir(1, 3, RoundHalfEven, 0))
	// rem == 0 → never rounds.
	require.Equal(t, roundStay, roundDir(0, 3, RoundHalfEven, 0))
	// RoundUp always rounds away (rem != 0).
	require.Equal(t, roundAway, roundDir(1, 100, RoundUp, 0))
	// RoundDown never rounds.
	require.Equal(t, roundStay, roundDir(99, 100, RoundDown, 0))
}

// TestR12_roundDir_AllModes_Table is a focused parity table for roundDir across
// modes and quotient parities, locking in the away-from-zero semantics for the
// Div callers.
func TestR12_roundDir_AllModes_Table(t *testing.T) {
	cases := []struct {
		rem, div int64
		mode     Rounding
		oddQ     int64 // truncated quotient that is odd
		evenQ    int64 // truncated quotient that is even
		wantOdd  roundDirection
		wantEven roundDirection
	}{
		// Exact half: half-even differs by parity; half-up/up always away; down stay.
		{1, 2, RoundHalfEven, 251, 250, roundAway, roundStay},
		{1, 2, RoundHalfUp, 251, 250, roundAway, roundAway},
		{1, 2, RoundUp, 251, 250, roundAway, roundAway},
		{1, 2, RoundDown, 251, 250, roundStay, roundStay},
		// Over half (2/3 ≈ 0.67): always round away under half-even/half-up/up.
		{2, 3, RoundHalfEven, 0, 1, roundAway, roundAway},
		{2, 3, RoundHalfUp, 0, 1, roundAway, roundAway},
		// Under half (1/3 ≈ 0.33): stay under half-even/half-up; up always away.
		{1, 3, RoundHalfEven, 0, 1, roundStay, roundStay},
		{1, 3, RoundUp, 0, 1, roundAway, roundAway},
	}
	for _, c := range cases {
		gotOdd := roundDir(c.rem, c.div, c.mode, c.oddQ)
		require.Equalf(t, c.wantOdd, gotOdd,
			"roundDir(%d,%d,%v,odd=%d): want %v", c.rem, c.div, c.mode, c.oddQ, c.wantOdd)
		gotEven := roundDir(c.rem, c.div, c.mode, c.evenQ)
		require.Equalf(t, c.wantEven, gotEven,
			"roundDir(%d,%d,%v,even=%d): want %v", c.rem, c.div, c.mode, c.evenQ, c.wantEven)
	}
}
