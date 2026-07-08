// Package money provides exact, immutable fixed-point monetary arithmetic in a
// chosen ISO 4217 currency.
//
// A Money value stores its amount in minor units (cents) as an int64 — so
// addition, subtraction, multiplication by an integer, and comparison have no
// floating-point drift and need no allocation. Multiplication by a ratio (tax,
// discount, FX) and division apply an explicit rounding mode. Allocate splits a
// value into shares without losing a minor unit.
//
// Ad-tech / finance / commerce uses: exact eCPM/CPM accounting, spend and
// budget math, tax/discount application, payout splitting — anywhere a float
// rounding error would be a billing bug. The int64 minor-unit floor (~9.2e18)
// is far above any realistic fiat amount.
package money

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Currency is an ISO 4217 currency: a 3-letter code, its 3-digit numeric code,
// and the minor-unit decimal count (0 for JPY, 2 for USD, 3 for KWD).
type Currency struct {
	Code     string
	Numeric  string
	Decimals int
}

var (
	// ErrCurrencyMismatch is returned by operations combining two Money values
	// denominated in different currencies.
	ErrCurrencyMismatch = errors.New("money: currency mismatch")
	// ErrUnknownCurrency is returned when a currency code is not registered.
	ErrUnknownCurrency = errors.New("money: unknown currency")
	// ErrOverflow is returned when a result does not fit in int64 minor units.
	ErrOverflow = errors.New("money: overflow")
	// ErrInvalidAmount is returned for an unparseable amount string.
	ErrInvalidAmount = errors.New("money: invalid amount")
	// ErrDivideByZero is returned by Div when the divisor is 0.
	ErrDivideByZero = errors.New("money: divide by zero")
	// ErrNoRatios is returned by Allocate when the ratios slice is empty.
	ErrNoRatios = errors.New("money: no ratios")
	// ErrNegativeRatio is returned by Allocate when a ratio is negative.
	ErrNegativeRatio = errors.New("money: negative ratio")
	// ErrZeroRatios is returned by Allocate when all ratios sum to zero.
	ErrZeroRatios = errors.New("money: ratios sum to zero")
)

// Rounding selects how a non-integer minor-unit result is rounded.
type Rounding int

const (
	// RoundHalfUp rounds 0.5 away from zero (the common commercial default).
	RoundHalfUp Rounding = iota
	// RoundHalfEven rounds 0.5 to the nearest even (banker's rounding).
	RoundHalfEven
	// RoundDown truncates toward zero.
	RoundDown
	// RoundUp rounds away from zero.
	RoundUp
)

// Money is an amount in minor units of its Currency. The zero value is 0 of an
// unspecified currency; always construct with FromMinor, FromMajor, or Parse.
type Money struct {
	amount int64
	cur    Currency
}

// FromMinor builds a Money from an int64 already in minor units (e.g. cents).
//
// math.MinInt64 is rejected: it has no symmetric positive counterpart, so Abs,
// Negate, and String all misbehave on it (Abs stays negative, String emits a
// double-negated magnitude). Realistic fiat amounts are far inside the int64
// range, so rejecting this single boundary value is lossless in practice.
func FromMinor(minor int64, code string) (Money, error) {
	c, ok := Lookup(code)
	if !ok {
		return Money{}, fmt.Errorf("%w: %s", ErrUnknownCurrency, code)
	}
	if minor == math.MinInt64 {
		return Money{}, fmt.Errorf("%w: minor amount out of range", ErrOverflow)
	}
	return Money{amount: minor, cur: c}, nil
}

// MustFromMinor is FromMinor that panics on an unknown currency. Use for
// compile-time-known codes.
func MustFromMinor(minor int64, code string) Money {
	m, err := FromMinor(minor, code)
	if err != nil {
		panic(err)
	}
	return m
}

// FromMajor builds a Money from a whole + fractional component already split by
// the caller (avoids parsing). frac is in major-unit fractions and is scaled to
// the currency's decimals; e.g. FromMajor(12, 34, "USD") == 1234 cents.
// frac must have at most the currency's decimals digits.
func FromMajor(whole, frac int, code string) (Money, error) {
	c, ok := Lookup(code)
	if !ok {
		return Money{}, fmt.Errorf("%w: %s", ErrUnknownCurrency, code)
	}
	if c.Decimals == 0 {
		if frac != 0 {
			return Money{}, fmt.Errorf("%w: fractional part for 0-decimal currency", ErrInvalidAmount)
		}
		if int64(whole) == math.MinInt64 {
			return Money{}, fmt.Errorf("%w: minor amount out of range", ErrOverflow)
		}
		return Money{amount: int64(whole), cur: c}, nil
	}
	// whole and frac must share a sign (frac==0 is sign-neutral). A mixed sign
	// like FromMajor(12, -34) is ambiguous and was previously swallowed,
	// silently producing whole*10^dec - frac (e.g. 1166) instead of an error.
	if (whole < 0) != (frac < 0) && whole != 0 && frac != 0 {
		return Money{}, fmt.Errorf("%w: whole and fractional parts have mismatched signs", ErrInvalidAmount)
	}
	fracStr := strconv.Itoa(absInt(frac))
	if len(fracStr) > c.Decimals {
		return Money{}, fmt.Errorf("%w: too many fractional digits", ErrInvalidAmount)
	}
	fracStr = strings.Repeat("0", c.Decimals-len(fracStr)) + fracStr
	scaledFrac, err := strconv.ParseInt(fracStr, 10, 64)
	if err != nil {
		return Money{}, fmt.Errorf("%w: %w", ErrInvalidAmount, err)
	}
	minor := int64(whole)
	for range c.Decimals {
		minor, err = mulChecked(minor, 10)
		if err != nil {
			return Money{}, err
		}
	}
	if whole < 0 || frac < 0 {
		minor -= scaledFrac
	} else {
		minor += scaledFrac
	}
	if minor == math.MinInt64 {
		return Money{}, fmt.Errorf("%w: minor amount out of range", ErrOverflow)
	}
	return Money{amount: minor, cur: c}, nil
}

// Parse parses a decimal amount string (e.g. "12.34", "-0.05") in the given
// currency into minor units. The fractional part must not exceed the currency's
// decimals.
func Parse(code, amount string) (Money, error) {
	c, ok := Lookup(code)
	if !ok {
		return Money{}, fmt.Errorf("%w: %s", ErrUnknownCurrency, code)
	}
	s := strings.TrimSpace(amount)
	if s == "" {
		return Money{}, ErrInvalidAmount
	}
	neg := false
	if s[0] == '-' {
		neg = true
		s = s[1:]
	} else if s[0] == '+' {
		s = s[1:]
	}
	parts := strings.SplitN(s, ".", 2)
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || whole < 0 {
		return Money{}, fmt.Errorf("%w: %q", ErrInvalidAmount, amount)
	}
	frac := int64(0)
	if len(parts) == 2 {
		fs := parts[1]
		if len(fs) > c.Decimals {
			return Money{}, fmt.Errorf("%w: too many fractional digits", ErrInvalidAmount)
		}
		fs = fs + strings.Repeat("0", c.Decimals-len(fs))
		frac, err = strconv.ParseInt(fs, 10, 64)
		if err != nil || frac < 0 {
			return Money{}, fmt.Errorf("%w: %q", ErrInvalidAmount, amount)
		}
	}
	minor := whole
	for range c.Decimals {
		minor, err = mulChecked(minor, 10)
		if err != nil {
			return Money{}, err
		}
	}
	minor += frac
	if neg {
		minor = -minor
	}
	if minor == math.MinInt64 {
		return Money{}, fmt.Errorf("%w: minor amount out of range", ErrOverflow)
	}
	return Money{amount: minor, cur: c}, nil
}

// Amount returns the minor-unit amount (e.g. cents).
func (m Money) Amount() int64 { return m.amount }

// Currency returns the currency.
func (m Money) Currency() Currency { return m.cur }

// IsZero reports whether the amount is zero.
func (m Money) IsZero() bool { return m.amount == 0 }

// IsPositive reports whether the amount is greater than zero.
func (m Money) IsPositive() bool { return m.amount > 0 }

// IsNegative reports whether the amount is less than zero.
func (m Money) IsNegative() bool { return m.amount < 0 }

// String renders the amount in major units with the currency's decimals and
// code, e.g. "12.34 USD". Always shows exactly Decimals digits.
func (m Money) String() string {
	abs := m.amount
	sign := ""
	if abs < 0 {
		sign = "-"
		abs = -abs
	}
	if m.cur.Decimals == 0 {
		return fmt.Sprintf("%s%d %s", sign, abs, m.cur.Code)
	}
	div := int64(1)
	for range m.cur.Decimals {
		div *= 10
	}
	whole := abs / div
	frac := abs % div
	return fmt.Sprintf("%s%d.%0*d %s", sign, whole, m.cur.Decimals, frac, m.cur.Code)
}

// Add returns m + other; requires the same currency.
func (m Money) Add(other Money) (Money, error) {
	if err := m.same(other); err != nil {
		return Money{}, err
	}
	sum, err := addChecked(m.amount, other.amount)
	if err != nil {
		return Money{}, err
	}
	return Money{amount: sum, cur: m.cur}, nil
}

// Sub returns m - other; requires the same currency.
func (m Money) Sub(other Money) (Money, error) {
	if err := m.same(other); err != nil {
		return Money{}, err
	}
	// Subtract without pre-negating other.amount: -math.MinInt64 overflows to
	// math.MinInt64, so a naive addChecked(m.amount, -other.amount) would wrap
	// silently and miss the overflow (e.g. MaxInt64 - MinInt64).
	diff, err := subChecked(m.amount, other.amount)
	if err != nil {
		return Money{}, err
	}
	return Money{amount: diff, cur: m.cur}, nil
}

// Mul multiplies the amount by an integer scalar (exact).
func (m Money) Mul(factor int64) (Money, error) {
	p, err := mulChecked(m.amount, factor)
	if err != nil {
		return Money{}, err
	}
	return Money{amount: p, cur: m.cur}, nil
}

// Scale multiplies the amount by a non-integer ratio (e.g. 1.07 for 7% tax, or
// 0.15 for a 15% take rate) and rounds to the currency's minor unit.
func (m Money) Scale(ratio float64, mode Rounding) (Money, error) {
	if math.IsNaN(ratio) || math.IsInf(ratio, 0) {
		return Money{}, fmt.Errorf("%w: bad ratio", ErrInvalidAmount)
	}
	scaled := float64(m.amount) * ratio
	rounded, err := roundFloatToInt64(scaled, mode)
	if err != nil {
		return Money{}, err
	}
	return Money{amount: rounded, cur: m.cur}, nil
}

// Div divides the amount by an integer divisor, rounding the remainder.
//
// Integer division truncates toward zero, so the rounding decision operates on
// the absolute remainder. When a step away from the truncation is required the
// adjustment is sign-aware: it moves the quotient away from zero (q-- for a
// negative true result, q++ otherwise), never toward zero.
func (m Money) Div(divisor int64, mode Rounding) (Money, error) {
	if divisor == 0 {
		return Money{}, ErrDivideByZero
	}
	q := m.amount / divisor
	r := m.amount % divisor
	negative := (m.amount < 0) != (divisor < 0) // sign of the true (non-truncated) result
	switch roundDir(r, divisor, mode, q) {
	case roundAway:
		if negative {
			q--
		} else {
			q++
		}
	}
	return Money{amount: q, cur: m.cur}, nil
}

// Negate returns the additive inverse.
func (m Money) Negate() Money { return Money{amount: -m.amount, cur: m.cur} }

// Abs returns the absolute value.
func (m Money) Abs() Money {
	if m.amount < 0 {
		return Money{amount: -m.amount, cur: m.cur}
	}
	return m
}

// Cmp compares by amount; requires the same currency. Returns -1, 0, +1.
func (m Money) Cmp(other Money) (int, error) {
	if err := m.same(other); err != nil {
		return 0, err
	}
	switch {
	case m.amount < other.amount:
		return -1, nil
	case m.amount > other.amount:
		return 1, nil
	default:
		return 0, nil
	}
}

// Equal reports amount-and-currency equality.
func (m Money) Equal(other Money) bool { return m.amount == other.amount && m.cur == other.cur }

// Allocate splits m into len(ratios) parts proportional to the ratios, without
// losing any minor unit (the remainder is distributed to the largest shares).
// ratios must be non-negative and sum > 0.
func (m Money) Allocate(ratios []int) ([]Money, error) {
	if len(ratios) == 0 {
		return nil, ErrNoRatios
	}
	total := 0
	for _, r := range ratios {
		if r < 0 {
			return nil, ErrNegativeRatio
		}
		total += r
	}
	if total == 0 {
		return nil, ErrZeroRatios
	}
	out := make([]Money, len(ratios))
	allocated := int64(0)
	for i, r := range ratios {
		product, err := mulChecked(m.amount, int64(r))
		if err != nil {
			return nil, err
		}
		share := product / int64(total)
		out[i] = Money{amount: share, cur: m.cur}
		allocated += share
	}
	// Distribute the rounding remainder (>=0 for positive amounts) one unit at a
	// time to the ratios with the largest fractional remainder.
	remainder := m.amount - allocated
	if remainder != 0 {
		type idxFrac struct {
			i    int
			frac float64
		}
		fracs := make([]idxFrac, len(ratios))
		for i, r := range ratios {
			fracs[i] = idxFrac{i, float64(m.amount)*float64(r)/float64(total) - float64(out[i].amount)}
		}
		// Sort by frac desc (simple selection of top |remainder|).
		step := int64(1)
		if remainder < 0 {
			step = -1
		}
		for remaining := absI64(remainder); remaining > 0; remaining-- {
			best := 0
			for j := 1; j < len(fracs); j++ {
				if fracs[j].frac > fracs[best].frac {
					best = j
				}
			}
			out[best].amount += step
			fracs[best].frac -= float64(step)
		}
	}
	return out, nil
}

func (m Money) same(other Money) error {
	if m.cur != other.cur {
		return fmt.Errorf("%w: %s vs %s", ErrCurrencyMismatch, m.cur.Code, other.cur.Code)
	}
	return nil
}

// --- checked arithmetic & rounding helpers ---

func addChecked(a, b int64) (int64, error) {
	s := a + b
	if (s > a) != (b > 0) {
		return 0, ErrOverflow
	}
	return s, nil
}

// subChecked computes a-b with overflow detection that does NOT rely on
// negating b (avoiding the math.MinInt64 wrap-around trap).
func subChecked(a, b int64) (int64, error) {
	d := a - b
	// Overflow occurred iff a and b have opposite signs AND the result's sign
	// disagrees with a's sign. Formally: (a^b)&(a^d) sign-bit set.
	if (a^b)&(a^d) < 0 {
		return 0, ErrOverflow
	}
	return d, nil
}

func mulChecked(a, b int64) (int64, error) {
	if a == 0 || b == 0 {
		return 0, nil
	}
	p := a * b
	if p/b != a {
		return 0, ErrOverflow
	}
	return p, nil
}

func roundFloatToInt64(f float64, mode Rounding) (int64, error) {
	if f >= math.MaxInt64+1 || f <= math.MinInt64 {
		return 0, ErrOverflow
	}
	switch mode {
	case RoundDown:
		return int64(math.Trunc(f)), nil
	case RoundUp:
		if f >= 0 {
			return int64(math.Ceil(f)), nil
		}
		return int64(math.Floor(f)), nil
	case RoundHalfEven:
		return int64(math.RoundToEven(f)), nil
	default: // RoundHalfUp
		return int64(math.Round(f)), nil
	}
}

// roundDir reports whether the quotient of an integer division should move one
// step away from zero to honor the rounding mode. The truncated quotient q is
// required so that RoundHalfEven can resolve an exact half toward the nearest
// EVEN quotient (banker's rounding): if absRem*2 == absDiv and q is odd, the
// result rounds away from zero to make it even; if q is already even it stays.
//
// The remainder/divisor signs don't affect the decision because truncation
// toward zero makes the magnitude behavior uniform — absRem and absDiv carry
// the relevant information. Callers apply the direction in the away-from-zero
// sense themselves (they know the sign of the true result).
type roundDirection int

const (
	roundStay roundDirection = iota
	roundAway
)

func roundDir(rem, divisor int64, mode Rounding, q int64) roundDirection {
	if rem == 0 {
		return roundStay
	}
	absRem := absI64(rem)
	absDiv := absI64(divisor)
	switch mode {
	case RoundDown:
		return roundStay
	case RoundUp:
		return roundAway
	case RoundHalfEven:
		if absRem*2 != absDiv {
			return boolRoundDir(absRem*2 > absDiv)
		}
		// Exact half: round to the nearest even quotient. q is the truncated
		// (toward-zero) quotient; moving away from zero toggles its parity.
		// Round away only when q is currently odd, which makes the result even.
		if q%2 != 0 {
			return roundAway
		}
		return roundStay
	default: // RoundHalfUp
		return boolRoundDir(absRem*2 >= absDiv)
	}
}

func boolRoundDir(b bool) roundDirection {
	if b {
		return roundAway
	}
	return roundStay
}

// shouldRoundUp reports whether an integer division remainder rounds the
// quotient away from zero. It is the legacy parity-unaware view used by helpers
// that don't have the truncated quotient: for RoundHalfEven at an exact half it
// returns true (the half-up fallback), since the true banker's-rounding answer
// depends on q's parity (see roundDir). New code should call roundDir instead.
func shouldRoundUp(rem, divisor int64, mode Rounding) bool {
	if mode == RoundHalfEven {
		if rem == 0 {
			return false
		}
		if absI64(rem)*2 == absI64(divisor) {
			return true // exact half: legacy half-up fallback (parity unknown)
		}
	}
	return roundDir(rem, divisor, mode, 0) == roundAway
}

func absI64(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
