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
		// Empty slice: error contract (no panic).
		v, err := random.RandIn([]int{})
		convey.So(err, convey.ShouldEqual, random.ErrEmptySlice)
		convey.So(v, convey.ShouldEqual, 0)

		v2, err2 := random.RandIn([]int{1})
		convey.So(err2, convey.ShouldBeNil)
		convey.So(v2, convey.ShouldEqual, 1)

		v3, err3 := random.RandIn([]int{1, 2})
		convey.So(err3, convey.ShouldBeNil)
		convey.So(v3, convey.ShouldBeIn, []int{1, 2})

		v4, err4 := random.RandIn([]string{"1", "2"})
		convey.So(err4, convey.ShouldBeNil)
		convey.So(v4, convey.ShouldBeIn, []string{"1", "2"})

		v5, err5 := random.RandIn([]float64{1, 2})
		convey.So(err5, convey.ShouldBeNil)
		convey.So(v5, convey.ShouldBeIn, []float64{1, 2})

		v6, err6 := random.RandIn([]float32{1, 2})
		convey.So(err6, convey.ShouldBeNil)
		convey.So(v6, convey.ShouldNotBeIn, []float64{1, 2})
	})

}

func TestMustRandIn(t *testing.T) {
	convey.Convey("TestMustRandIn", t, func() {
		convey.So(func() { random.MustRandIn([]int{}) }, convey.ShouldPanic)
		convey.So(random.MustRandIn([]int{1}), convey.ShouldEqual, 1)
		convey.So(random.MustRandIn([]int{1, 2}), convey.ShouldBeIn, []int{1, 2})
		convey.So(random.MustRandIn([]string{"1", "2"}), convey.ShouldBeIn, []string{"1", "2"})
	})

}

func TestRandNIn(t *testing.T) {
	convey.Convey("TestRandNIn", t, func() {
		convey.So(func() { random.MustRandIn([]int{}) }, convey.ShouldPanic)
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

// TestRandStringWithKind_CombinedBits verifies the kind bitmask selects exactly
// the documented character groups, including the non-contiguous kind=5
// (digits+lowercase) that the old maxBits/clear-lowest-bit loop mapped wrong
// (it produced lowercase+uppercase). Each requested group must appear and each
// unrequested group must not, over a long draw.
func TestRandStringWithKind_CombinedBits(t *testing.T) {
	cases := []struct {
		kind              int
		wantDigit         bool
		wantUpper, wantLo bool
	}{
		{1, true, false, false},
		{2, false, true, false},
		{4, false, false, true},
		{3, true, true, false},
		{5, true, false, true}, // the regression: digits + lowercase
		{6, false, true, true},
		{7, true, true, true},
	}
	hasRange := func(s string, lo, hi byte) bool {
		for i := 0; i < len(s); i++ {
			if s[i] >= lo && s[i] <= hi {
				return true
			}
		}
		return false
	}
	for _, tc := range cases {
		s := string(random.RandStringWithKind(3000, tc.kind))

		// No character may fall outside the union of requested groups.
		for i := 0; i < len(s); i++ {
			c := s[i]
			ok := (tc.wantDigit && c >= '0' && c <= '9') ||
				(tc.wantUpper && c >= 'A' && c <= 'Z') ||
				(tc.wantLo && c >= 'a' && c <= 'z')
			if !ok {
				t.Errorf("kind=%d: char %q outside the requested groups", tc.kind, c)
				break
			}
		}
		if tc.wantDigit && !hasRange(s, '0', '9') {
			t.Errorf("kind=%d: expected digits, none appeared", tc.kind)
		}
		if tc.wantUpper && !hasRange(s, 'A', 'Z') {
			t.Errorf("kind=%d: expected uppercase, none appeared", tc.kind)
		}
		if tc.wantLo && !hasRange(s, 'a', 'z') {
			t.Errorf("kind=%d: expected lowercase, none appeared", tc.kind)
		}
		if !tc.wantDigit && hasRange(s, '0', '9') {
			t.Errorf("kind=%d: unexpected digits present", tc.kind)
		}
		if !tc.wantUpper && hasRange(s, 'A', 'Z') {
			t.Errorf("kind=%d: unexpected uppercase present", tc.kind)
		}
		if !tc.wantLo && hasRange(s, 'a', 'z') {
			t.Errorf("kind=%d: unexpected lowercase present", tc.kind)
		}
	}
}
