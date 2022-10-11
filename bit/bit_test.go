package bit_test

import (
	"math"
	"testing"

	"github.com/smartystreets/goconvey/convey"

	"github.com/v8fg/kit4go/bit"
)

func TestHasOppositeSigns(t *testing.T) {
	convey.Convey("TestHasOppositeSigns", t, func() {
		convey.So(bit.HasOppositeSigns(-1, 0), convey.ShouldBeTrue)
		convey.So(bit.HasOppositeSigns(1, 0), convey.ShouldBeFalse)
		convey.So(bit.HasOppositeSigns(-1, 1), convey.ShouldBeTrue)

		convey.So(bit.HasOppositeSigns(int8(1), int8(2)), convey.ShouldBeFalse)
		convey.So(bit.HasOppositeSigns(int8(-1), int8(1)), convey.ShouldBeTrue)
		convey.So(bit.HasOppositeSigns(uint8(1), uint8(2)), convey.ShouldBeFalse)

		convey.So(bit.HasOppositeSigns(int16(1), int16(2)), convey.ShouldBeFalse)
		convey.So(bit.HasOppositeSigns(int16(-1), int16(1)), convey.ShouldBeTrue)
		convey.So(bit.HasOppositeSigns(int32(1), int32(2)), convey.ShouldBeFalse)
		convey.So(bit.HasOppositeSigns(int32(-1), int32(1)), convey.ShouldBeTrue)
		convey.So(bit.HasOppositeSigns(int64(1), int64(2)), convey.ShouldBeFalse)
		convey.So(bit.HasOppositeSigns(int64(-1), int64(1)), convey.ShouldBeTrue)
	})
}

func TestMin(t *testing.T) {
	convey.Convey("TestMin", t, func() {
		convey.So(bit.Min(1, 3), convey.ShouldEqual, 1)
		convey.So(bit.Min(1, 15), convey.ShouldEqual, 1)
		convey.So(bit.Min(2, -15), convey.ShouldEqual, -15)
		convey.So(bit.Min(-15, 2), convey.ShouldEqual, -15)
		convey.So(bit.Min(2, -64), convey.ShouldEqual, -64)
		convey.So(bit.Min(-64, 2), convey.ShouldEqual, -64)
		convey.So(bit.Min(2, 64), convey.ShouldEqual, 2)
		convey.So(bit.Min(1024, 64), convey.ShouldEqual, 64)
		convey.So(bit.Min(math.MaxInt, 64), convey.ShouldEqual, 64)
		convey.So(bit.Min(64, math.MaxInt), convey.ShouldEqual, 64)
		convey.So(bit.Min(math.MaxInt8, 64), convey.ShouldEqual, 64)
		convey.So(bit.Min(64, math.MaxInt8), convey.ShouldEqual, 64)
		convey.So(bit.Min(math.MaxInt8, 128), convey.ShouldEqual, math.MaxInt8)
		convey.So(bit.Min(128, math.MaxInt8), convey.ShouldEqual, math.MaxInt8)
		convey.So(bit.Min(math.MaxInt32, 64), convey.ShouldEqual, 64)
		convey.So(bit.Min(64, math.MaxInt32), convey.ShouldEqual, 64)
		convey.So(bit.Min(math.MaxUint32, 64), convey.ShouldEqual, 64)
		convey.So(bit.Min(math.MaxInt64, 64), convey.ShouldEqual, 64)
		convey.So(bit.Min(64, math.MaxInt64), convey.ShouldEqual, 64)
		convey.So(bit.Min(math.MaxUint32, math.MaxInt64), convey.ShouldEqual, math.MaxUint32)
		convey.So(bit.Min(math.MaxInt64, math.MaxUint32), convey.ShouldEqual, math.MaxUint32)
	})
}

func TestMax(t *testing.T) {
	convey.Convey("TestMax", t, func() {
		convey.So(bit.Max(1, 3), convey.ShouldEqual, 3)
		convey.So(bit.Max(1, 15), convey.ShouldEqual, 15)
		convey.So(bit.Max(2, -15), convey.ShouldEqual, 2)
		convey.So(bit.Max(-15, 2), convey.ShouldEqual, 2)
		convey.So(bit.Max(2, -64), convey.ShouldEqual, 2)
		convey.So(bit.Max(-64, 2), convey.ShouldEqual, 2)
		convey.So(bit.Max(2, 64), convey.ShouldEqual, 64)
		convey.So(bit.Max(1024, 64), convey.ShouldEqual, 1024)
		convey.So(bit.Max(64, math.MaxInt8), convey.ShouldEqual, math.MaxInt8)
		convey.So(bit.Max(math.MaxInt8, 64), convey.ShouldEqual, math.MaxInt8)
		convey.So(bit.Max(64, math.MaxInt), convey.ShouldEqual, math.MaxInt)
		convey.So(bit.Max(math.MaxInt, 64), convey.ShouldEqual, math.MaxInt)
		convey.So(bit.Max(math.MaxInt32, 64), convey.ShouldEqual, math.MaxInt32)
		convey.So(bit.Max(64, math.MaxInt32), convey.ShouldEqual, math.MaxInt32)
		convey.So(bit.Max(math.MaxInt64, 64), convey.ShouldEqual, math.MaxInt64)
		convey.So(bit.Max(64, math.MaxInt64), convey.ShouldEqual, math.MaxInt64)
		convey.So(bit.Max(math.MaxUint32, math.MaxInt64), convey.ShouldEqual, math.MaxInt64)
		convey.So(bit.Max(math.MaxInt64, math.MaxUint32), convey.ShouldEqual, math.MaxInt64)
	})
}

func TestAbs(t *testing.T) {
	convey.Convey("TestAbs", t, func() {
		convey.So(bit.Abs(1), convey.ShouldEqual, 1)
		convey.So(bit.Abs(-1), convey.ShouldEqual, 1)
		convey.So(bit.Abs(16), convey.ShouldEqual, 16)
		convey.So(bit.Abs(-16), convey.ShouldEqual, 16)
		convey.So(bit.Abs(math.MaxInt), convey.ShouldEqual, math.MaxInt)
		convey.So(bit.Abs(-math.MaxInt), convey.ShouldEqual, math.MaxInt)
		convey.So(bit.Abs(math.MaxInt16), convey.ShouldEqual, math.MaxInt16)
		convey.So(bit.Abs(math.MinInt16), convey.ShouldEqual, math.MaxInt16+1)
		convey.So(bit.Abs(math.MaxInt64), convey.ShouldEqual, math.MaxInt64)
		convey.So(bit.Abs(math.MinInt), convey.ShouldEqual, math.MinInt)     // -9223372036854775808, overflow
		convey.So(bit.Abs(math.MinInt64), convey.ShouldEqual, math.MinInt64) // -9223372036854775808, overflow
	})
}

func TestCountOneBit(t *testing.T) {
	convey.Convey("TestCountOneBit", t, func() {
		convey.So(bit.CountOneBit(0), convey.ShouldEqual, 0)
		convey.So(bit.CountOneBit(1), convey.ShouldEqual, 1)
		convey.So(bit.CountOneBit(2), convey.ShouldEqual, 1)
		convey.So(bit.CountOneBit(3), convey.ShouldEqual, 2)
		convey.So(bit.CountOneBit(4), convey.ShouldEqual, 1)
		convey.So(bit.CountOneBit(5), convey.ShouldEqual, 2)
		convey.So(bit.CountOneBit(8), convey.ShouldEqual, 1)

		convey.So(bit.CountOneBit(-0), convey.ShouldEqual, 0)
		convey.So(bit.CountOneBit(-2), convey.ShouldEqual, 63)
		convey.So(bit.CountOneBit(int8(-2)), convey.ShouldEqual, 7)
		convey.So(bit.CountOneBit(int16(-2)), convey.ShouldEqual, 15)
		convey.So(bit.CountOneBit(int32(-2)), convey.ShouldEqual, 31)
		convey.So(bit.CountOneBit(int64(-2)), convey.ShouldEqual, 63)
	})
}

func TestIsPowerOfTwo(t *testing.T) {
	convey.Convey("TestIsPowerOfTwo", t, func() {
		convey.So(bit.IsPowerOfTwo(0), convey.ShouldBeFalse)
		convey.So(bit.IsPowerOfTwo(1), convey.ShouldBeTrue)
		convey.So(bit.IsPowerOfTwo(2), convey.ShouldBeTrue)
		convey.So(bit.IsPowerOfTwo(3), convey.ShouldBeFalse)
		convey.So(bit.IsPowerOfTwo(4), convey.ShouldBeTrue)
		convey.So(bit.IsPowerOfTwo(6), convey.ShouldBeFalse)
		convey.So(bit.IsPowerOfTwo(8), convey.ShouldBeTrue)
		convey.So(bit.IsPowerOfTwo(-8), convey.ShouldBeFalse)
		convey.So(bit.IsPowerOfTwo(-0), convey.ShouldBeFalse)
	})
}

func TestRightOneBitNum(t *testing.T) {
	convey.Convey("TestRightOneBitNum", t, func() {
		convey.So(bit.RightOneBitNum(0), convey.ShouldEqual, 0)
		convey.So(bit.RightOneBitNum(1), convey.ShouldEqual, 1)
		convey.So(bit.RightOneBitNum(2), convey.ShouldEqual, 2)
		convey.So(bit.RightOneBitNum(3), convey.ShouldEqual, 1)
		convey.So(bit.RightOneBitNum(4), convey.ShouldEqual, 4)
		convey.So(bit.RightOneBitNum(5), convey.ShouldEqual, 1)
		convey.So(bit.RightOneBitNum(8), convey.ShouldEqual, 8)
		convey.So(bit.RightOneBitNum(-8), convey.ShouldEqual, 8)
		convey.So(bit.RightOneBitNum(-15), convey.ShouldEqual, 1)
		convey.So(bit.RightOneBitNum(-14), convey.ShouldEqual, 2)
		convey.So(bit.RightOneBitNum(-12), convey.ShouldEqual, 4)
		convey.So(bit.RightOneBitNum(-16), convey.ShouldEqual, 16)
	})
}

func TestLeftOneBitNum(t *testing.T) {
	convey.Convey("TestLeftOneBitNum", t, func() {
		convey.So(bit.LeftOneBitNum(0), convey.ShouldEqual, 0)
		convey.So(bit.LeftOneBitNum(1), convey.ShouldEqual, 1)
		convey.So(bit.LeftOneBitNum(2), convey.ShouldEqual, 2)
		convey.So(bit.LeftOneBitNum(3), convey.ShouldEqual, 2)
		convey.So(bit.LeftOneBitNum(4), convey.ShouldEqual, 4)
		convey.So(bit.LeftOneBitNum(5), convey.ShouldEqual, 4)
		convey.So(bit.LeftOneBitNum(8), convey.ShouldEqual, 8)
		convey.So(bit.LeftOneBitNum(-8), convey.ShouldEqual, 0)
		convey.So(bit.LeftOneBitNum(-15), convey.ShouldEqual, 0)
		convey.So(bit.LeftOneBitNum(-14), convey.ShouldEqual, 0)
		convey.So(bit.LeftOneBitNum(-12), convey.ShouldEqual, 0)
		convey.So(bit.LeftOneBitNum(-16), convey.ShouldEqual, 0)
	})
}

func TestNextHighestPowerOfTwo(t *testing.T) {
	convey.Convey("TestNextHighestPowerOfTwo", t, func() {
		convey.So(bit.NextHighestPowerOfTwo(0), convey.ShouldEqual, 0)
		convey.So(bit.NextHighestPowerOfTwo(1), convey.ShouldEqual, 1)
		convey.So(bit.NextHighestPowerOfTwo(2), convey.ShouldEqual, 2)
		convey.So(bit.NextHighestPowerOfTwo(3), convey.ShouldEqual, 4)
		convey.So(bit.NextHighestPowerOfTwo(4), convey.ShouldEqual, 4)
		convey.So(bit.NextHighestPowerOfTwo(5), convey.ShouldEqual, 8)
		convey.So(bit.NextHighestPowerOfTwo(5), convey.ShouldEqual, 8)
		convey.So(bit.NextHighestPowerOfTwo(8), convey.ShouldEqual, 8)
		convey.So(bit.NextHighestPowerOfTwo(9), convey.ShouldEqual, 16)
		convey.So(bit.NextHighestPowerOfTwo(-8), convey.ShouldEqual, 0)
		convey.So(bit.NextHighestPowerOfTwo(10245), convey.ShouldEqual, 16384)
	})
}

func TestPreHighestPowerOfTwo(t *testing.T) {
	convey.Convey("TestPreHighestPowerOfTwo", t, func() {
		convey.So(bit.PreHighestPowerOfTwo(0), convey.ShouldEqual, 0)
		convey.So(bit.PreHighestPowerOfTwo(1), convey.ShouldEqual, 1)
		convey.So(bit.PreHighestPowerOfTwo(2), convey.ShouldEqual, 2)
		convey.So(bit.PreHighestPowerOfTwo(3), convey.ShouldEqual, 2)
		convey.So(bit.PreHighestPowerOfTwo(4), convey.ShouldEqual, 4)
		convey.So(bit.PreHighestPowerOfTwo(5), convey.ShouldEqual, 4)
		convey.So(bit.PreHighestPowerOfTwo(5), convey.ShouldEqual, 4)
		convey.So(bit.PreHighestPowerOfTwo(8), convey.ShouldEqual, 8)
		convey.So(bit.PreHighestPowerOfTwo(9), convey.ShouldEqual, 8)
		convey.So(bit.PreHighestPowerOfTwo(-8), convey.ShouldEqual, 0)
	})
}

func TestSwap(t *testing.T) {
	convey.Convey("TestSwap", t, func() {
		nums := []int{1, 2}
		bit.Swap(nums)
		convey.So(nums, convey.ShouldResemble, []int{2, 1})
		bit.Swap(nums)
		convey.So(nums, convey.ShouldResemble, []int{1, 2})
	})
}

func TestSum(t *testing.T) {
	convey.Convey("TestSum", t, func() {
		convey.So(bit.Sum(-1, 2), convey.ShouldEqual, 1)
		convey.So(bit.Sum(1, 2), convey.ShouldEqual, 3)
		convey.So(bit.Sum(1, math.MaxUint32), convey.ShouldEqual, 4294967296)
	})
}

func TestMaxBits(t *testing.T) {
	convey.Convey("TestMaxBits", t, func() {
		convey.So(bit.MaxBits(-1), convey.ShouldEqual, 0)
		convey.So(bit.MaxBits(0), convey.ShouldEqual, 0)
		convey.So(bit.MaxBits(1), convey.ShouldEqual, 1)
		convey.So(bit.MaxBits(2), convey.ShouldEqual, 2)
		convey.So(bit.MaxBits(3), convey.ShouldEqual, 2)
		convey.So(bit.MaxBits(4), convey.ShouldEqual, 3)
		convey.So(bit.MaxBits(math.MaxInt), convey.ShouldEqual, 63)
		convey.So(bit.MaxBits(math.MaxInt32), convey.ShouldEqual, 31)
		convey.So(bit.MaxBits(math.MaxInt64), convey.ShouldEqual, 63)
		convey.So(bit.MaxBits(uint(math.MaxUint)), convey.ShouldEqual, 64)
		convey.So(bit.MaxBits(uint32(math.MaxUint32)), convey.ShouldEqual, 32)
		convey.So(bit.MaxBits(uint64(math.MaxUint64)), convey.ShouldEqual, 64)
	})
}

func TestGetBit(t *testing.T) {
	convey.Convey("TestGetBit", t, func() {
		convey.So(func() { bit.GetBit(-1, -1) }, convey.ShouldPanic)
		convey.So(func() { bit.GetBit(1, -1) }, convey.ShouldPanic)
		convey.So(bit.GetBit(-1, 1), convey.ShouldEqual, 1)
		convey.So(bit.GetBit(5, 1), convey.ShouldEqual, 0)
		convey.So(bit.GetBit(5, 2), convey.ShouldEqual, 1)
		convey.So(bit.GetBit(5, 3), convey.ShouldEqual, 0)
		convey.So(bit.GetBit(math.MinInt, 64), convey.ShouldEqual, 1)
		convey.So(bit.GetBit(math.MaxInt, 64), convey.ShouldEqual, 0)
		convey.So(bit.GetBit(uint(math.MaxUint), 64), convey.ShouldEqual, 0)
	})
}

func TestReverseBit(t *testing.T) {
	convey.Convey("TestReverseBit", t, func() {
		convey.So(func() { bit.ReverseBit(-1, -1) }, convey.ShouldPanic)
		convey.So(func() { bit.ReverseBit(1, -1) }, convey.ShouldPanic)
		convey.So(bit.ReverseBit(-1, 0), convey.ShouldEqual, -2)
		convey.So(bit.ReverseBit(-1, 1), convey.ShouldEqual, -3)
		convey.So(bit.ReverseBit(5, 0), convey.ShouldEqual, 4)
		convey.So(bit.ReverseBit(5, 1), convey.ShouldEqual, 7)
		convey.So(bit.ReverseBit(5, 2), convey.ShouldEqual, 1)
		convey.So(bit.ReverseBit(5, 3), convey.ShouldEqual, 13)
		convey.So(bit.ReverseBit(math.MinInt, 64), convey.ShouldEqual, -9223372036854775808)
		convey.So(bit.ReverseBit(math.MaxInt, 63), convey.ShouldEqual, math.MaxInt-1<<63)
		convey.So(bit.ReverseBit(math.MaxInt, 64), convey.ShouldEqual, math.MaxInt)
		convey.So(bit.ReverseBit(math.MaxInt, 64), convey.ShouldEqual, 9223372036854775807)
		convey.So(bit.ReverseBit(uint(math.MaxUint), 64), convey.ShouldEqual, uint(math.MaxUint))
		convey.So(bit.ReverseBit(uint(math.MaxUint), 64), convey.ShouldEqual, uint(18446744073709551615))
	})
}

func TestSetBit(t *testing.T) {
	convey.Convey("TestSetBit", t, func() {
		convey.So(func() { bit.SetBit(-1, -1) }, convey.ShouldPanic)
		convey.So(func() { bit.SetBit(1, -1) }, convey.ShouldPanic)
		convey.So(bit.SetBit(-1, 0), convey.ShouldEqual, -1)
		convey.So(bit.SetBit(-1, 1), convey.ShouldEqual, -1)
		convey.So(bit.SetBit(5, 0), convey.ShouldEqual, 5)
		convey.So(bit.SetBit(5, 1), convey.ShouldEqual, 7)
		convey.So(bit.SetBit(5, 2), convey.ShouldEqual, 5)
		convey.So(bit.SetBit(5, 3), convey.ShouldEqual, 13)
		convey.So(bit.SetBit(math.MinInt, 64), convey.ShouldEqual, -9223372036854775808)
		convey.So(bit.SetBit(math.MaxInt, 64), convey.ShouldEqual, math.MaxInt)
		convey.So(bit.SetBit(math.MaxInt, 64), convey.ShouldEqual, 9223372036854775807)
		convey.So(bit.SetBit(uint(math.MaxUint), 64), convey.ShouldEqual, uint(math.MaxUint))
		convey.So(bit.SetBit(uint(math.MaxUint), 64), convey.ShouldEqual, uint(18446744073709551615))
	})
}

func TestUnsetBit(t *testing.T) {
	convey.Convey("TestUnsetBit", t, func() {
		convey.So(func() { bit.UnsetBit(-1, -1) }, convey.ShouldPanic)
		convey.So(func() { bit.UnsetBit(1, -1) }, convey.ShouldPanic)
		convey.So(bit.UnsetBit(-1, 1), convey.ShouldEqual, -3)
		convey.So(bit.UnsetBit(5, 0), convey.ShouldEqual, 4)
		convey.So(bit.UnsetBit(5, 1), convey.ShouldEqual, 5)
		convey.So(bit.UnsetBit(5, 2), convey.ShouldEqual, 1)
		convey.So(bit.UnsetBit(5, 3), convey.ShouldEqual, 5)
		convey.So(bit.UnsetBit(math.MinInt, 64), convey.ShouldEqual, -9223372036854775808)
		convey.So(bit.UnsetBit(math.MaxInt, 64), convey.ShouldEqual, math.MaxInt)
		convey.So(bit.UnsetBit(math.MaxInt, 64), convey.ShouldEqual, 9223372036854775807)
		convey.So(bit.UnsetBit(uint(math.MaxUint), 64), convey.ShouldEqual, uint(math.MaxUint))
		convey.So(bit.UnsetBit(uint(math.MaxUint), 64), convey.ShouldEqual, uint(18446744073709551615))
	})
}
