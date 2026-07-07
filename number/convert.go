// Package number provides generic helpers for converting between numeric types
// and byte slices (big- and little-endian), and for rounding numeric values to
// a given precision with several rounding modes (half-away-from-zero,
// ties-to-even, floor, ceil, and truncation).
//
// The conversion helpers operate on the [BinaryType] constraint, which covers
// the fixed-width integer, unsigned integer, float and bool types together with
// their slices and pointers. The rounding helpers operate on the [Int], [Uint]
// and [Float] constraints and return the same type as the input (except for the
// float rounders, which always return float64).
package number

import (
	"bytes"
	"encoding/binary"
)

// BinaryType is the data type accepted by the binary conversion helpers. It
// constrains the argument to the fixed-width integer, unsigned integer, float
// and bool types together with their slices and pointers, all of which can be
// (de)serialized by encoding/binary.
type BinaryType interface {
	~*bool | ~bool | ~[]bool |
		~*int8 | ~int8 | ~[]int8 |
		~*uint8 | ~uint8 | ~[]uint8 |
		~*int16 | ~int16 | ~[]int16 |
		~*uint16 | ~uint16 | ~[]uint16 |
		~*int32 | ~int32 | ~[]int32 |
		~*uint32 | ~uint32 | ~[]uint32 |
		~*int64 | ~int64 | ~[]int64 |
		~*uint64 | ~uint64 | ~[]uint64 |
		~*float32 | ~float32 | ~[]float32 |
		~*float64 | ~float64 | ~[]float64
}

// ToBytes converts data of a [BinaryType] to its big-endian byte
// representation. The returned error, if any, comes from encoding/binary.Write.
//
// The BinaryType as followings:
//
//	~*bool | ~bool | ~[]bool |
//	~*int8 | ~int8 | ~[]int8 |
//	~*uint8 | ~uint8 | ~[]uint8 |
//	~*int16 | ~int16 | ~[]int16 |
//	~*uint16 | ~uint16 | ~[]uint16 |
//	~*int32 | ~int32 | ~[]int32 |
//	~*uint32 | ~uint32 | ~[]uint32 |
//	~*int64 | ~int64 | ~[]int64 |
//	~*uint64 | ~uint64 | ~[]uint64 |
//	~*float32 | ~float32 | ~[]float32 |
//	~*float64 | ~float64 | ~[]float64
func ToBytes[T BinaryType](data T) ([]byte, error) {
	buf := bytes.NewBuffer([]byte{})
	err := binary.Write(buf, binary.BigEndian, data)
	return buf.Bytes(), err
}

// ToBytesLittleEndian converts data of a [BinaryType] to its little-endian
// byte representation. It is the little-endian counterpart of [ToBytes].
//
//	The BinaryType as followings:
//	~*bool | ~bool | ~[]bool |
//	~*int8 | ~int8 | ~[]int8 |
//	~*uint8 | ~uint8 | ~[]uint8 |
//	~*int16 | ~int16 | ~[]int16 |
//	~*uint16 | ~uint16 | ~[]uint16 |
//	~*int32 | ~int32 | ~[]int32 |
//	~*uint32 | ~uint32 | ~[]uint32 |
//	~*int64 | ~int64 | ~[]int64 |
//	~*uint64 | ~uint64 | ~[]uint64 |
//	~*float32 | ~float32 | ~[]float32 |
//	~*float64 | ~float64 | ~[]float64
func ToBytesLittleEndian[T BinaryType](data T) ([]byte, error) {
	buf := bytes.NewBuffer([]byte{})
	err := binary.Write(buf, binary.LittleEndian, data)
	return buf.Bytes(), err
}

// BytesToData converts the big-endian bytes in data into a value of the same
// type as kindAnyData. kindAnyData both selects the target type and receives the
// decoded result. It is the inverse of [ToBytes].
//
// The limits:
//  1. data []byte represent the slice, kindAnyData`s length and type must be same with the real.
//  2. data []byte represent the raw type or pointer, kindAnyData shall set zero value like *new(type).
func BytesToData[T BinaryType](data []byte, kindAnyData T) (T, error) {
	buf := bytes.NewBuffer(data)
	err := binary.Read(buf, binary.BigEndian, &kindAnyData)
	return kindAnyData, err
}

// BytesToDataLittleEndian converts the little-endian bytes in data into a value
// of the same type as kindAnyData. It is the little-endian counterpart of
// [BytesToData] and the inverse of [ToBytesLittleEndian].
//
// The limits:
//  1. data []byte represent the slice, kindAnyData`s length and type must be same with the real.
//  2. data []byte represent the raw type or pointer, kindAnyData shall set zero value like *new(type).
func BytesToDataLittleEndian[T BinaryType](data []byte, kindAnyData T) (T, error) {
	buf := bytes.NewBuffer(data)
	err := binary.Read(buf, binary.LittleEndian, &kindAnyData)
	return kindAnyData, err
}

// BytesToUint converts the big-endian byte slice b to an unsigned integer by
// accumulating each byte (most-significant first). The length of b is limited
// by the width of uint on the host platform.
func BytesToUint(b []byte) (result uint) {
	n := len(b)
	for i := range n {
		result = result << 8
		result += uint(b[i])
	}
	return result
}
