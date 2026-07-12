// Package decimal provides arbitrary-precision decimal arithmetic for financial
// calculations where float64 drift is unacceptable. Immutable values backed by
// math/big. Pure standard library.
//
// Ad-tech / finance uses: CPM/eCPM computation, tax/discount rates, FX
// conversion, bid clearing prices — anywhere a float rounding error would be a
// billing or settlement bug.
package decimal

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
)

// Decimal is an immutable arbitrary-precision decimal backed by math/big.
// Scale is the number of decimal places; all Decimal values in a computation
// should share the same scale for exact results.
type Decimal struct {
	value *big.Int // unscaled value (value / 10^scale = actual number)
	scale int      // number of decimal places
}

var (
	// ErrParse is returned when a string cannot be parsed as a decimal.
	ErrParse = errors.New("decimal: parse error")
	// ErrScaleMismatch is returned by operations on decimals with different scales.
	ErrScaleMismatch = errors.New("decimal: scale mismatch")
	// ErrNegativeScale is returned (by Parse) or panicked (by New) when a scale
	// is negative. A negative scale makes String() produce an invalid result, so
	// it is rejected at construction rather than rendered.
	ErrNegativeScale = errors.New("decimal: negative scale")
	// ErrDivideByZero is returned by Div when the divisor is 0.
	ErrDivideByZero = errors.New("decimal: divide by zero")
)

var (
	ten = big.NewInt(10)
)

// precisionTable[i] = 10^i (precomputed to avoid repeated big.Int Exp).
var precisionTable [20]*big.Int

func init() {
	for i := range 20 {
		precisionTable[i] = new(big.Int).Exp(ten, big.NewInt(int64(i)), nil)
	}
}

func pow10(n int) *big.Int {
	if n >= 0 && n < len(precisionTable) {
		return precisionTable[n]
	}
	return new(big.Int).Exp(ten, big.NewInt(int64(n)), nil)
}

// New creates a Decimal from an unscaled int64 value and a scale. For example,
// New(12345, 3) = 12.345. It panics if scale is negative, since a negative scale
// has no valid decimal rendering (String() would slice out of range). Use a
// non-negative scale; this matches the kit4go constructor convention of
// rejecting structurally invalid parameters up front.
func New(unscaled int64, scale int) Decimal {
	if scale < 0 {
		panic(fmt.Errorf("%w: scale %d (unscaled %d)", ErrNegativeScale, scale, unscaled))
	}
	return Decimal{value: big.NewInt(unscaled), scale: scale}
}

// FromInt creates a Decimal with scale 0 from an int64.
func FromInt(v int64) Decimal { return New(v, 0) }

// MustParse parses s at the given scale; panics on error. Use for constants.
func MustParse(s string, scale int) Decimal {
	d, err := Parse(s, scale)
	if err != nil {
		panic(err)
	}
	return d
}

// Parse parses a decimal string (e.g. "12.345", "-0.05") into a Decimal with
// the given scale. The input is rescaled: if the input has fewer decimal places
// than scale, trailing zeros are added; if more, it is an error. It returns an
// error wrapping ErrNegativeScale if scale is negative.
func Parse(s string, scale int) (Decimal, error) {
	if scale < 0 {
		return Decimal{}, fmt.Errorf("%w: %d", ErrNegativeScale, scale)
	}
	if s == "" {
		return Decimal{}, fmt.Errorf("%w: empty string", ErrParse)
	}
	neg := false
	if strings.HasPrefix(s, "-") {
		neg = true
		s = s[1:]
	} else if strings.HasPrefix(s, "+") {
		s = s[1:]
	}
	// Reject a second/doubled sign (e.g. "++5", "-+5", "+-5"): big.Int.SetString
	// accepts a leading sign, so without this guard "+-5" parses as -5.00 and
	// "--5" cancels to +5.00. Also rejects a bare sign.
	if len(s) == 0 || s[0] == '+' || s[0] == '-' {
		return Decimal{}, fmt.Errorf("%w: invalid number %q", ErrParse, s)
	}
	parts := strings.SplitN(s, ".", 2)
	whole := parts[0]
	frac := ""
	if len(parts) == 2 {
		frac = parts[1]
	}
	if len(frac) > scale {
		return Decimal{}, fmt.Errorf("%w: too many fractional digits (%d > scale %d)", ErrParse, len(frac), scale)
	}
	// Pad frac to scale.
	frac = frac + strings.Repeat("0", scale-len(frac))
	combined := whole + frac
	v, ok := new(big.Int).SetString(combined, 10)
	if !ok {
		return Decimal{}, fmt.Errorf("%w: invalid number %q", ErrParse, s)
	}
	if neg {
		v.Neg(v)
	}
	return Decimal{value: v, scale: scale}, nil
}

// String renders the decimal as a string with exactly scale decimal places.
// It is defensive against a non-positive scale: scale <= 0 renders the unscaled
// value as a whole number, so a malformed Decimal can never panic here.
func (d Decimal) String() string {
	if d.value == nil {
		return "0"
	}
	v := d.value
	neg := v.Sign() < 0
	abs := new(big.Int).Abs(v)
	str := abs.String()
	if d.scale <= 0 {
		if neg {
			return "-" + str
		}
		return str
	}
	// Pad with leading zeros to ensure at least scale+1 digits.
	for len(str) <= d.scale {
		str = "0" + str
	}
	result := str[:len(str)-d.scale] + "." + str[len(str)-d.scale:]
	if neg {
		result = "-" + result
	}
	return result
}

// Unscaled returns a copy of the unscaled big.Int value. The returned value is
// independent of d's internal state: mutating it does not affect d (or any value
// d was derived from), honoring Decimal's Immutable contract. A zero-value
// Decimal{} returns a fresh big.Int(0).
func (d Decimal) Unscaled() *big.Int {
	if d.value == nil {
		return big.NewInt(0)
	}
	return new(big.Int).Set(d.value)
}

// Scale returns the decimal scale.
func (d Decimal) Scale() int { return d.scale }

// Sign returns -1, 0, or +1.
func (d Decimal) Sign() int {
	if d.value == nil {
		return 0
	}
	return d.value.Sign()
}

// IsZero reports whether d is zero.
func (d Decimal) IsZero() bool { return d.Sign() == 0 }

// valueOrZero returns d.value, treating a nil (zero-value Decimal{}) receiver as
// big.Int(0). This keeps the arithmetic methods nil-safe: a struct/map zero value
// behaves identically to FromInt(0). The returned pointer is never mutated by the
// caller in these methods (each builds a fresh big.Int for the result), so sharing
// the package-level zero is safe and allocation-free on the zero-value path.
func (d Decimal) valueOrZero() *big.Int {
	if d.value != nil {
		return d.value
	}
	return big.NewInt(0)
}

// Add returns d + other (must share the same scale). A zero-value Decimal{}
// operand is treated as 0.
func (d Decimal) Add(other Decimal) (Decimal, error) {
	if d.scale != other.scale {
		return Decimal{}, fmt.Errorf("%w: %d vs %d", ErrScaleMismatch, d.scale, other.scale)
	}
	return Decimal{value: new(big.Int).Add(d.valueOrZero(), other.valueOrZero()), scale: d.scale}, nil
}

// Sub returns d - other (must share the same scale). A zero-value Decimal{}
// operand is treated as 0.
func (d Decimal) Sub(other Decimal) (Decimal, error) {
	if d.scale != other.scale {
		return Decimal{}, fmt.Errorf("%w: %d vs %d", ErrScaleMismatch, d.scale, other.scale)
	}
	return Decimal{value: new(big.Int).Sub(d.valueOrZero(), other.valueOrZero()), scale: d.scale}, nil
}

// Mul multiplies by an integer factor. The result preserves d's scale. A zero-value
// Decimal{} receiver is treated as 0.
func (d Decimal) Mul(factor int64) Decimal {
	return Decimal{value: new(big.Int).Mul(d.valueOrZero(), big.NewInt(factor)), scale: d.scale}
}

// MulDecimal returns d * other. Result scale = d.scale + other.scale. A zero-value
// Decimal{} operand is treated as 0.
func (d Decimal) MulDecimal(other Decimal) Decimal {
	return Decimal{
		value: new(big.Int).Mul(d.valueOrZero(), other.valueOrZero()),
		scale: d.scale + other.scale,
	}
}

// Div divides d by divisor, truncating to the original scale. It returns
// ErrDivideByZero (matchable via errors.Is) when divisor is 0. A zero-value
// Decimal{} receiver is treated as 0.
func (d Decimal) Div(divisor int64) (Decimal, error) {
	if divisor == 0 {
		return Decimal{}, ErrDivideByZero
	}
	// big.Int division truncates toward zero, which matches the scale-preserving
	// integer division of unscaled values.
	q := new(big.Int).Quo(d.valueOrZero(), big.NewInt(divisor))
	return Decimal{value: q, scale: d.scale}, nil
}

// Cmp compares d and other (must share the same scale). Returns -1, 0, +1. A
// zero-value Decimal{} operand is treated as 0.
func (d Decimal) Cmp(other Decimal) (int, error) {
	if d.scale != other.scale {
		return 0, fmt.Errorf("%w: %d vs %d", ErrScaleMismatch, d.scale, other.scale)
	}
	return d.valueOrZero().Cmp(other.valueOrZero()), nil
}

// Negate returns -d. A zero-value Decimal{} receiver is treated as 0.
func (d Decimal) Negate() Decimal {
	return Decimal{value: new(big.Int).Neg(d.valueOrZero()), scale: d.scale}
}

// Abs returns |d|. A zero-value Decimal{} receiver is treated as 0.
func (d Decimal) Abs() Decimal {
	if d.valueOrZero().Sign() < 0 {
		return Decimal{value: new(big.Int).Abs(d.valueOrZero()), scale: d.scale}
	}
	if d.value == nil {
		// Avoid returning the nil-value receiver; yield a real zero at d's scale.
		return Decimal{value: big.NewInt(0), scale: d.scale}
	}
	return d
}

// Rescale changes the scale of d to newScale. If newScale > d.scale, trailing
// zeros are appended (multiply by 10^(newScale-d.scale)). If newScale < d.scale,
// the value is truncated (divide by 10^(d.scale-newScale)). A negative newScale is
// a no-op: it is returned unchanged (d), since a negative scale has no valid
// decimal rendering. A zero-value Decimal{} receiver is treated as 0.
func (d Decimal) Rescale(newScale int) Decimal {
	if newScale < 0 || newScale == d.scale {
		return d
	}
	v := d.valueOrZero()
	if newScale > d.scale {
		diff := newScale - d.scale
		return Decimal{value: new(big.Int).Mul(v, pow10(diff)), scale: newScale}
	}
	diff := d.scale - newScale
	return Decimal{value: new(big.Int).Quo(v, pow10(diff)), scale: newScale}
}
