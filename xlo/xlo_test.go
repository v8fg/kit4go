package xlo_test

import (
	"strconv"
	"testing"

	"github.com/smartystreets/goconvey/convey"

	"github.com/v8fg/kit4go/xlo"
)

func TestFindUniques(t *testing.T) {
	convey.SetDefaultFailureMode(convey.FailureContinues)
	convey.Convey("TestFindUniques", t, func() {
		convey.Convey("int case", func() {
			convey.So(xlo.Uniq([]int{1, 2, 3}), convey.ShouldResemble, []int{1, 2, 3})
			convey.So(xlo.Uniq([]int{1, 2, 2, 3, 1, 2, 1, 2, 2, 3, 1, 2}), convey.ShouldResemble, []int{1, 2, 3})
			convey.So(xlo.Uniq([]int{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}), convey.ShouldResemble, []int{1})
		})
		convey.Convey("string case", func() {
			convey.So(xlo.Uniq([]string{"1", "2", "3"}), convey.ShouldResemble, []string{"1", "2", "3"})
			convey.So(xlo.Uniq([]string{"1", "2", "1", "2", "1"}), convey.ShouldResemble, []string{"1", "2"})
		})

	})
}

func TestLoFindUniques(t *testing.T) {
	convey.SetDefaultFailureMode(convey.FailureContinues)
	convey.Convey("TestLoFindUniques", t, func() {
		convey.Convey("int case", func() {
			convey.So(xlo.LoUniq([]int{1, 2, 3}), convey.ShouldResemble, []int{1, 2, 3})
			convey.So(xlo.LoUniq([]int{1, 2, 2, 3, 1, 2, 1, 2, 2, 3, 1, 2}), convey.ShouldResemble, []int{1, 2, 3})
			convey.So(xlo.LoUniq([]int{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}), convey.ShouldResemble, []int{1})
		})
		convey.Convey("string case", func() {
			convey.So(xlo.LoUniq([]string{"1", "2", "3"}), convey.ShouldResemble, []string{"1", "2", "3"})
			convey.So(xlo.LoUniq([]string{"1", "2", "1", "2", "1"}), convey.ShouldResemble, []string{"1", "2"})
		})
	})
}

func TestLoMap(t *testing.T) {
	convey.SetDefaultFailureMode(convey.FailureContinues)
	convey.Convey("TestLoMap", t, func() {
		original := []int{1, 2, 3}
		ret := xlo.LoMap(original, func(item int, index int) string {
			return strconv.FormatInt(int64(item), 10)
		})
		convey.So(ret, convey.ShouldResemble, []string{"1", "2", "3"})
	})
}

func TestLopMap(t *testing.T) {
	convey.SetDefaultFailureMode(convey.FailureContinues)
	convey.Convey("TestLopMap", t, func() {
		original := []int{1, 2, 3}
		ret := xlo.LopMap(original, func(item int, index int) string {
			return strconv.FormatInt(int64(item), 10)
		})
		convey.So(ret, convey.ShouldResemble, []string{"1", "2", "3"})
	})
}
