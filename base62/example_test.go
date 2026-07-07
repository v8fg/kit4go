package base62_test

import (
	"errors"
	"fmt"

	"github.com/v8fg/kit4go/base62"
)

func ExampleEncode() {
	// Encode maps an integer id to a compact, URL-safe slug. The alphabet is
	// 0-9, A-Z, a-z: index 0 -> '0', 61 -> 'z', 62 -> "10" (one column over).
	fmt.Println(base62.Encode(0))
	fmt.Println(base62.Encode(61))
	fmt.Println(base62.Encode(62))
	fmt.Println(base62.Encode(12345))
	// Output:
	// 0
	// z
	// 10
	// 3D7
}

func ExampleDecode() {
	// Decode reverses Encode. The error is non-nil for an empty string or any
	// byte outside the alphabet.
	for _, s := range []string{"0", "z", "10", "3D7"} {
		n, err := base62.Decode(s)
		fmt.Printf("%s -> %d %v\n", s, n, err)
	}
	_, err := base62.Decode("3d-7") // '-' is not in the alphabet
	fmt.Println(errors.Is(err, base62.ErrInvalid))
	// Output:
	// 0 -> 0 <nil>
	// z -> 61 <nil>
	// 10 -> 62 <nil>
	// 3D7 -> 12345 <nil>
	// true
}

func ExampleEncodeWithAlphabet() {
	// A custom alphabet maps the same integer to a different slug. Here the
	// alphabet is reversed, so 0 -> 'z', 61 -> '0', and 62 spills to two
	// columns: index[1]='y', index[0]='z' -> "yz".
	custom := "zyxwvutsrqponmlkjihgfedcbaZYXWVUTSRQPONMLKJIHGFEDCBA9876543210"
	fmt.Println(base62.EncodeWithAlphabet(0, custom))
	fmt.Println(base62.EncodeWithAlphabet(61, custom))
	fmt.Println(base62.EncodeWithAlphabet(62, custom))
	// Output:
	// z
	// 0
	// yz
}

func ExampleDecodeWithAlphabet() {
	// DecodeWithAlphabet reverses EncodeWithAlphabet using the same custom
	// alphabet, and rejects a malformed alphabet with ErrAlphabet.
	custom := "zyxwvutsrqponmlkjihgfedcbaZYXWVUTSRQPONMLKJIHGFEDCBA9876543210"
	n, err := base62.DecodeWithAlphabet("yz", custom)
	fmt.Printf("yz -> %d %v\n", n, err)

	// A 36-byte alphabet fails the length guard.
	_, err = base62.DecodeWithAlphabet("0", "0123456789abcdefghijklmnopqrstuvwxyz")
	fmt.Println(errors.Is(err, base62.ErrAlphabet))
	// Output:
	// yz -> 62 <nil>
	// true
}
