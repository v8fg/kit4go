package number

import (
	"bytes"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
)

// Int is a type constraint that matches any signed integer type or any type
// whose underlying type is a signed integer (int, int8, int16, int32, int64).
type Int interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64
}

// Uint is a type constraint that matches any unsigned integer type or any type
// whose underlying type is an unsigned integer (uint, uint8, uint16, uint32,
// uint64).
type Uint interface {
	~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64
}

// Float is a type constraint that matches any floating-point type or any type
// whose underlying type is a float (float32, float64).
type Float interface {
	~float32 | ~float64
}

// regForNumber holds the active regex used to parse string numbers. It is accessed
// concurrently (RestoreToRealNumberStr / regSplitNormalNumber read it while
// SetRegForNumber may swap it), so it is stored in an atomic pointer to keep all
// reads and writes race-free.
var regForNumber atomic.Pointer[regexp.Regexp]
var regForNumber6 = regexp.MustCompile(`([+\-])?(?:(0|[1-9]\d*)?(?:\.?)(\d*)?|\.\d+)(?:[eE]([+\-])?(\d+))?`)
var regForNumber7 = regexp.MustCompile(`([+\-])?(?:(0|[1-9]\d*)?(\.?)(\d*)?|\.\d+)(?:[eE]([+\-])?(\d+))?`)

func init() {
	// subMatch size shall 6
	regForNumber.Store(regForNumber6)
}

// SetRegForNumber selects which regular expression is used to parse string
// numbers by [RestoreToRealNumberStr] and the truncating rounders. The swap is
// atomic and safe to call concurrently with readers.
//
//	true: regForNumber7
//	false: regForNumber6
func SetRegForNumber(useRegForNumber7 bool) {
	if useRegForNumber7 {
		// subMatch size shall 7
		regForNumber.Store(regForNumber7)
	} else {
		regForNumber.Store(regForNumber6)
	}
}

// Round returns f rounded to the given number of decimal places using
// round-half-away-from-zero semantics (via math.Round), as a float64. T must be
// float32 or float64.
func Round[T Float](f T, precision uint) float64 {
	n10 := math.Pow10(int(precision))
	// return float64(int(float64(f)*n10+math.Copysign(0.5, float64(f)*n10))) / n10
	return math.Round(float64(f)*n10) / n10
}

// RoundToEven returns f rounded to the given number of decimal places using
// round-half-to-even (banker's) semantics (via math.RoundToEven), as a float64.
// T must be float32 or float64.
func RoundToEven[T Float](f T, precision uint) float64 {
	n10 := math.Pow10(int(precision))
	// return float64(int(float64(f)*n10+math.Copysign(0.5, float64(f)*n10))) / n10
	return math.RoundToEven(float64(f)*n10) / n10
}

// RoundFloor returns f rounded down (towards negative infinity) to the given
// number of decimal places (via math.Floor), as a float64. T must be float32 or
// float64.
func RoundFloor[T Float](f T, precision uint) float64 {
	n10 := math.Pow10(int(precision))
	return math.Floor(float64(f)*n10) / n10
}

// RoundCeil returns f rounded up (towards positive infinity) to the given
// number of decimal places (via math.Ceil), as a float64. T must be float32 or
// float64.
func RoundCeil[T Float](f T, precision uint) float64 {
	n10 := math.Pow10(int(precision))
	return math.Ceil(float64(f)*n10) / n10
}

// RoundTrunc returns f truncated towards zero to precision decimal places,
// preserving the same type T. A positive precision keeps that many fractional
// digits; a negative precision truncates that many integer digits (replacing
// them with zeros), and an over-large negative precision yields the zero value
// of T. The conversion is string-based and therefore immune to float
// representation drift.
//
// type can:
//
//	~int | ~int8 | ~int16 | ~int32 | ~int64
//	~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64
//	~float32 | ~float64
//
// support exponential.
//
// valid precision: float32(6), float64(15)
func RoundTrunc[T Int | Uint | Float](f T, precision int) T {
	s := RestoreToRealNumberStr(f)
	if s == "0" {
		return T(0)
	}

	if len(s) == 0 || s == "NaN" || s == "+Inf" || s == "-Inf" {
		return f
	}

	// For integer-typed T, ParseFloat would lose precision for values whose
	// magnitude exceeds 2^53 (e.g. RoundTrunc(int64(1<<53+1), 0) dropped to
	// 1<<53). The string-based truncation logic above is still used to compute
	// the result string, but we parse it back with the integer parsing path
	// (ParseInt/ParseUint) so large integers survive losslessly. Truncating an
	// integer at precision >= 0 is a no-op, so short-circuit and return f
	// unchanged without any string round-trip.
	if precision >= 0 && isIntegerKind(f) {
		return f
	}

	sig, integer, fractional := regSplitNormalNumber(s)
	sb := bytes.NewBufferString(sig)

	if precision > 0 {
		sb.WriteString(integer)
		if precision < len(fractional) {
			sb.WriteString(".")
			sb.WriteString(fractional[:precision])
		} else {
			sb.WriteString(".")
			sb.WriteString(fractional)
			sb.WriteString(strings.Repeat("0", precision-len(fractional)))
		}
	} else {
		if -precision < len(integer) {
			sb.WriteString(integer[:len(integer)+precision])
			sb.WriteString(strings.Repeat("0", -precision))
		} else {
			return 0
		}
	}

	// Integer types: parse the (integer) result string losslessly via the
	// integer path instead of ParseFloat, which would corrupt magnitudes > 2^53.
	// The negative-precision branch above always produces an integer string for
	// integer inputs, so ParseInt/ParseUint applies.
	if isIntegerKind(f) {
		if v, ok := parseIntLossless[T](sb.String()); ok {
			return v
		}
		// Unparseable integer string: fall through to the float path rather
		// than silently returning a wrong zero value.
	}

	ret, _ := strconv.ParseFloat(sb.String(), 64)
	if ret == -0 {
		return 0
	}
	return T(ret)
}

// isIntegerKind reports whether f's kind is a signed or unsigned integer. It is
// the runtime kind check backing the integer short-circuit in [RoundTrunc];
// generics cannot express it statically without splitting the API.
func isIntegerKind[T any](f T) bool {
	switch reflect.TypeOf(f).Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return true
	}
	return false
}

// parseIntLossless parses the decimal integer string s into T without going
// through float64, preserving magnitudes beyond 2^53. s may carry a leading
// sign. ok is false when s is empty or the parse fails (e.g. overflow), letting
// the caller fall back to the float path.
func parseIntLossless[T Int | Uint | Float](s string) (T, bool) {
	if len(s) == 0 {
		var zero T
		return zero, false
	}
	// Distinguish signed vs unsigned by the presence of a leading '-': an
	// unsigned integer can never be negative, so signed parsing is only correct
	// for inputs that carry (or could carry) a sign. We parse signed into int64
	// and unsigned into uint64, then convert to T.
	if s[0] == '-' {
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			var zero T
			return zero, false
		}
		return T(v), true
	}
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		var zero T
		return zero, false
	}
	return T(v), true
}

// RoundTruncStr is like [RoundTrunc] but returns the truncated result as a
// decimal string instead of converting it back to T, avoiding any loss of
// precision from float parsing.
//
// type can:
//
//	~int | ~int8 | ~int16 | ~int32 | ~int64
//	~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64
//	~float32 | ~float64
func RoundTruncStr[T Int | Uint | Float](f T, precision int) string {
	s := RestoreToRealNumberStr(f)

	if s == "0" || len(s) == 0 || s == "NaN" || s == "+Inf" || s == "-Inf" {
		return s
	}

	sig, integer, fractional := regSplitNormalNumber(s)
	sb := bytes.NewBufferString(sig)

	if precision > 0 {
		sb.WriteString(integer)
		if precision < len(fractional) {
			sb.WriteString(".")
			sb.WriteString(fractional[:precision])
		} else {
			sb.WriteString(".")
			sb.WriteString(fractional)
			sb.WriteString(strings.Repeat("0", precision-len(fractional)))
		}
	} else {
		if -precision < len(integer) {
			sb.WriteString(integer[:len(integer)+precision])
			sb.WriteString(strings.Repeat("0", -precision))
		} else {
			return "0"
		}
	}
	if sb.String() == "-0" {
		return "0"
	}
	return sb.String()
}

// regSplitNormalNumber input shall only contain: sig, integer and fractional parts
func regSplitNormalNumber(s string) (sig, integer, fractional string) {
	re := regForNumber.Load()
	matches := re.FindStringSubmatch(s)

	if len(matches) == 6 {
		sig = matches[1]
		integer = matches[2]
		fractional = matches[3]
	} else {
		sig = matches[1]
		integer = matches[2]
		fractional = matches[4]
	}
	return
}

// RestoreToRealNumberStr converts the numeric input f to its canonical decimal
// string form, expanding any exponential notation so the result has no
// exponent and at most one fractional part. It is the string primitive used by
// the truncating rounders and never loses precision to float formatting
// because it operates on the textual representation of f.
//
// type can:
//
//	~int | ~int8 | ~int16 | ~int32 | ~int64
//	~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64
//	~float32 | ~float64
//
// regexp matches: 0=match string, 1(+/-)=number sig, 2(int)=integer, 3(.)=dot, 4=(int)fractional or decimal,
// 5(+/-)=exponential sig, 6((int))=exponential value
//
// use: regexp.MustCompile(`([+\-])?(?:(0|[1-9]\d*)?(\.?)(\d*)?|\.\d+)(?:[eE]([+\-])?(\d+))?`)
//
// regexp matches: 0=match string, 1(+/-)=number sig, 2(int)=integer, 3=(int)fractional or decimal,
// 4(+/-)=exponential sig, 5((int))=exponential value
//
// use: regexp.MustCompile(`([+\-])?(?:(0|[1-9]\d*)?(?:\.?)(\d*)?|\.\d+)(?:[eE]([+\-])?(\d+))?`)
func RestoreToRealNumberStr[T Int | Uint | Float](f T) string {
	s := fmt.Sprint(f)
	if s == "NaN" || s == "+Inf" || s == "-Inf" {
		return s
	}

	if s == "0" || s == "-0" || s == "+0" {
		return "0"
	}

	re := regForNumber.Load()
	matches := re.FindStringSubmatch(s)

	var sig string
	var integer string
	var fractional string
	var expSig string
	var expVal int

	if len(matches) == 6 {
		sig = matches[1]
		integer = matches[2]
		fractional = matches[3]
		expSig = matches[4]
		_expVal, _ := strconv.ParseInt(matches[5], 10, 64)
		expVal = int(_expVal)
	} else {
		sig = matches[1]
		integer = matches[2]
		// dot := matches[3]
		fractional = matches[4]
		expSig = matches[5]
		_expVal, _ := strconv.ParseInt(matches[6], 10, 64)
		expVal = int(_expVal)
	}

	sb := bytes.NewBufferString(sig)
	if len(expSig) == 1 {
		if expSig[0] == '+' {
			sb.WriteString(integer)
			if expVal < len(fractional) {
				sb.WriteString(fractional[0:expVal])
				sb.WriteString(".")
				sb.WriteString(fractional[expVal:])
			} else {
				sb.WriteString(fractional)
				sb.WriteString(strings.Repeat("0", expVal-len(fractional)))
			}
		} else {
			if expVal >= len(integer) {
				sb.WriteString("0.")
				sb.WriteString(strings.Repeat("0", expVal-len(integer)))
				sb.WriteString(integer)
				// } else {
				// 	// not fact with IEEE-754
				// 	sb.WriteString(integer[:expVal-len(integer)])
				// 	sb.WriteString(".")
			}
			sb.WriteString(fractional)
		}
	} else {
		sb.WriteString(integer)
		if len(fractional) > 0 {
			sb.WriteString(".")
			sb.WriteString(fractional)
		}
	}

	return sb.String()
}
