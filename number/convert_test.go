package number_test

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"

	"github.com/v8fg/kit4go/number"
)

func TestToBytes(t *testing.T) {
	convey.Convey("TestToBytes", t, func() {
		var bytes []byte

		bytes, _ = number.ToBytes(new(bool))
		convey.So(bytes, convey.ShouldResemble, []byte{0})
		bytes, _ = number.ToBytes(true)
		convey.So(bytes, convey.ShouldResemble, []byte{1})
		bytes, _ = number.ToBytes(false)
		convey.So(bytes, convey.ShouldResemble, []byte{0})
		bytes, _ = number.ToBytes([]bool{true, false})
		convey.So(bytes, convey.ShouldResemble, []byte{1, 0})

		bytes, _ = number.ToBytes(new(int8))
		convey.So(bytes, convey.ShouldResemble, []byte{0})
		bytes, _ = number.ToBytes(int8(0))
		convey.So(bytes, convey.ShouldResemble, []byte{0})
		bytes, _ = number.ToBytes([]int8{0, 1})
		convey.So(bytes, convey.ShouldResemble, []byte{0, 1})

		bytes, _ = number.ToBytes(new(uint8))
		convey.So(bytes, convey.ShouldResemble, []byte{0})
		bytes, _ = number.ToBytes(uint8(0))
		convey.So(bytes, convey.ShouldResemble, []byte{0})
		bytes, _ = number.ToBytes([]uint8{0, 1})
		convey.So(bytes, convey.ShouldResemble, []byte{0, 1})

		bytes, _ = number.ToBytes(new(int16))
		convey.So(bytes, convey.ShouldResemble, []byte{0, 0})
		bytes, _ = number.ToBytes(int16(0))
		convey.So(bytes, convey.ShouldResemble, []byte{0, 0})
		bytes, _ = number.ToBytes([]int16{0, 1})
		convey.So(bytes, convey.ShouldResemble, []byte{0, 0, 0, 1})

		bytes, _ = number.ToBytes(new(uint16))
		convey.So(bytes, convey.ShouldResemble, []byte{0, 0})
		bytes, _ = number.ToBytes(uint16(0))
		convey.So(bytes, convey.ShouldResemble, []byte{0, 0})
		bytes, _ = number.ToBytes([]uint16{0, 1})
		convey.So(bytes, convey.ShouldResemble, []byte{0, 0, 0, 1})

		bytes, _ = number.ToBytes(new(int32))
		convey.So(bytes, convey.ShouldResemble, []byte{0, 0, 0, 0})
		bytes, _ = number.ToBytes(int32(0))
		convey.So(bytes, convey.ShouldResemble, []byte{0, 0, 0, 0})
		bytes, _ = number.ToBytes([]int32{0, 1})
		convey.So(bytes, convey.ShouldResemble, []byte{0, 0, 0, 0, 0, 0, 0, 1})

		bytes, _ = number.ToBytes(new(uint32))
		convey.So(bytes, convey.ShouldResemble, []byte{0, 0, 0, 0})
		bytes, _ = number.ToBytes(uint32(0))
		convey.So(bytes, convey.ShouldResemble, []byte{0, 0, 0, 0})
		bytes, _ = number.ToBytes([]uint32{0, 1})
		convey.So(bytes, convey.ShouldResemble, []byte{0, 0, 0, 0, 0, 0, 0, 1})

		bytes, _ = number.ToBytes(new(int64))
		convey.So(bytes, convey.ShouldResemble, []byte{0, 0, 0, 0, 0, 0, 0, 0})
		bytes, _ = number.ToBytes(int64(0))
		convey.So(bytes, convey.ShouldResemble, []byte{0, 0, 0, 0, 0, 0, 0, 0})
		bytes, _ = number.ToBytes([]int64{0, 1})
		convey.So(bytes, convey.ShouldResemble, []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1})

		bytes, _ = number.ToBytes(new(uint64))
		convey.So(bytes, convey.ShouldResemble, []byte{0, 0, 0, 0, 0, 0, 0, 0})
		bytes, _ = number.ToBytes(uint64(0))
		convey.So(bytes, convey.ShouldResemble, []byte{0, 0, 0, 0, 0, 0, 0, 0})
		bytes, _ = number.ToBytes([]uint64{0, 1})
		convey.So(bytes, convey.ShouldResemble, []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1})

		bytes, _ = number.ToBytes(new(float32))
		convey.So(bytes, convey.ShouldResemble, []byte{0, 0, 0, 0})
		bytes, _ = number.ToBytes(float32(0))
		convey.So(bytes, convey.ShouldResemble, []byte{0, 0, 0, 0})
		bytes, _ = number.ToBytes([]float32{0, 1})
		convey.So(bytes, convey.ShouldResemble, []byte{0, 0, 0, 0, 63, 128, 0, 0})

		bytes, _ = number.ToBytes(new(float64))
		convey.So(bytes, convey.ShouldResemble, []byte{0, 0, 0, 0, 0, 0, 0, 0})
		bytes, _ = number.ToBytes(float64(0))
		convey.So(bytes, convey.ShouldResemble, []byte{0, 0, 0, 0, 0, 0, 0, 0})
		bytes, _ = number.ToBytes([]float64{0, 1})
		convey.So(bytes, convey.ShouldResemble, []byte{0, 0, 0, 0, 0, 0, 0, 0, 63, 240, 0, 0, 0, 0, 0, 0})
	})
}

func TestToBytesLittleEndian(t *testing.T) {
	convey.Convey("TestToBytesLittleEndian", t, func() {
		var bytes []byte

		bytes, _ = number.ToBytesLittleEndian(new(bool))
		convey.So(bytes, convey.ShouldResemble, []byte{0})
		bytes, _ = number.ToBytesLittleEndian(true)
		convey.So(bytes, convey.ShouldResemble, []byte{1})
		bytes, _ = number.ToBytesLittleEndian(false)
		convey.So(bytes, convey.ShouldResemble, []byte{0})
		bytes, _ = number.ToBytesLittleEndian([]bool{true, false})
		convey.So(bytes, convey.ShouldResemble, []byte{1, 0})

		bytes, _ = number.ToBytesLittleEndian(new(int8))
		convey.So(bytes, convey.ShouldResemble, []byte{0})
		bytes, _ = number.ToBytesLittleEndian(int8(0))
		convey.So(bytes, convey.ShouldResemble, []byte{0})
		bytes, _ = number.ToBytesLittleEndian([]int8{0, 1})
		convey.So(bytes, convey.ShouldResemble, []byte{0, 1})

		bytes, _ = number.ToBytesLittleEndian(new(uint8))
		convey.So(bytes, convey.ShouldResemble, []byte{0})
		bytes, _ = number.ToBytesLittleEndian(uint8(0))
		convey.So(bytes, convey.ShouldResemble, []byte{0})
		bytes, _ = number.ToBytesLittleEndian([]uint8{0, 1})
		convey.So(bytes, convey.ShouldResemble, []byte{0, 1})

		bytes, _ = number.ToBytesLittleEndian(new(int16))
		convey.So(bytes, convey.ShouldResemble, []byte{0, 0})
		bytes, _ = number.ToBytesLittleEndian(int16(0))
		convey.So(bytes, convey.ShouldResemble, []byte{0, 0})
		bytes, _ = number.ToBytesLittleEndian([]int16{0, 1})
		convey.So(bytes, convey.ShouldResemble, []byte{0, 0, 1, 0})

		bytes, _ = number.ToBytesLittleEndian(new(uint16))
		convey.So(bytes, convey.ShouldResemble, []byte{0, 0})
		bytes, _ = number.ToBytesLittleEndian(uint16(0))
		convey.So(bytes, convey.ShouldResemble, []byte{0, 0})
		bytes, _ = number.ToBytesLittleEndian([]uint16{0, 1})
		convey.So(bytes, convey.ShouldResemble, []byte{0, 0, 1, 0})

		bytes, _ = number.ToBytesLittleEndian(new(int32))
		convey.So(bytes, convey.ShouldResemble, []byte{0, 0, 0, 0})
		bytes, _ = number.ToBytesLittleEndian(int32(0))
		convey.So(bytes, convey.ShouldResemble, []byte{0, 0, 0, 0})
		bytes, _ = number.ToBytesLittleEndian([]int32{0, 1})
		convey.So(bytes, convey.ShouldResemble, []byte{0, 0, 0, 0, 1, 0, 0, 0})

		bytes, _ = number.ToBytesLittleEndian(new(uint32))
		convey.So(bytes, convey.ShouldResemble, []byte{0, 0, 0, 0})
		bytes, _ = number.ToBytesLittleEndian(uint32(0))
		convey.So(bytes, convey.ShouldResemble, []byte{0, 0, 0, 0})
		bytes, _ = number.ToBytesLittleEndian([]uint32{0, 1})
		convey.So(bytes, convey.ShouldResemble, []byte{0, 0, 0, 0, 1, 0, 0, 0})

		bytes, _ = number.ToBytesLittleEndian(new(int64))
		convey.So(bytes, convey.ShouldResemble, []byte{0, 0, 0, 0, 0, 0, 0, 0})
		bytes, _ = number.ToBytesLittleEndian(int64(0))
		convey.So(bytes, convey.ShouldResemble, []byte{0, 0, 0, 0, 0, 0, 0, 0})
		bytes, _ = number.ToBytesLittleEndian([]int64{0, 1})
		convey.So(bytes, convey.ShouldResemble, []byte{0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0})

		bytes, _ = number.ToBytesLittleEndian(new(uint64))
		convey.So(bytes, convey.ShouldResemble, []byte{0, 0, 0, 0, 0, 0, 0, 0})
		bytes, _ = number.ToBytesLittleEndian(uint64(0))
		convey.So(bytes, convey.ShouldResemble, []byte{0, 0, 0, 0, 0, 0, 0, 0})
		bytes, _ = number.ToBytesLittleEndian([]uint64{0, 1})
		convey.So(bytes, convey.ShouldResemble, []byte{0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0})

		bytes, _ = number.ToBytesLittleEndian(new(float32))
		convey.So(bytes, convey.ShouldResemble, []byte{0, 0, 0, 0})
		bytes, _ = number.ToBytesLittleEndian(float32(0))
		convey.So(bytes, convey.ShouldResemble, []byte{0, 0, 0, 0})
		bytes, _ = number.ToBytesLittleEndian([]float32{0, 1})
		convey.So(bytes, convey.ShouldResemble, []byte{0, 0, 0, 0, 0, 0, 128, 63})

		bytes, _ = number.ToBytesLittleEndian(new(float64))
		convey.So(bytes, convey.ShouldResemble, []byte{0, 0, 0, 0, 0, 0, 0, 0})
		bytes, _ = number.ToBytesLittleEndian(float64(0))
		convey.So(bytes, convey.ShouldResemble, []byte{0, 0, 0, 0, 0, 0, 0, 0})
		bytes, _ = number.ToBytesLittleEndian([]float64{0, 1})
		convey.So(bytes, convey.ShouldResemble, []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 240, 63})
	})
}

func TestBytesToData(t *testing.T) {
	convey.Convey("TestBytesToData", t, func() {
		var data any
		data, _ = number.BytesToData([]byte{0}, *new(bool))
		convey.So(data, convey.ShouldResemble, false)
		data, _ = number.BytesToData([]byte{1}, *new(bool))
		convey.So(data, convey.ShouldResemble, true)
		data, _ = number.BytesToData([]byte{0}, false)
		convey.So(data, convey.ShouldEqual, false)
		data, _ = number.BytesToData([]byte{0}, true)
		convey.So(data, convey.ShouldEqual, false)
		data, _ = number.BytesToData([]byte{1}, false)
		convey.So(data, convey.ShouldEqual, true)
		data, _ = number.BytesToData([]byte{1}, true)
		convey.So(data, convey.ShouldEqual, true)
		data, _ = number.BytesToData([]byte{1, 0, 1}, []bool{false, false, false})
		convey.So(data, convey.ShouldResemble, []bool{true, false, true})

		data, _ = number.BytesToData([]byte{0}, *new(int8))
		convey.So(data, convey.ShouldEqual, 0)
		data, _ = number.BytesToData([]byte{0}, int8(0))
		convey.So(data, convey.ShouldEqual, 0)
		data, _ = number.BytesToData([]byte{0}, []int8{0})
		convey.So(data, convey.ShouldResemble, []int8{0})

		data, _ = number.BytesToData([]byte{0}, *new(uint8))
		convey.So(data, convey.ShouldEqual, 0)
		data, _ = number.BytesToData([]byte{0}, uint8(0))
		convey.So(data, convey.ShouldEqual, 0)
		data, _ = number.BytesToData([]byte{0}, []uint8{0})
		convey.So(data, convey.ShouldResemble, []uint8{0})

		data, _ = number.BytesToData([]byte{0, 1}, *new(int16))
		convey.So(data, convey.ShouldEqual, 1)
		data, _ = number.BytesToData([]byte{0, 1}, int16(0))
		convey.So(data, convey.ShouldEqual, 1)
		data, _ = number.BytesToData([]byte{0, 1}, []int16{0})
		convey.So(data, convey.ShouldResemble, []int16{1})

		data, _ = number.BytesToData([]byte{0, 1}, *new(uint16))
		convey.So(data, convey.ShouldEqual, 1)
		data, _ = number.BytesToData([]byte{0, 1}, uint16(0))
		convey.So(data, convey.ShouldEqual, 1)
		data, _ = number.BytesToData([]byte{0, 1}, []uint16{0})
		convey.So(data, convey.ShouldResemble, []uint16{1})

		data, _ = number.BytesToData([]byte{0, 0, 0, 1}, *new(int32))
		convey.So(data, convey.ShouldEqual, 1)
		data, _ = number.BytesToData([]byte{0, 0, 0, 1}, int32(0))
		convey.So(data, convey.ShouldEqual, 1)
		data, _ = number.BytesToData([]byte{0, 0, 0, 1}, []int32{0})
		convey.So(data, convey.ShouldResemble, []int32{1})

		data, _ = number.BytesToData([]byte{0, 0, 0, 1}, *new(uint32))
		convey.So(data, convey.ShouldEqual, 1)
		data, _ = number.BytesToData([]byte{0, 0, 0, 1}, uint32(0))
		convey.So(data, convey.ShouldEqual, 1)
		data, _ = number.BytesToData([]byte{0, 0, 0, 1}, []uint32{0})
		convey.So(data, convey.ShouldResemble, []uint32{1})

		data, _ = number.BytesToData([]byte{0, 0, 0, 0, 0, 0, 0, 1}, *new(int64))
		convey.So(data, convey.ShouldEqual, 1)
		data, _ = number.BytesToData([]byte{0, 0, 0, 0, 0, 0, 0, 1}, int64(0))
		convey.So(data, convey.ShouldEqual, 1)
		data, _ = number.BytesToData([]byte{0, 0, 0, 0, 0, 0, 0, 1}, []int64{0})
		convey.So(data, convey.ShouldResemble, []int64{1})

		data, _ = number.BytesToData([]byte{0, 0, 0, 0, 0, 0, 0, 1}, *new(uint64))
		convey.So(data, convey.ShouldEqual, 1)
		data, _ = number.BytesToData([]byte{0, 0, 0, 0, 0, 0, 0, 1}, uint64(0))
		convey.So(data, convey.ShouldEqual, 1)
		data, _ = number.BytesToData([]byte{0, 0, 0, 0, 0, 0, 0, 1}, []uint64{0})
		convey.So(data, convey.ShouldResemble, []uint64{1})

		data, _ = number.BytesToData([]byte{0, 0, 0, 0}, *new(float32))
		convey.So(data, convey.ShouldEqual, 0)
		data, _ = number.BytesToData([]byte{0, 0, 0, 0}, float32(0))
		convey.So(data, convey.ShouldEqual, 0)
		data, _ = number.BytesToData([]byte{0, 0, 0, 0}, []float32{0})
		convey.So(data, convey.ShouldResemble, []float32{0})

		data, _ = number.BytesToData([]byte{63, 128, 0, 0}, *new(float32))
		convey.So(data, convey.ShouldEqual, 1)
		data, _ = number.BytesToData([]byte{63, 128, 0, 0}, float32(0))
		convey.So(data, convey.ShouldEqual, 1)
		data, _ = number.BytesToData([]byte{63, 128, 0, 0}, []float32{0})
		convey.So(data, convey.ShouldResemble, []float32{1})
		data, _ = number.BytesToData([]byte{0, 0, 0, 0, 63, 128, 0, 0}, []float32{0, 0})
		convey.So(data, convey.ShouldResemble, []float32{0, 1})

		data, _ = number.BytesToData([]byte{0, 0, 0, 0, 0, 0, 0, 0}, *new(float64))
		convey.So(data, convey.ShouldEqual, 0)
		data, _ = number.BytesToData([]byte{0, 0, 0, 0, 0, 0, 0, 0}, float64(0))
		convey.So(data, convey.ShouldEqual, 0)
		data, _ = number.BytesToData([]byte{0, 0, 0, 0, 0, 0, 0, 0}, []float64{0})
		convey.So(data, convey.ShouldResemble, []float64{0})

		data, _ = number.BytesToData([]byte{63, 240, 0, 0, 0, 0, 0, 0}, *new(float64))
		convey.So(data, convey.ShouldEqual, 1)
		data, _ = number.BytesToData([]byte{63, 240, 0, 0, 0, 0, 0, 0}, float64(0))
		convey.So(data, convey.ShouldEqual, 1)
		data, _ = number.BytesToData([]byte{63, 240, 0, 0, 0, 0, 0, 0}, []float64{0})
		convey.So(data, convey.ShouldResemble, []float64{1})
		data, _ = number.BytesToData([]byte{0, 0, 0, 0, 0, 0, 0, 0, 63, 240, 0, 0, 0, 0, 0, 0}, []float64{0, 0})
		convey.So(data, convey.ShouldResemble, []float64{0, 1})
	})
}

func TestBytesToDataLittleEndian(t *testing.T) {
	convey.Convey("TestBytesToDataLittleEndian", t, func() {
		var data any
		data, _ = number.BytesToDataLittleEndian([]byte{0}, *new(bool))
		convey.So(data, convey.ShouldResemble, false)
		data, _ = number.BytesToDataLittleEndian([]byte{1}, *new(bool))
		convey.So(data, convey.ShouldResemble, true)
		data, _ = number.BytesToDataLittleEndian([]byte{0}, false)
		convey.So(data, convey.ShouldEqual, false)
		data, _ = number.BytesToDataLittleEndian([]byte{0}, true)
		convey.So(data, convey.ShouldEqual, false)
		data, _ = number.BytesToDataLittleEndian([]byte{1}, false)
		convey.So(data, convey.ShouldEqual, true)
		data, _ = number.BytesToDataLittleEndian([]byte{1}, true)
		convey.So(data, convey.ShouldEqual, true)
		data, _ = number.BytesToDataLittleEndian([]byte{1, 0, 1}, []bool{false, false, false})
		convey.So(data, convey.ShouldResemble, []bool{true, false, true})

		data, _ = number.BytesToDataLittleEndian([]byte{0}, *new(int8))
		convey.So(data, convey.ShouldEqual, 0)
		data, _ = number.BytesToDataLittleEndian([]byte{0}, int8(0))
		convey.So(data, convey.ShouldEqual, 0)
		data, _ = number.BytesToDataLittleEndian([]byte{0}, []int8{0})
		convey.So(data, convey.ShouldResemble, []int8{0})

		data, _ = number.BytesToDataLittleEndian([]byte{0}, *new(uint8))
		convey.So(data, convey.ShouldEqual, 0)
		data, _ = number.BytesToDataLittleEndian([]byte{0}, uint8(0))
		convey.So(data, convey.ShouldEqual, 0)
		data, _ = number.BytesToDataLittleEndian([]byte{0}, []uint8{0})
		convey.So(data, convey.ShouldResemble, []uint8{0})

		data, _ = number.BytesToDataLittleEndian([]byte{1, 0}, *new(int16))
		convey.So(data, convey.ShouldEqual, 1)
		data, _ = number.BytesToDataLittleEndian([]byte{1, 0}, int16(0))
		convey.So(data, convey.ShouldEqual, 1)
		data, _ = number.BytesToDataLittleEndian([]byte{1, 0}, []int16{0})
		convey.So(data, convey.ShouldResemble, []int16{1})

		data, _ = number.BytesToDataLittleEndian([]byte{1, 0}, *new(uint16))
		convey.So(data, convey.ShouldEqual, 1)
		data, _ = number.BytesToDataLittleEndian([]byte{1, 0}, uint16(0))
		convey.So(data, convey.ShouldEqual, 1)
		data, _ = number.BytesToDataLittleEndian([]byte{1, 0}, []uint16{0})
		convey.So(data, convey.ShouldResemble, []uint16{1})

		data, _ = number.BytesToDataLittleEndian([]byte{1, 0, 0, 0}, *new(int32))
		convey.So(data, convey.ShouldEqual, 1)
		data, _ = number.BytesToDataLittleEndian([]byte{1, 0, 0, 0}, int32(0))
		convey.So(data, convey.ShouldEqual, 1)
		data, _ = number.BytesToDataLittleEndian([]byte{1, 0, 0, 0}, []int32{0})
		convey.So(data, convey.ShouldResemble, []int32{1})

		data, _ = number.BytesToDataLittleEndian([]byte{1, 0, 0, 0}, *new(uint32))
		convey.So(data, convey.ShouldEqual, 1)
		data, _ = number.BytesToDataLittleEndian([]byte{1, 0, 0, 0}, uint32(0))
		convey.So(data, convey.ShouldEqual, 1)
		data, _ = number.BytesToDataLittleEndian([]byte{1, 0, 0, 0}, []uint32{0})
		convey.So(data, convey.ShouldResemble, []uint32{1})

		data, _ = number.BytesToDataLittleEndian([]byte{1, 0, 0, 0, 0, 0, 0, 0}, *new(int64))
		convey.So(data, convey.ShouldEqual, 1)
		data, _ = number.BytesToDataLittleEndian([]byte{1, 0, 0, 0, 0, 0, 0, 0}, int64(0))
		convey.So(data, convey.ShouldEqual, 1)
		data, _ = number.BytesToDataLittleEndian([]byte{1, 0, 0, 0, 0, 0, 0, 0}, []int64{0})
		convey.So(data, convey.ShouldResemble, []int64{1})

		data, _ = number.BytesToDataLittleEndian([]byte{1, 0, 0, 0, 0, 0, 0, 0}, *new(uint64))
		convey.So(data, convey.ShouldEqual, 1)
		data, _ = number.BytesToDataLittleEndian([]byte{1, 0, 0, 0, 0, 0, 0, 0}, uint64(0))
		convey.So(data, convey.ShouldEqual, 1)
		data, _ = number.BytesToDataLittleEndian([]byte{1, 0, 0, 0, 0, 0, 0, 0}, []uint64{0})
		convey.So(data, convey.ShouldResemble, []uint64{1})

		data, _ = number.BytesToDataLittleEndian([]byte{0, 0, 0, 0}, *new(float32))
		convey.So(data, convey.ShouldEqual, 0)
		data, _ = number.BytesToDataLittleEndian([]byte{0, 0, 0, 0}, float32(0))
		convey.So(data, convey.ShouldEqual, 0)
		data, _ = number.BytesToDataLittleEndian([]byte{0, 0, 0, 0}, []float32{0})
		convey.So(data, convey.ShouldResemble, []float32{0})

		data, _ = number.BytesToDataLittleEndian([]byte{0, 0, 128, 63}, *new(float32))
		convey.So(data, convey.ShouldEqual, 1)
		data, _ = number.BytesToDataLittleEndian([]byte{0, 0, 128, 63}, float32(0))
		convey.So(data, convey.ShouldEqual, 1)
		data, _ = number.BytesToDataLittleEndian([]byte{0, 0, 128, 63}, []float32{0})
		convey.So(data, convey.ShouldResemble, []float32{1})
		data, _ = number.BytesToDataLittleEndian([]byte{0, 0, 0, 0, 0, 0, 128, 63}, []float32{0, 0})
		convey.So(data, convey.ShouldResemble, []float32{0, 1})

		data, _ = number.BytesToDataLittleEndian([]byte{0, 0, 0, 0, 0, 0, 0, 0}, *new(float64))
		convey.So(data, convey.ShouldEqual, 0)
		data, _ = number.BytesToDataLittleEndian([]byte{0, 0, 0, 0, 0, 0, 0, 0}, float64(0))
		convey.So(data, convey.ShouldEqual, 0)
		data, _ = number.BytesToDataLittleEndian([]byte{0, 0, 0, 0, 0, 0, 0, 0}, []float64{0})
		convey.So(data, convey.ShouldResemble, []float64{0})

		data, _ = number.BytesToDataLittleEndian([]byte{0, 0, 0, 0, 0, 0, 240, 63}, *new(float64))
		convey.So(data, convey.ShouldEqual, 1)
		data, _ = number.BytesToDataLittleEndian([]byte{0, 0, 0, 0, 0, 0, 240, 63}, float64(0))
		convey.So(data, convey.ShouldEqual, 1)
		data, _ = number.BytesToDataLittleEndian([]byte{0, 0, 0, 0, 0, 0, 240, 63}, []float64{0})
		convey.So(data, convey.ShouldResemble, []float64{1})
		data, _ = number.BytesToDataLittleEndian([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 240, 63}, []float64{0, 0})
		convey.So(data, convey.ShouldResemble, []float64{0, 1})
	})
}
