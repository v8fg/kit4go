package random_test

import (
	"fmt"

	"github.com/v8fg/kit4go/random"
)

func ExampleNumericCode() {
	// A 6-digit SMS/email verification code (length is deterministic; the
	// digits themselves are random, so only the length is asserted).
	fmt.Println(len(random.NumericCode(6)))
	// Output: 6
}

func ExampleRandUniCodeByUID() {
	// RandUniCodeByUID derives a short, stable invitation/referral code from a
	// user id: the same (uid, n) always maps to the same string (DefaultSALT),
	// so it is safe to assert its exact value. A different uid yields a
	// different code; n is capped at 10.
	fmt.Println(random.RandUniCodeByUID(12345, 6))
	// Output: q6Mjsu
}

func ExampleRandStringWithKind() {
	// RandStringWithKind picks n characters from the charset selected by the
	// kind bitmask (0001=digits, 0010=upper, 0100=lower). The output bytes are
	// random, so only the length (and, here, that every byte is a decimal
	// digit) is asserted — no // Output line.
	const kindDigits = 1
	code := random.RandStringWithKind(8, kindDigits)
	fmt.Println(len(code))
	for _, b := range code {
		if b < '0' || b > '9' {
			fmt.Println("non-digit byte")
			return
		}
	}
	fmt.Println("all digits")
	// (no // Output: the byte values are random, only the class is fixed)
}
