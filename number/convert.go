package number

import (
	"bytes"
	"encoding/binary"
)

// BinaryType the data type for binary Write.
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

// ToBytes converts the data of the limited type BinaryType to byte with big endian.
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
func ToBytes[T BinaryType](data T) ([]byte, error) {
	buf := bytes.NewBuffer([]byte{})
	err := binary.Write(buf, binary.BigEndian, data)
	return buf.Bytes(), err
}

// ToBytesLittleEndian converts the data of the limited type to byte with little endian.
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

// BytesToData converts the bytes to the corresponding data same with the type of kindAnyData, with big endian.
//
// The limits:
//  1. data []byte represent the slice, kindAnyData`s length and type must be same with the real.
//  2. data []byte represent the raw type or pointer, kindAnyData shall set zero value like *new(type).
func BytesToData[T BinaryType](data []byte, kindAnyData T) (T, error) {
	buf := bytes.NewBuffer(data)
	err := binary.Read(buf, binary.BigEndian, &kindAnyData)
	return kindAnyData, err
}

// BytesToDataLittleEndian converts the bytes to the corresponding data same with the type of kindAnyData, with little endian.
//
// The limits:
//  1. data []byte represent the slice, kindAnyData`s length and type must be same with the real.
//  2. data []byte represent the raw type or pointer, kindAnyData shall set zero value like *new(type).
func BytesToDataLittleEndian[T BinaryType](data []byte, kindAnyData T) (T, error) {
	buf := bytes.NewBuffer(data)
	err := binary.Read(buf, binary.LittleEndian, &kindAnyData)
	return kindAnyData, err
}
