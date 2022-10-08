package random_test

import (
	"math"
	"testing"

	"github.com/smartystreets/goconvey/convey"

	"github.com/v8fg/kit4go/random"
)

func TestRandUniCodeByUID(t *testing.T) {
	convey.Convey("TestRandUniCodeByUID", t, func() {
		convey.So(random.RandUniCodeByUID(0, 0), convey.ShouldHaveLength, 0)
		convey.So(random.RandUniCodeByUID(0, 1), convey.ShouldHaveLength, 1)
		convey.So(random.RandUniCodeByUID(0, 2), convey.ShouldHaveLength, 2)
		convey.So(random.RandUniCodeByUID(0, 0), convey.ShouldEqual, "")
		convey.So(random.RandUniCodeByUID(0, 1), convey.ShouldEqual, "5")
		convey.So(random.RandUniCodeByUID(0, 2), convey.ShouldEqual, "5w")
		convey.So(random.RandUniCodeByUID(0, 3), convey.ShouldEqual, "5Cw")
		convey.So(random.RandUniCodeByUID(0, 4), convey.ShouldEqual, "5iCw")
	})

}

func TestRandUniCodeByUIDWithSalt(t *testing.T) {
	convey.Convey("TestRandUniCodeByUIDWithSalt", t, func() {
		convey.So(random.RandUniCodeByUIDWithSalt(0, 0, 0), convey.ShouldHaveLength, 0)
		convey.So(random.RandUniCodeByUIDWithSalt(0, 1, 0), convey.ShouldHaveLength, 1)
		convey.So(random.RandUniCodeByUIDWithSalt(0, 2, 0), convey.ShouldHaveLength, 2)
		convey.So(random.RandUniCodeByUIDWithSalt(0, 0, 0), convey.ShouldEqual, "")
		convey.So(random.RandUniCodeByUIDWithSalt(0, 1, 0), convey.ShouldEqual, "0")
		convey.So(random.RandUniCodeByUIDWithSalt(0, 2, 0), convey.ShouldEqual, "00")
		convey.So(random.RandUniCodeByUIDWithSalt(0, 3, 0), convey.ShouldEqual, "000")
		convey.So(random.RandUniCodeByUIDWithSalt(0, 4, 0), convey.ShouldEqual, "0000")
		convey.So(random.RandUniCodeByUIDWithSalt(1, 1, math.MaxInt), convey.ShouldEqual, "a")
		convey.So(random.RandUniCodeByUIDWithSalt(1, 2, math.MaxInt), convey.ShouldEqual, "aW")
		convey.So(random.RandUniCodeByUIDWithSalt(1, 3, math.MaxInt), convey.ShouldEqual, "asW")
		convey.So(random.RandUniCodeByUIDWithSalt(1, 4, math.MaxInt), convey.ShouldEqual, "azsW")
		convey.So(random.RandUniCodeByUIDWithSalt(1, 10, math.MaxInt), convey.ShouldEqual, "agcWDOsrLz")
		convey.So(random.RandUniCodeByUIDWithSalt(1, 11, math.MaxInt), convey.ShouldEqual, "agcWDOsrLz")
	})
}

func TestRandIn(t *testing.T) {
	convey.Convey("TestRandIn", t, func() {
		convey.So(func() { random.RandIn([]int{}) }, convey.ShouldPanic)
		convey.So(random.RandIn([]int{1}), convey.ShouldEqual, 1)
		convey.So(random.RandIn([]int{1, 2}), convey.ShouldBeIn, []int{1, 2})
		convey.So(random.RandIn([]string{"1", "2"}), convey.ShouldBeIn, []string{"1", "2"})
		convey.So(random.RandIn([]float64{1, 2}), convey.ShouldBeIn, []float64{1, 2})
		convey.So(random.RandIn([]float32{1, 2}), convey.ShouldNotBeIn, []float64{1, 2})
	})

}

func TestRandNIn(t *testing.T) {
	convey.Convey("TestRandNIn", t, func() {
		convey.So(func() { random.RandIn([]int{}) }, convey.ShouldPanic)
		convey.So(random.RandNIn(0, []int{}), convey.ShouldResemble, []int{})
		convey.So(random.RandNIn(0, []int{1}), convey.ShouldHaveLength, 0)
		convey.So(random.RandNIn(0, []int{1}), convey.ShouldResemble, []int{})
		convey.So(random.RandNIn(1, []int{}), convey.ShouldResemble, []int{})
		convey.So(random.RandNIn(1, []int{1}), convey.ShouldResemble, []int{1})
		convey.So(random.RandNIn(2, []int{1}), convey.ShouldResemble, []int{1})
		convey.So(random.RandNIn(1, []int{0, 1}), convey.ShouldHaveLength, 1)
		convey.So(random.RandNIn(1, []string{"1", "2"}), convey.ShouldHaveLength, 1)
		convey.So(random.RandNIn(2, []string{"1", "2"}), convey.ShouldHaveLength, 2)
		convey.So(random.RandNIn(2, []float64{1, 2}), convey.ShouldHaveLength, 2)
		convey.So(random.RandNIn(2, []float32{2}), convey.ShouldResemble, []float32{2})
		convey.So(random.RandNIn(2, []float32{2}), convey.ShouldNotEqual, []float64{2})
	})

}

func TestRandStringInCharset(t *testing.T) {
	convey.Convey("TestRandStringInCharset", t, func() {
		convey.So(func() { random.RandStringInCharset(0, []rune{}) }, convey.ShouldPanic)
		convey.So(random.RandStringInCharset(0, []rune{1}), convey.ShouldEqual, "")
		convey.So(random.RandStringInCharset(1, []rune{'A'}), convey.ShouldEqual, "A")
		convey.So(random.RandStringInCharset(2, []rune("AB")), convey.ShouldHaveLength, 2)
	})
}

func TestRandStringWithKind(t *testing.T) {
	convey.Convey("TestRandStringWithKind", t, func() {
		convey.So(random.RandStringWithKind(0, 0), convey.ShouldResemble, []byte{})
		convey.So(random.RandStringWithKind(0, 8), convey.ShouldResemble, []byte{})
		convey.So(string(random.RandStringWithKind(1, 1)), convey.ShouldBeBetweenOrEqual, "0", "9")
		convey.So(string(random.RandStringWithKind(1, 2)), convey.ShouldBeBetweenOrEqual, "A", "Z")
		convey.So(string(random.RandStringWithKind(1, 4)), convey.ShouldBeBetweenOrEqual, "a", "z")
	})
}

func TestRandStringWithLetter(t *testing.T) {
	convey.Convey("TestRandStringWithLetter", t, func() {
		convey.So(random.RandStringWithLetter(0), convey.ShouldEqual, "")
		convey.So(random.RandStringWithLetter(1), convey.ShouldBeBetweenOrEqual, "A", "z")
		convey.So(random.RandStringWithLetter(66), convey.ShouldHaveLength, 66)
	})
}

func TestRandStringWithLetterDigits(t *testing.T) {
	convey.Convey("TestRandStringWithLetterDigits", t, func() {
		convey.So(random.RandStringWithLetterDigits(0), convey.ShouldEqual, "")
		convey.So(random.RandStringWithLetterDigits(1), convey.ShouldBeBetweenOrEqual, "0", "z")
		convey.So(random.RandStringWithLetterDigits(66), convey.ShouldHaveLength, 66)
	})
}
