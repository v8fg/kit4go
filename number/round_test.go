package number_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/v8fg/kit4go/number"
)

type args[T float32 | float64] struct {
	f         T
	precision uint
}

var roundTestData []args[float64]

func init() {
	roundTestData = []args[float64]{
		{math.Pi, 0},
		{math.Pi, 1},
		{math.Pi, 2},
		{math.Pi, 3},
		{math.Inf(-1), 0},
		{math.Inf(1), 0},
		{0.0001, 0},
		{0.0001e-3, 0},
		{0.0000000123, 0},
		{0, 0},
		{0.5, 0},
		{0.5, 1},
		{0.5, 2},
		{0.99, 0},
		{0.99, 1},
		{0.99, 2},
		{1.5, 0},
		{1.5, 1},
		{1.5, 2},
		{2.5, 0},
		{2.5, 1},
		{2.5, 2},
		{66, 0},
		{66, 1},
		{66, 2},
	}
}

func ExampleRound() {
	for i := 0; i < len(roundTestData); i++ {
		num := roundTestData[i].f
		precision := roundTestData[i].precision
		fmt.Printf("[Round] result:%+8v, precision:%+3v, num:%+18v\n", number.Round(num, precision), precision, num)

		num *= -1
		fmt.Printf("[Round] result:%+8v, precision:%+3v, num:%+18v\n", number.Round(num, precision), precision, num)
	}

	// Output:
	// [Round] result:       3, precision:  0, num: 3.141592653589793
	// [Round] result:      -3, precision:  0, num:-3.141592653589793
	// [Round] result:     3.1, precision:  1, num: 3.141592653589793
	// [Round] result:    -3.1, precision:  1, num:-3.141592653589793
	// [Round] result:    3.14, precision:  2, num: 3.141592653589793
	// [Round] result:   -3.14, precision:  2, num:-3.141592653589793
	// [Round] result:   3.142, precision:  3, num: 3.141592653589793
	// [Round] result:  -3.142, precision:  3, num:-3.141592653589793
	// [Round] result:    -Inf, precision:  0, num:              -Inf
	// [Round] result:    +Inf, precision:  0, num:              +Inf
	// [Round] result:    +Inf, precision:  0, num:              +Inf
	// [Round] result:    -Inf, precision:  0, num:              -Inf
	// [Round] result:       0, precision:  0, num:            0.0001
	// [Round] result:      -0, precision:  0, num:           -0.0001
	// [Round] result:       0, precision:  0, num:             1e-07
	// [Round] result:      -0, precision:  0, num:            -1e-07
	// [Round] result:       0, precision:  0, num:          1.23e-08
	// [Round] result:      -0, precision:  0, num:         -1.23e-08
	// [Round] result:       0, precision:  0, num:                 0
	// [Round] result:      -0, precision:  0, num:                -0
	// [Round] result:       1, precision:  0, num:               0.5
	// [Round] result:      -1, precision:  0, num:              -0.5
	// [Round] result:     0.5, precision:  1, num:               0.5
	// [Round] result:    -0.5, precision:  1, num:              -0.5
	// [Round] result:     0.5, precision:  2, num:               0.5
	// [Round] result:    -0.5, precision:  2, num:              -0.5
	// [Round] result:       1, precision:  0, num:              0.99
	// [Round] result:      -1, precision:  0, num:             -0.99
	// [Round] result:       1, precision:  1, num:              0.99
	// [Round] result:      -1, precision:  1, num:             -0.99
	// [Round] result:    0.99, precision:  2, num:              0.99
	// [Round] result:   -0.99, precision:  2, num:             -0.99
	// [Round] result:       2, precision:  0, num:               1.5
	// [Round] result:      -2, precision:  0, num:              -1.5
	// [Round] result:     1.5, precision:  1, num:               1.5
	// [Round] result:    -1.5, precision:  1, num:              -1.5
	// [Round] result:     1.5, precision:  2, num:               1.5
	// [Round] result:    -1.5, precision:  2, num:              -1.5
	// [Round] result:       3, precision:  0, num:               2.5
	// [Round] result:      -3, precision:  0, num:              -2.5
	// [Round] result:     2.5, precision:  1, num:               2.5
	// [Round] result:    -2.5, precision:  1, num:              -2.5
	// [Round] result:     2.5, precision:  2, num:               2.5
	// [Round] result:    -2.5, precision:  2, num:              -2.5
	// [Round] result:      66, precision:  0, num:                66
	// [Round] result:     -66, precision:  0, num:               -66
	// [Round] result:      66, precision:  1, num:                66
	// [Round] result:     -66, precision:  1, num:               -66
	// [Round] result:      66, precision:  2, num:                66
	// [Round] result:     -66, precision:  2, num:               -66
}

func ExampleRoundToEven() {
	for i := 0; i < len(roundTestData); i++ {
		num := roundTestData[i].f
		precision := roundTestData[i].precision
		fmt.Printf("[RoundToEven] result:%+8v, precision:%+3v, num:%+18v\n", number.RoundToEven(num, precision), precision, num)

		num *= -1
		fmt.Printf("[RoundToEven] result:%+8v, precision:%+3v, num:%+18v\n", number.RoundToEven(num, precision), precision, num)
	}

	// Output:
	// [RoundToEven] result:       3, precision:  0, num: 3.141592653589793
	// [RoundToEven] result:      -3, precision:  0, num:-3.141592653589793
	// [RoundToEven] result:     3.1, precision:  1, num: 3.141592653589793
	// [RoundToEven] result:    -3.1, precision:  1, num:-3.141592653589793
	// [RoundToEven] result:    3.14, precision:  2, num: 3.141592653589793
	// [RoundToEven] result:   -3.14, precision:  2, num:-3.141592653589793
	// [RoundToEven] result:   3.142, precision:  3, num: 3.141592653589793
	// [RoundToEven] result:  -3.142, precision:  3, num:-3.141592653589793
	// [RoundToEven] result:    -Inf, precision:  0, num:              -Inf
	// [RoundToEven] result:    +Inf, precision:  0, num:              +Inf
	// [RoundToEven] result:    +Inf, precision:  0, num:              +Inf
	// [RoundToEven] result:    -Inf, precision:  0, num:              -Inf
	// [RoundToEven] result:       0, precision:  0, num:            0.0001
	// [RoundToEven] result:      -0, precision:  0, num:           -0.0001
	// [RoundToEven] result:       0, precision:  0, num:             1e-07
	// [RoundToEven] result:      -0, precision:  0, num:            -1e-07
	// [RoundToEven] result:       0, precision:  0, num:          1.23e-08
	// [RoundToEven] result:      -0, precision:  0, num:         -1.23e-08
	// [RoundToEven] result:       0, precision:  0, num:                 0
	// [RoundToEven] result:      -0, precision:  0, num:                -0
	// [RoundToEven] result:       0, precision:  0, num:               0.5
	// [RoundToEven] result:      -0, precision:  0, num:              -0.5
	// [RoundToEven] result:     0.5, precision:  1, num:               0.5
	// [RoundToEven] result:    -0.5, precision:  1, num:              -0.5
	// [RoundToEven] result:     0.5, precision:  2, num:               0.5
	// [RoundToEven] result:    -0.5, precision:  2, num:              -0.5
	// [RoundToEven] result:       1, precision:  0, num:              0.99
	// [RoundToEven] result:      -1, precision:  0, num:             -0.99
	// [RoundToEven] result:       1, precision:  1, num:              0.99
	// [RoundToEven] result:      -1, precision:  1, num:             -0.99
	// [RoundToEven] result:    0.99, precision:  2, num:              0.99
	// [RoundToEven] result:   -0.99, precision:  2, num:             -0.99
	// [RoundToEven] result:       2, precision:  0, num:               1.5
	// [RoundToEven] result:      -2, precision:  0, num:              -1.5
	// [RoundToEven] result:     1.5, precision:  1, num:               1.5
	// [RoundToEven] result:    -1.5, precision:  1, num:              -1.5
	// [RoundToEven] result:     1.5, precision:  2, num:               1.5
	// [RoundToEven] result:    -1.5, precision:  2, num:              -1.5
	// [RoundToEven] result:       2, precision:  0, num:               2.5
	// [RoundToEven] result:      -2, precision:  0, num:              -2.5
	// [RoundToEven] result:     2.5, precision:  1, num:               2.5
	// [RoundToEven] result:    -2.5, precision:  1, num:              -2.5
	// [RoundToEven] result:     2.5, precision:  2, num:               2.5
	// [RoundToEven] result:    -2.5, precision:  2, num:              -2.5
	// [RoundToEven] result:      66, precision:  0, num:                66
	// [RoundToEven] result:     -66, precision:  0, num:               -66
	// [RoundToEven] result:      66, precision:  1, num:                66
	// [RoundToEven] result:     -66, precision:  1, num:               -66
	// [RoundToEven] result:      66, precision:  2, num:                66
	// [RoundToEven] result:     -66, precision:  2, num:               -66
}

func ExampleRoundFloor() {
	for i := 0; i < len(roundTestData); i++ {
		num := roundTestData[i].f
		precision := roundTestData[i].precision
		fmt.Printf("[RoundFloor] result:%+8v, precision:%+3v, num:%+18v\n", number.RoundFloor(num, precision), precision, num)

		num *= -1
		fmt.Printf("[RoundFloor] result:%+8v, precision:%+3v, num:%+18v\n", number.RoundFloor(num, precision), precision, num)
	}

	// Output:
	// [RoundFloor] result:       3, precision:  0, num: 3.141592653589793
	// [RoundFloor] result:      -4, precision:  0, num:-3.141592653589793
	// [RoundFloor] result:     3.1, precision:  1, num: 3.141592653589793
	// [RoundFloor] result:    -3.2, precision:  1, num:-3.141592653589793
	// [RoundFloor] result:    3.14, precision:  2, num: 3.141592653589793
	// [RoundFloor] result:   -3.15, precision:  2, num:-3.141592653589793
	// [RoundFloor] result:   3.141, precision:  3, num: 3.141592653589793
	// [RoundFloor] result:  -3.142, precision:  3, num:-3.141592653589793
	// [RoundFloor] result:    -Inf, precision:  0, num:              -Inf
	// [RoundFloor] result:    +Inf, precision:  0, num:              +Inf
	// [RoundFloor] result:    +Inf, precision:  0, num:              +Inf
	// [RoundFloor] result:    -Inf, precision:  0, num:              -Inf
	// [RoundFloor] result:       0, precision:  0, num:            0.0001
	// [RoundFloor] result:      -1, precision:  0, num:           -0.0001
	// [RoundFloor] result:       0, precision:  0, num:             1e-07
	// [RoundFloor] result:      -1, precision:  0, num:            -1e-07
	// [RoundFloor] result:       0, precision:  0, num:          1.23e-08
	// [RoundFloor] result:      -1, precision:  0, num:         -1.23e-08
	// [RoundFloor] result:       0, precision:  0, num:                 0
	// [RoundFloor] result:      -0, precision:  0, num:                -0
	// [RoundFloor] result:       0, precision:  0, num:               0.5
	// [RoundFloor] result:      -1, precision:  0, num:              -0.5
	// [RoundFloor] result:     0.5, precision:  1, num:               0.5
	// [RoundFloor] result:    -0.5, precision:  1, num:              -0.5
	// [RoundFloor] result:     0.5, precision:  2, num:               0.5
	// [RoundFloor] result:    -0.5, precision:  2, num:              -0.5
	// [RoundFloor] result:       0, precision:  0, num:              0.99
	// [RoundFloor] result:      -1, precision:  0, num:             -0.99
	// [RoundFloor] result:     0.9, precision:  1, num:              0.99
	// [RoundFloor] result:      -1, precision:  1, num:             -0.99
	// [RoundFloor] result:    0.99, precision:  2, num:              0.99
	// [RoundFloor] result:   -0.99, precision:  2, num:             -0.99
	// [RoundFloor] result:       1, precision:  0, num:               1.5
	// [RoundFloor] result:      -2, precision:  0, num:              -1.5
	// [RoundFloor] result:     1.5, precision:  1, num:               1.5
	// [RoundFloor] result:    -1.5, precision:  1, num:              -1.5
	// [RoundFloor] result:     1.5, precision:  2, num:               1.5
	// [RoundFloor] result:    -1.5, precision:  2, num:              -1.5
	// [RoundFloor] result:       2, precision:  0, num:               2.5
	// [RoundFloor] result:      -3, precision:  0, num:              -2.5
	// [RoundFloor] result:     2.5, precision:  1, num:               2.5
	// [RoundFloor] result:    -2.5, precision:  1, num:              -2.5
	// [RoundFloor] result:     2.5, precision:  2, num:               2.5
	// [RoundFloor] result:    -2.5, precision:  2, num:              -2.5
	// [RoundFloor] result:      66, precision:  0, num:                66
	// [RoundFloor] result:     -66, precision:  0, num:               -66
	// [RoundFloor] result:      66, precision:  1, num:                66
	// [RoundFloor] result:     -66, precision:  1, num:               -66
	// [RoundFloor] result:      66, precision:  2, num:                66
	// [RoundFloor] result:     -66, precision:  2, num:               -66
}

func ExampleRoundCeil() {
	for i := 0; i < len(roundTestData); i++ {
		num := roundTestData[i].f
		precision := roundTestData[i].precision
		fmt.Printf("[RoundCeil] result:%+8v, precision:%+3v, num:%+18v\n", number.RoundCeil(num, precision), precision, num)

		num *= -1
		fmt.Printf("[RoundCeil] result:%+8v, precision:%+3v, num:%+18v\n", number.RoundCeil(num, precision), precision, num)
	}

	// Output:
	// [RoundCeil] result:       4, precision:  0, num: 3.141592653589793
	// [RoundCeil] result:      -3, precision:  0, num:-3.141592653589793
	// [RoundCeil] result:     3.2, precision:  1, num: 3.141592653589793
	// [RoundCeil] result:    -3.1, precision:  1, num:-3.141592653589793
	// [RoundCeil] result:    3.15, precision:  2, num: 3.141592653589793
	// [RoundCeil] result:   -3.14, precision:  2, num:-3.141592653589793
	// [RoundCeil] result:   3.142, precision:  3, num: 3.141592653589793
	// [RoundCeil] result:  -3.141, precision:  3, num:-3.141592653589793
	// [RoundCeil] result:    -Inf, precision:  0, num:              -Inf
	// [RoundCeil] result:    +Inf, precision:  0, num:              +Inf
	// [RoundCeil] result:    +Inf, precision:  0, num:              +Inf
	// [RoundCeil] result:    -Inf, precision:  0, num:              -Inf
	// [RoundCeil] result:       1, precision:  0, num:            0.0001
	// [RoundCeil] result:      -0, precision:  0, num:           -0.0001
	// [RoundCeil] result:       1, precision:  0, num:             1e-07
	// [RoundCeil] result:      -0, precision:  0, num:            -1e-07
	// [RoundCeil] result:       1, precision:  0, num:          1.23e-08
	// [RoundCeil] result:      -0, precision:  0, num:         -1.23e-08
	// [RoundCeil] result:       0, precision:  0, num:                 0
	// [RoundCeil] result:      -0, precision:  0, num:                -0
	// [RoundCeil] result:       1, precision:  0, num:               0.5
	// [RoundCeil] result:      -0, precision:  0, num:              -0.5
	// [RoundCeil] result:     0.5, precision:  1, num:               0.5
	// [RoundCeil] result:    -0.5, precision:  1, num:              -0.5
	// [RoundCeil] result:     0.5, precision:  2, num:               0.5
	// [RoundCeil] result:    -0.5, precision:  2, num:              -0.5
	// [RoundCeil] result:       1, precision:  0, num:              0.99
	// [RoundCeil] result:      -0, precision:  0, num:             -0.99
	// [RoundCeil] result:       1, precision:  1, num:              0.99
	// [RoundCeil] result:    -0.9, precision:  1, num:             -0.99
	// [RoundCeil] result:    0.99, precision:  2, num:              0.99
	// [RoundCeil] result:   -0.99, precision:  2, num:             -0.99
	// [RoundCeil] result:       2, precision:  0, num:               1.5
	// [RoundCeil] result:      -1, precision:  0, num:              -1.5
	// [RoundCeil] result:     1.5, precision:  1, num:               1.5
	// [RoundCeil] result:    -1.5, precision:  1, num:              -1.5
	// [RoundCeil] result:     1.5, precision:  2, num:               1.5
	// [RoundCeil] result:    -1.5, precision:  2, num:              -1.5
	// [RoundCeil] result:       3, precision:  0, num:               2.5
	// [RoundCeil] result:      -2, precision:  0, num:              -2.5
	// [RoundCeil] result:     2.5, precision:  1, num:               2.5
	// [RoundCeil] result:    -2.5, precision:  1, num:              -2.5
	// [RoundCeil] result:     2.5, precision:  2, num:               2.5
	// [RoundCeil] result:    -2.5, precision:  2, num:              -2.5
	// [RoundCeil] result:      66, precision:  0, num:                66
	// [RoundCeil] result:     -66, precision:  0, num:               -66
	// [RoundCeil] result:      66, precision:  1, num:                66
	// [RoundCeil] result:     -66, precision:  1, num:               -66
	// [RoundCeil] result:      66, precision:  2, num:                66
	// [RoundCeil] result:     -66, precision:  2, num:               -66
}

func ExampleRoundTrunc() {
	for i := 0; i < len(roundTestData); i++ {
		num := roundTestData[i].f
		precision := int(roundTestData[i].precision)
		fmt.Printf("[RoundTrunc] result:%+8v, precision:%+3v, num:%+18v\n", number.RoundTrunc(num, precision), precision, num)

		precision *= -1
		fmt.Printf("[RoundTrunc] result:%+8v, precision:%+3v, num:%+18v\n", number.RoundTrunc(num, precision), precision, num)

		num *= -1
		fmt.Printf("[RoundTrunc] result:%+8v, precision:%+3v, num:%+18v\n", number.RoundTrunc(num, precision), precision, num)

		precision *= -1
		fmt.Printf("[RoundTrunc] result:%+8v, precision:%+3v, num:%+18v\n", number.RoundTrunc(num, precision), precision, num)
	}

	// Output:
	// [RoundTrunc] result:       3, precision:  0, num: 3.141592653589793
	// [RoundTrunc] result:       3, precision:  0, num: 3.141592653589793
	// [RoundTrunc] result:      -3, precision:  0, num:-3.141592653589793
	// [RoundTrunc] result:      -3, precision:  0, num:-3.141592653589793
	// [RoundTrunc] result:     3.1, precision:  1, num: 3.141592653589793
	// [RoundTrunc] result:       0, precision: -1, num: 3.141592653589793
	// [RoundTrunc] result:       0, precision: -1, num:-3.141592653589793
	// [RoundTrunc] result:    -3.1, precision:  1, num:-3.141592653589793
	// [RoundTrunc] result:    3.14, precision:  2, num: 3.141592653589793
	// [RoundTrunc] result:       0, precision: -2, num: 3.141592653589793
	// [RoundTrunc] result:       0, precision: -2, num:-3.141592653589793
	// [RoundTrunc] result:   -3.14, precision:  2, num:-3.141592653589793
	// [RoundTrunc] result:   3.141, precision:  3, num: 3.141592653589793
	// [RoundTrunc] result:       0, precision: -3, num: 3.141592653589793
	// [RoundTrunc] result:       0, precision: -3, num:-3.141592653589793
	// [RoundTrunc] result:  -3.141, precision:  3, num:-3.141592653589793
	// [RoundTrunc] result:    -Inf, precision:  0, num:              -Inf
	// [RoundTrunc] result:    -Inf, precision:  0, num:              -Inf
	// [RoundTrunc] result:    +Inf, precision:  0, num:              +Inf
	// [RoundTrunc] result:    +Inf, precision:  0, num:              +Inf
	// [RoundTrunc] result:    +Inf, precision:  0, num:              +Inf
	// [RoundTrunc] result:    +Inf, precision:  0, num:              +Inf
	// [RoundTrunc] result:    -Inf, precision:  0, num:              -Inf
	// [RoundTrunc] result:    -Inf, precision:  0, num:              -Inf
	// [RoundTrunc] result:       0, precision:  0, num:            0.0001
	// [RoundTrunc] result:       0, precision:  0, num:            0.0001
	// [RoundTrunc] result:       0, precision:  0, num:           -0.0001
	// [RoundTrunc] result:       0, precision:  0, num:           -0.0001
	// [RoundTrunc] result:       0, precision:  0, num:             1e-07
	// [RoundTrunc] result:       0, precision:  0, num:             1e-07
	// [RoundTrunc] result:       0, precision:  0, num:            -1e-07
	// [RoundTrunc] result:       0, precision:  0, num:            -1e-07
	// [RoundTrunc] result:       0, precision:  0, num:          1.23e-08
	// [RoundTrunc] result:       0, precision:  0, num:          1.23e-08
	// [RoundTrunc] result:       0, precision:  0, num:         -1.23e-08
	// [RoundTrunc] result:       0, precision:  0, num:         -1.23e-08
	// [RoundTrunc] result:       0, precision:  0, num:                 0
	// [RoundTrunc] result:       0, precision:  0, num:                 0
	// [RoundTrunc] result:       0, precision:  0, num:                -0
	// [RoundTrunc] result:       0, precision:  0, num:                -0
	// [RoundTrunc] result:       0, precision:  0, num:               0.5
	// [RoundTrunc] result:       0, precision:  0, num:               0.5
	// [RoundTrunc] result:       0, precision:  0, num:              -0.5
	// [RoundTrunc] result:       0, precision:  0, num:              -0.5
	// [RoundTrunc] result:     0.5, precision:  1, num:               0.5
	// [RoundTrunc] result:       0, precision: -1, num:               0.5
	// [RoundTrunc] result:       0, precision: -1, num:              -0.5
	// [RoundTrunc] result:    -0.5, precision:  1, num:              -0.5
	// [RoundTrunc] result:     0.5, precision:  2, num:               0.5
	// [RoundTrunc] result:       0, precision: -2, num:               0.5
	// [RoundTrunc] result:       0, precision: -2, num:              -0.5
	// [RoundTrunc] result:    -0.5, precision:  2, num:              -0.5
	// [RoundTrunc] result:       0, precision:  0, num:              0.99
	// [RoundTrunc] result:       0, precision:  0, num:              0.99
	// [RoundTrunc] result:       0, precision:  0, num:             -0.99
	// [RoundTrunc] result:       0, precision:  0, num:             -0.99
	// [RoundTrunc] result:     0.9, precision:  1, num:              0.99
	// [RoundTrunc] result:       0, precision: -1, num:              0.99
	// [RoundTrunc] result:       0, precision: -1, num:             -0.99
	// [RoundTrunc] result:    -0.9, precision:  1, num:             -0.99
	// [RoundTrunc] result:    0.99, precision:  2, num:              0.99
	// [RoundTrunc] result:       0, precision: -2, num:              0.99
	// [RoundTrunc] result:       0, precision: -2, num:             -0.99
	// [RoundTrunc] result:   -0.99, precision:  2, num:             -0.99
	// [RoundTrunc] result:       1, precision:  0, num:               1.5
	// [RoundTrunc] result:       1, precision:  0, num:               1.5
	// [RoundTrunc] result:      -1, precision:  0, num:              -1.5
	// [RoundTrunc] result:      -1, precision:  0, num:              -1.5
	// [RoundTrunc] result:     1.5, precision:  1, num:               1.5
	// [RoundTrunc] result:       0, precision: -1, num:               1.5
	// [RoundTrunc] result:       0, precision: -1, num:              -1.5
	// [RoundTrunc] result:    -1.5, precision:  1, num:              -1.5
	// [RoundTrunc] result:     1.5, precision:  2, num:               1.5
	// [RoundTrunc] result:       0, precision: -2, num:               1.5
	// [RoundTrunc] result:       0, precision: -2, num:              -1.5
	// [RoundTrunc] result:    -1.5, precision:  2, num:              -1.5
	// [RoundTrunc] result:       2, precision:  0, num:               2.5
	// [RoundTrunc] result:       2, precision:  0, num:               2.5
	// [RoundTrunc] result:      -2, precision:  0, num:              -2.5
	// [RoundTrunc] result:      -2, precision:  0, num:              -2.5
	// [RoundTrunc] result:     2.5, precision:  1, num:               2.5
	// [RoundTrunc] result:       0, precision: -1, num:               2.5
	// [RoundTrunc] result:       0, precision: -1, num:              -2.5
	// [RoundTrunc] result:    -2.5, precision:  1, num:              -2.5
	// [RoundTrunc] result:     2.5, precision:  2, num:               2.5
	// [RoundTrunc] result:       0, precision: -2, num:               2.5
	// [RoundTrunc] result:       0, precision: -2, num:              -2.5
	// [RoundTrunc] result:    -2.5, precision:  2, num:              -2.5
	// [RoundTrunc] result:      66, precision:  0, num:                66
	// [RoundTrunc] result:      66, precision:  0, num:                66
	// [RoundTrunc] result:     -66, precision:  0, num:               -66
	// [RoundTrunc] result:     -66, precision:  0, num:               -66
	// [RoundTrunc] result:      66, precision:  1, num:                66
	// [RoundTrunc] result:      60, precision: -1, num:                66
	// [RoundTrunc] result:     -60, precision: -1, num:               -66
	// [RoundTrunc] result:     -66, precision:  1, num:               -66
	// [RoundTrunc] result:      66, precision:  2, num:                66
	// [RoundTrunc] result:       0, precision: -2, num:                66
	// [RoundTrunc] result:       0, precision: -2, num:               -66
	// [RoundTrunc] result:     -66, precision:  2, num:               -66
}

func ExampleRoundTruncStr() {
	for i := 0; i < len(roundTestData); i++ {
		num := roundTestData[i].f
		precision := int(roundTestData[i].precision)
		fmt.Printf("[RoundTruncStr] result:%+8v, precision:%+3v, num:%+18v\n", number.RoundTruncStr(num, precision), precision, num)

		precision *= -1
		fmt.Printf("[RoundTruncStr] result:%+8v, precision:%+3v, num:%+18v\n", number.RoundTruncStr(num, precision), precision, num)

		num *= -1
		fmt.Printf("[RoundTruncStr] result:%+8v, precision:%+3v, num:%+18v\n", number.RoundTruncStr(num, precision), precision, num)

		precision *= -1
		fmt.Printf("[RoundTruncStr] result:%+8v, precision:%+3v, num:%+18v\n", number.RoundTruncStr(num, precision), precision, num)
	}

	// Output:
	// [RoundTruncStr] result:       3, precision:  0, num: 3.141592653589793
	// [RoundTruncStr] result:       3, precision:  0, num: 3.141592653589793
	// [RoundTruncStr] result:      -3, precision:  0, num:-3.141592653589793
	// [RoundTruncStr] result:      -3, precision:  0, num:-3.141592653589793
	// [RoundTruncStr] result:     3.1, precision:  1, num: 3.141592653589793
	// [RoundTruncStr] result:       0, precision: -1, num: 3.141592653589793
	// [RoundTruncStr] result:       0, precision: -1, num:-3.141592653589793
	// [RoundTruncStr] result:    -3.1, precision:  1, num:-3.141592653589793
	// [RoundTruncStr] result:    3.14, precision:  2, num: 3.141592653589793
	// [RoundTruncStr] result:       0, precision: -2, num: 3.141592653589793
	// [RoundTruncStr] result:       0, precision: -2, num:-3.141592653589793
	// [RoundTruncStr] result:   -3.14, precision:  2, num:-3.141592653589793
	// [RoundTruncStr] result:   3.141, precision:  3, num: 3.141592653589793
	// [RoundTruncStr] result:       0, precision: -3, num: 3.141592653589793
	// [RoundTruncStr] result:       0, precision: -3, num:-3.141592653589793
	// [RoundTruncStr] result:  -3.141, precision:  3, num:-3.141592653589793
	// [RoundTruncStr] result:    -Inf, precision:  0, num:              -Inf
	// [RoundTruncStr] result:    -Inf, precision:  0, num:              -Inf
	// [RoundTruncStr] result:    +Inf, precision:  0, num:              +Inf
	// [RoundTruncStr] result:    +Inf, precision:  0, num:              +Inf
	// [RoundTruncStr] result:    +Inf, precision:  0, num:              +Inf
	// [RoundTruncStr] result:    +Inf, precision:  0, num:              +Inf
	// [RoundTruncStr] result:    -Inf, precision:  0, num:              -Inf
	// [RoundTruncStr] result:    -Inf, precision:  0, num:              -Inf
	// [RoundTruncStr] result:       0, precision:  0, num:            0.0001
	// [RoundTruncStr] result:       0, precision:  0, num:            0.0001
	// [RoundTruncStr] result:       0, precision:  0, num:           -0.0001
	// [RoundTruncStr] result:       0, precision:  0, num:           -0.0001
	// [RoundTruncStr] result:       0, precision:  0, num:             1e-07
	// [RoundTruncStr] result:       0, precision:  0, num:             1e-07
	// [RoundTruncStr] result:       0, precision:  0, num:            -1e-07
	// [RoundTruncStr] result:       0, precision:  0, num:            -1e-07
	// [RoundTruncStr] result:       0, precision:  0, num:          1.23e-08
	// [RoundTruncStr] result:       0, precision:  0, num:          1.23e-08
	// [RoundTruncStr] result:       0, precision:  0, num:         -1.23e-08
	// [RoundTruncStr] result:       0, precision:  0, num:         -1.23e-08
	// [RoundTruncStr] result:       0, precision:  0, num:                 0
	// [RoundTruncStr] result:       0, precision:  0, num:                 0
	// [RoundTruncStr] result:       0, precision:  0, num:                -0
	// [RoundTruncStr] result:       0, precision:  0, num:                -0
	// [RoundTruncStr] result:       0, precision:  0, num:               0.5
	// [RoundTruncStr] result:       0, precision:  0, num:               0.5
	// [RoundTruncStr] result:       0, precision:  0, num:              -0.5
	// [RoundTruncStr] result:       0, precision:  0, num:              -0.5
	// [RoundTruncStr] result:     0.5, precision:  1, num:               0.5
	// [RoundTruncStr] result:       0, precision: -1, num:               0.5
	// [RoundTruncStr] result:       0, precision: -1, num:              -0.5
	// [RoundTruncStr] result:    -0.5, precision:  1, num:              -0.5
	// [RoundTruncStr] result:    0.50, precision:  2, num:               0.5
	// [RoundTruncStr] result:       0, precision: -2, num:               0.5
	// [RoundTruncStr] result:       0, precision: -2, num:              -0.5
	// [RoundTruncStr] result:   -0.50, precision:  2, num:              -0.5
	// [RoundTruncStr] result:       0, precision:  0, num:              0.99
	// [RoundTruncStr] result:       0, precision:  0, num:              0.99
	// [RoundTruncStr] result:       0, precision:  0, num:             -0.99
	// [RoundTruncStr] result:       0, precision:  0, num:             -0.99
	// [RoundTruncStr] result:     0.9, precision:  1, num:              0.99
	// [RoundTruncStr] result:       0, precision: -1, num:              0.99
	// [RoundTruncStr] result:       0, precision: -1, num:             -0.99
	// [RoundTruncStr] result:    -0.9, precision:  1, num:             -0.99
	// [RoundTruncStr] result:    0.99, precision:  2, num:              0.99
	// [RoundTruncStr] result:       0, precision: -2, num:              0.99
	// [RoundTruncStr] result:       0, precision: -2, num:             -0.99
	// [RoundTruncStr] result:   -0.99, precision:  2, num:             -0.99
	// [RoundTruncStr] result:       1, precision:  0, num:               1.5
	// [RoundTruncStr] result:       1, precision:  0, num:               1.5
	// [RoundTruncStr] result:      -1, precision:  0, num:              -1.5
	// [RoundTruncStr] result:      -1, precision:  0, num:              -1.5
	// [RoundTruncStr] result:     1.5, precision:  1, num:               1.5
	// [RoundTruncStr] result:       0, precision: -1, num:               1.5
	// [RoundTruncStr] result:       0, precision: -1, num:              -1.5
	// [RoundTruncStr] result:    -1.5, precision:  1, num:              -1.5
	// [RoundTruncStr] result:    1.50, precision:  2, num:               1.5
	// [RoundTruncStr] result:       0, precision: -2, num:               1.5
	// [RoundTruncStr] result:       0, precision: -2, num:              -1.5
	// [RoundTruncStr] result:   -1.50, precision:  2, num:              -1.5
	// [RoundTruncStr] result:       2, precision:  0, num:               2.5
	// [RoundTruncStr] result:       2, precision:  0, num:               2.5
	// [RoundTruncStr] result:      -2, precision:  0, num:              -2.5
	// [RoundTruncStr] result:      -2, precision:  0, num:              -2.5
	// [RoundTruncStr] result:     2.5, precision:  1, num:               2.5
	// [RoundTruncStr] result:       0, precision: -1, num:               2.5
	// [RoundTruncStr] result:       0, precision: -1, num:              -2.5
	// [RoundTruncStr] result:    -2.5, precision:  1, num:              -2.5
	// [RoundTruncStr] result:    2.50, precision:  2, num:               2.5
	// [RoundTruncStr] result:       0, precision: -2, num:               2.5
	// [RoundTruncStr] result:       0, precision: -2, num:              -2.5
	// [RoundTruncStr] result:   -2.50, precision:  2, num:              -2.5
	// [RoundTruncStr] result:      66, precision:  0, num:                66
	// [RoundTruncStr] result:      66, precision:  0, num:                66
	// [RoundTruncStr] result:     -66, precision:  0, num:               -66
	// [RoundTruncStr] result:     -66, precision:  0, num:               -66
	// [RoundTruncStr] result:    66.0, precision:  1, num:                66
	// [RoundTruncStr] result:      60, precision: -1, num:                66
	// [RoundTruncStr] result:     -60, precision: -1, num:               -66
	// [RoundTruncStr] result:   -66.0, precision:  1, num:               -66
	// [RoundTruncStr] result:   66.00, precision:  2, num:                66
	// [RoundTruncStr] result:       0, precision: -2, num:                66
	// [RoundTruncStr] result:       0, precision: -2, num:               -66
	// [RoundTruncStr] result:  -66.00, precision:  2, num:               -66
}

func ExampleRestoreToRealNumberStr() {
	number.SetRegForNumber(true)
	fmt.Println(number.RestoreToRealNumberStr(math.NaN()))
	fmt.Println(number.RestoreToRealNumberStr(math.Inf(-1)))
	fmt.Println(number.RestoreToRealNumberStr(math.Inf(+1)))
	fmt.Println(number.RestoreToRealNumberStr(int64(1234567890123456789)))

	number.SetRegForNumber(false)
	fmt.Println(number.RestoreToRealNumberStr(math.NaN()))
	fmt.Println(number.RestoreToRealNumberStr(math.Inf(-1)))
	fmt.Println(number.RestoreToRealNumberStr(math.Inf(+1)))
	fmt.Println(number.RestoreToRealNumberStr(-0e0))
	fmt.Println(number.RestoreToRealNumberStr(-0e1))
	fmt.Println(number.RestoreToRealNumberStr(float32(0.0001)))
	fmt.Println(number.RestoreToRealNumberStr(float32(0.0001e-3)))
	fmt.Println(number.RestoreToRealNumberStr(float32(0.00000001234567890123456789)))
	fmt.Println(number.RestoreToRealNumberStr(4321.65655500e5))
	fmt.Println(number.RestoreToRealNumberStr(4321.65655500e-2))
	fmt.Println(number.RestoreToRealNumberStr(4321.123e-2))
	fmt.Println(number.RestoreToRealNumberStr(1234567890123456789.123456789))

	// Output:
	// NaN
	// -Inf
	// +Inf
	// 1234567890123456789
	// NaN
	// -Inf
	// +Inf
	// 0
	// 0
	// 0.0001
	// 0.0000001
	// 0.000000012345679
	// 432165655.5
	// 43.21656555
	// 43.21123
	// 1234567890123456800
}

func TestRoundTrunc(t *testing.T) {
	precisions := []int{-11, 0, 11}
	for i := 0; i < len(precisions); i++ {
		if i&1 == 1 {
			number.SetRegForNumber(true)
		} else {
			number.SetRegForNumber(false)
		}

		fmt.Println(number.RoundTrunc(math.NaN(), precisions[i]))
		fmt.Println(number.RoundTrunc(math.Inf(-1), precisions[i]))
		fmt.Println(number.RoundTrunc(math.Inf(+1), precisions[i]))
		fmt.Println(number.RoundTrunc(-0e0, precisions[i]))
		fmt.Println(number.RoundTrunc(-0e1, precisions[i]))
		fmt.Println(number.RoundTrunc(float32(0.0001), precisions[i]))
		fmt.Println(number.RoundTrunc(float32(0.0001e-3), precisions[i]))
		fmt.Println(number.RoundTrunc(float32(0.00000001234567890123456789), precisions[i]))
		fmt.Println(number.RoundTrunc(4321.65655500e5, precisions[i]))
		fmt.Println(number.RoundTrunc(4321.65655500e-2, precisions[i]))
		fmt.Println(number.RoundTrunc(4321.123e-2, precisions[i]))
		fmt.Println(number.RoundTrunc(1234567890123456789.123456789, precisions[i]))
	}
}
