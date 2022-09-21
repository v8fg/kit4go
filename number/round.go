package number

import (
	"bytes"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Int marks the integer type or underlying integer type.
type Int interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64
}

// Uint marks the un signed integer type or underlying un signed integer type.
type Uint interface {
	~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64
}

// Float marks the float type or underlying float type.
type Float interface {
	~float32 | ~float64
}

var regForNumber *regexp.Regexp
var regForNumber6 = regexp.MustCompile(`([+\-])?(?:(0|[1-9]\d*)?(?:\.?)(\d*)?|\.\d+)(?:[eE]([+\-])?(\d+))?`)
var regForNumber7 = regexp.MustCompile(`([+\-])?(?:(0|[1-9]\d*)?(\.?)(\d*)?|\.\d+)(?:[eE]([+\-])?(\d+))?`)

func init() {
	// subMatch size shall 6
	regForNumber = regForNumber6
}

// SetRegForNumber sets which the regex string will use for parse the string number.
//
//	true: regForNumber7
//	false: regForNumber6
func SetRegForNumber(useRegForNumber7 bool) {
	if useRegForNumber7 {
		// subMatch size shall 7
		regForNumber = regForNumber7
	} else {
		regForNumber = regForNumber6
	}
}

// Round returns the nearest float64 with the given input and precision, type shall float32 | float64
func Round[T Float](f T, precision uint) float64 {
	n10 := math.Pow10(int(precision))
	// return float64(int(float64(f)*n10+math.Copysign(0.5, float64(f)*n10))) / n10
	return math.Round(float64(f)*n10) / n10
}

// RoundToEven returns the nearest float64(rounding ties to even) with the given input and precision, type shall float32 | float64
func RoundToEven[T Float](f T, precision uint) float64 {
	n10 := math.Pow10(int(precision))
	// return float64(int(float64(f)*n10+math.Copysign(0.5, float64(f)*n10))) / n10
	return math.RoundToEven(float64(f)*n10) / n10
}

// RoundFloor returns the down float64 with the given input and precision, type shall float32 | float64
func RoundFloor[T Float](f T, precision uint) float64 {
	n10 := math.Pow10(int(precision))
	return math.Floor(float64(f)*n10) / n10
}

// RoundCeil returns the ceil float64 with the given input and precision, type shall float32 | float64
func RoundCeil[T Float](f T, precision uint) float64 {
	n10 := math.Pow10(int(precision))
	return math.Ceil(float64(f)*n10) / n10
}

// RoundTrunc returns the truncate float64 with the given input and precision,
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

// RoundTruncStr returns the truncate string with the given input and precision,
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
	matches := regForNumber.FindStringSubmatch(s)

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

// RestoreToRealNumberStr converts the numeric input to the corresponding string, as the storage mechanism, some pos maybe override.
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

	matches := regForNumber.FindStringSubmatch(s)

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
