package number

import (
	"bytes"
	"fmt"
	"math"
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

	ret, _ := strconv.ParseFloat(sb.String(), 64)
	if ret == -0 {
		return 0
	}
	return T(ret)
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
