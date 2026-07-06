package number_test

import (
	"math"
	"sync"
	"testing"

	"github.com/v8fg/kit4go/number"
)

// TestSetRegForNumberNoDataRace guards the atomic.Pointer[regexp.Regexp] fix in
// round.go. SetRegForNumber swaps the shared regex used by RestoreToRealNumberStr /
// regSplitNormalNumber; doing that concurrently with readers used to be a data race
// (-race flagged the unsynchronized write vs read). This test hammers both sides at
// once and must stay -race clean.
func TestSetRegForNumberNoDataRace(t *testing.T) {
	t.Parallel()

	// Inputs that exercise both the integer+fractional path and the exponential
	// path inside RestoreToRealNumberStr / RoundTrunc.
	inputsF64 := []float64{
		math.Pi,
		-math.Pi,
		0.0001e-3,
		0.0000000123,
		4321.65655500e5,
		4321.65655500e-2,
		1234567890123456789.123456789,
		-0,
	}
	inputsF32 := []float32{
		0.0001,
		0.0001e-3,
		float32(0.00000001234567890123456789),
		-3.14,
	}
	precisions := []int{-11, 0, 2, 6, 11}

	var wg sync.WaitGroup
	const swappers = 2
	const readers = 8

	// Swappers: flip between reg6 and reg7 for the whole test duration.
	wg.Add(swappers)
	for i := 0; i < swappers; i++ {
		go func(use7 bool) {
			defer wg.Done()
			for j := 0; j < 2000; j++ {
				number.SetRegForNumber(use7)
				use7 = !use7
			}
		}(i%2 == 0)
	}

	// Readers: hammer the read sites (Round -> RestoreToRealNumberStr and
	// regSplitNormalNumber) while the regex is being swapped underneath.
	wg.Add(readers)
	for r := 0; r < readers; r++ {
		go func() {
			defer wg.Done()
			for n := 0; n < 4000; n++ {
				for _, f := range inputsF64 {
					_ = number.RoundTrunc(f, precisions[n%len(precisions)])
					_ = number.RestoreToRealNumberStr(f)
				}
				for _, f := range inputsF32 {
					_ = number.RoundTrunc(f, precisions[n%len(precisions)])
					_ = number.RestoreToRealNumberStr(f)
				}
			}
		}()
	}

	wg.Wait()

	// Leave the package in its default (reg6) state so other tests are unaffected
	// regardless of goroutine interleaving.
	number.SetRegForNumber(false)
}
