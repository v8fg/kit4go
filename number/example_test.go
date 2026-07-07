package number_test

import (
	"fmt"

	"github.com/v8fg/kit4go/number"
)

// ExampleToBytes serializes a fixed-width int32 to big-endian bytes and
// parses it back with BytesToData, showing the canonical round-trip for
// the binary conversion API. Endianness matters: big-endian yields the
// most-significant byte first.
func ExampleToBytes() {
	bs, err := number.ToBytes(int32(1))
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(bs)

	got, err := number.BytesToData(bs, int32(0))
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(got)
	// Output:
	// [0 0 0 1]
	// 1
}

// ExampleToBytesLittleEndian shows the little-endian counterpart: the
// least-significant byte comes first, so int32(1) serializes to a
// leading 0x01 followed by zero bytes. BytesToDataLittleEndian reads it
// back when given a matching destination type.
func ExampleToBytesLittleEndian() {
	bs, err := number.ToBytesLittleEndian(int32(1))
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(bs)

	got, err := number.BytesToDataLittleEndian(bs, int32(0))
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(got)
	// Output:
	// [1 0 0 0]
	// 1
}
