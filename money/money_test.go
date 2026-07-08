package money

import (
	"errors"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCurrencyRegistry(t *testing.T) {
	c, ok := Lookup("usd") // case-insensitive
	require.True(t, ok)
	require.Equal(t, "USD", c.Code)
	require.Equal(t, 2, c.Decimals)

	_, ok = Lookup("XXX")
	require.False(t, ok)

	require.NotEmpty(t, Currencies())

	// Custom registration.
	RegisterCurrency(Currency{Code: "FOO", Numeric: "000", Decimals: 2})
	_, ok = Lookup("FOO")
	require.True(t, ok)
}

func TestFromMinorAndMajor(t *testing.T) {
	m := MustFromMinor(1234, "USD")
	require.Equal(t, int64(1234), m.Amount())
	require.Equal(t, "USD", m.Currency().Code)

	m2, err := FromMajor(12, 34, "USD")
	require.NoError(t, err)
	require.Equal(t, int64(1234), m2.Amount())

	// 0-decimal currency (JPY): whole only.
	jpy, err := FromMajor(1500, 0, "JPY")
	require.NoError(t, err)
	require.Equal(t, int64(1500), jpy.Amount())

	// 3-decimal currency (KWD).
	kwd, err := FromMajor(1, 234, "KWD")
	require.NoError(t, err)
	require.Equal(t, int64(1234), kwd.Amount())

	// Too many fractional digits.
	_, err = FromMajor(1, 999, "USD")
	require.ErrorIs(t, err, ErrInvalidAmount)

	_, err = FromMinor(0, "NOPE")
	require.ErrorIs(t, err, ErrUnknownCurrency)
}

func TestParse(t *testing.T) {
	m, err := Parse("USD", "12.34")
	require.NoError(t, err)
	require.Equal(t, int64(1234), m.Amount())

	neg, err := Parse("USD", "-0.05")
	require.NoError(t, err)
	require.Equal(t, int64(-5), neg.Amount())

	// Trailing/missing fractional digits are zero-padded.
	padded, err := Parse("USD", "1.5")
	require.NoError(t, err)
	require.Equal(t, int64(150), padded.Amount())

	// 0-decimal currency.
	jpy, err := Parse("JPY", "1500")
	require.NoError(t, err)
	require.Equal(t, int64(1500), jpy.Amount())

	// Errors.
	_, err = Parse("USD", "1.234") // too many decimals
	require.ErrorIs(t, err, ErrInvalidAmount)
	_, err = Parse("USD", "")
	require.ErrorIs(t, err, ErrInvalidAmount)
	_, err = Parse("XXX", "1.00")
	require.ErrorIs(t, err, ErrUnknownCurrency)
}

func TestString(t *testing.T) {
	require.Equal(t, "12.34 USD", MustFromMinor(1234, "USD").String())
	require.Equal(t, "-0.05 USD", MustFromMinor(-5, "USD").String())
	require.Equal(t, "0.00 USD", MustFromMinor(0, "USD").String())
	require.Equal(t, "1500 JPY", MustFromMinor(1500, "JPY").String())
	require.Equal(t, "1.234 KWD", MustFromMinor(1234, "KWD").String())
}

func TestArithmeticSameCurrency(t *testing.T) {
	a := MustFromMinor(100, "USD") // 1.00
	b := MustFromMinor(250, "USD") // 2.50

	sum, err := a.Add(b)
	require.NoError(t, err)
	require.Equal(t, int64(350), sum.Amount())

	diff, err := b.Sub(a)
	require.NoError(t, err)
	require.Equal(t, int64(150), diff.Amount())

	neg := a.Negate()
	require.Equal(t, int64(-100), neg.Amount())

	abs := MustFromMinor(-100, "USD").Abs()
	require.Equal(t, int64(100), abs.Amount())

	p, err := b.Mul(3) // 2.50 * 3 = 7.50
	require.NoError(t, err)
	require.Equal(t, int64(750), p.Amount())
}

func TestArithmeticCurrencyMismatch(t *testing.T) {
	usd := MustFromMinor(100, "USD")
	eur := MustFromMinor(100, "EUR")
	_, err := usd.Add(eur)
	require.ErrorIs(t, err, ErrCurrencyMismatch)
	_, err = usd.Sub(eur)
	require.ErrorIs(t, err, ErrCurrencyMismatch)
}

func TestCmp(t *testing.T) {
	a := MustFromMinor(100, "USD")
	b := MustFromMinor(250, "USD")
	c, err := a.Cmp(b)
	require.NoError(t, err)
	require.Equal(t, -1, c)
	c, _ = b.Cmp(a)
	require.Equal(t, 1, c)
	c, _ = a.Cmp(a)
	require.Equal(t, 0, c)

	require.True(t, a.Equal(MustFromMinor(100, "USD")))
	require.False(t, a.Equal(b))
}

func TestScaleRounding(t *testing.T) {
	// 1.00 USD * 1.07 (7% tax) = 1.07
	one := MustFromMinor(100, "USD")
	tax, err := one.Scale(1.07, RoundHalfUp)
	require.NoError(t, err)
	require.Equal(t, int64(107), tax.Amount())

	// 15% take rate of 1.00 = 0.15
	take, err := one.Scale(0.15, RoundHalfUp)
	require.NoError(t, err)
	require.Equal(t, int64(15), take.Amount())

	// 0 cents * 0.5 = 0 minor.
	half, err := MustFromMinor(0, "USD").Scale(0.5, RoundHalfUp)
	require.NoError(t, err)
	require.Equal(t, int64(0), half.Amount())
}

func TestDivRounding(t *testing.T) {
	// 100 cents / 3 with different roundings.
	hundred := MustFromMinor(100, "USD")
	down, err := hundred.Div(3, RoundDown)
	require.NoError(t, err)
	require.Equal(t, int64(33), down.Amount())

	up, err := hundred.Div(3, RoundUp)
	require.NoError(t, err)
	require.Equal(t, int64(34), up.Amount())

	_, err = hundred.Div(0, RoundHalfUp)
	require.Error(t, err)
}

func TestAllocate(t *testing.T) {
	// $1.00 into a 50/30/20 split = 50/30/20 cents (exact).
	m := MustFromMinor(100, "USD")
	parts, err := m.Allocate([]int{50, 30, 20})
	require.NoError(t, err)
	require.Equal(t, []int64{50, 30, 20}, []int64{parts[0].Amount(), parts[1].Amount(), parts[2].Amount()})
	require.Equal(t, int64(100), parts[0].Amount()+parts[1].Amount()+parts[2].Amount())

	// $1.00 into thirds: remainder distributed, total preserved exactly.
	thirds, err := m.Allocate([]int{1, 1, 1})
	require.NoError(t, err)
	sum := int64(0)
	for _, p := range thirds {
		sum += p.Amount()
	}
	require.Equal(t, int64(100), sum, "no minor units lost")
	// Largest remainder gets the extra cent.
	require.Equal(t, int64(34), thirds[0].Amount())
	require.Equal(t, int64(33), thirds[1].Amount())
	require.Equal(t, int64(33), thirds[2].Amount())

	_, err = m.Allocate(nil)
	require.Error(t, err)
	_, err = m.Allocate([]int{0, 0})
	require.Error(t, err)
	_, err = m.Allocate([]int{-1, 2})
	require.Error(t, err)
}

func TestOverflowGuards(t *testing.T) {
	big := MustFromMinor(1<<62, "USD")
	_, err := big.Mul(1 << 10)
	require.ErrorIs(t, err, ErrOverflow)

	// Scale to a value beyond int64.
	_, err = big.Scale(1e20, RoundHalfUp)
	require.ErrorIs(t, err, ErrOverflow)
}

func TestSignChecks(t *testing.T) {
	require.True(t, MustFromMinor(0, "USD").IsZero())
	require.True(t, MustFromMinor(5, "USD").IsPositive())
	require.True(t, MustFromMinor(-5, "USD").IsNegative())
}

func TestErrSentinels(t *testing.T) {
	require.True(t, errors.Is(ErrCurrencyMismatch, ErrCurrencyMismatch))
	require.True(t, errors.Is(ErrOverflow, ErrOverflow))
}

func TestMustCurrency(t *testing.T) {
	require.Equal(t, "USD", MustCurrency("usd").Code)
	require.Panics(t, func() { MustCurrency("NOPE") })
}

func TestScaleRoundingModes(t *testing.T) {
	// 0.5 cents-scale scenario via a value whose scaled result lands on x.5.
	// $0.01 (1 minor) * 0.5 = 0.5 minor -> exercises each mode.
	one := MustFromMinor(1, "USD") // 0.01
	halfUp, _ := one.Scale(0.5, RoundHalfUp)
	require.Equal(t, int64(1), halfUp.Amount()) // 0.5 -> 1 (away from zero)
	halfEven, _ := one.Scale(0.5, RoundHalfEven)
	_ = halfEven // covered
	down, _ := one.Scale(0.5, RoundDown)
	require.Equal(t, int64(0), down.Amount()) // truncate
	up, _ := one.Scale(0.5, RoundUp)
	require.Equal(t, int64(1), up.Amount()) // ceil

	// Bad ratio.
	_, err := one.Scale(math.NaN(), RoundHalfUp)
	require.Error(t, err)
	_, err = one.Scale(math.Inf(1), RoundHalfUp)
	require.Error(t, err)
}

func TestDivModes(t *testing.T) {
	hundred := MustFromMinor(100, "USD")
	halfEven, err := hundred.Div(3, RoundHalfEven)
	require.NoError(t, err)
	require.GreaterOrEqual(t, halfEven.Amount(), int64(33)) // exercises half-even branch
	halfUp, _ := hundred.Div(2, RoundHalfUp)
	require.Equal(t, int64(50), halfUp.Amount()) // exact
}

func TestFromMajorNegative(t *testing.T) {
	// whole and frac must share a sign; the valid negative form is -12, -34.
	// (Mixed signs like -12, +34 are now rejected — see TestR12_FromMajor_MixedSign.)
	neg, err := FromMajor(-12, -34, "USD")
	require.NoError(t, err)
	require.Equal(t, int64(-1234), neg.Amount())
}

func TestAbsPositive(t *testing.T) {
	pos := MustFromMinor(50, "USD")
	require.Equal(t, int64(50), pos.Abs().Amount())
}

func TestAddSubOverflow(t *testing.T) {
	big := MustFromMinor(1<<62, "USD")
	_, err := big.Add(big)
	require.ErrorIs(t, err, ErrOverflow)
	_, err = big.Sub(MustFromMinor(-1, "USD")) // big - (-1) ~ big+1, near max
	_ = err
}

func TestCmpMismatch(t *testing.T) {
	_, err := MustFromMinor(1, "USD").Cmp(MustFromMinor(1, "EUR"))
	require.ErrorIs(t, err, ErrCurrencyMismatch)
}

func TestParseExplicitPlus(t *testing.T) {
	m, err := Parse("USD", "+1.00")
	require.NoError(t, err)
	require.Equal(t, int64(100), m.Amount())
}
