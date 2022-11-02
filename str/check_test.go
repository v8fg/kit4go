package str_test

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"

	"github.com/v8fg/kit4go/str"
)

func TestCharIsAlphabet(t *testing.T) {
	convey.Convey("TestCharIsAlphabet", t, func() {
		convey.So(str.CharIsAlphabet('a'), convey.ShouldBeTrue)
		convey.So(str.CharIsAlphabet('A'), convey.ShouldBeTrue)
		convey.So(str.CharIsAlphabet('0'), convey.ShouldBeFalse)
		convey.So(str.CharIsAlphabet('9'), convey.ShouldBeFalse)
	})
}

func TestCharIsNumber(t *testing.T) {
	convey.Convey("TestCharIsNumber", t, func() {
		convey.So(str.CharIsNumber('a'), convey.ShouldBeFalse)
		convey.So(str.CharIsNumber('A'), convey.ShouldBeFalse)
		convey.So(str.CharIsNumber('0'), convey.ShouldBeTrue)
		convey.So(str.CharIsNumber('9'), convey.ShouldBeTrue)
	})
}

func TestContainsAll(t *testing.T) {
	convey.Convey("TestContainsAll", t, func() {
		convey.So(str.ContainsAll("Go is the best language", []string{"go"}...), convey.ShouldBeFalse)
		convey.So(str.ContainsAll("Go is the best language", []string{"Go", "go"}...), convey.ShouldBeFalse)
		convey.So(str.ContainsAll("Go is the best language", []string{"Go"}...), convey.ShouldBeTrue)
		convey.So(str.ContainsAll("Go is the best language", "Go"), convey.ShouldBeTrue)
	})
}

func TestContainsAny(t *testing.T) {
	convey.Convey("TestContainsAny", t, func() {
		convey.So(str.ContainsAny("Go is the best language", []string{"go"}...), convey.ShouldBeFalse)
		convey.So(str.ContainsAny("Go is the best language", []string{"Go", "go"}...), convey.ShouldBeTrue)
		convey.So(str.ContainsAny("Go is the best language", []string{"Go"}...), convey.ShouldBeTrue)
		convey.So(str.ContainsAny("Go is the best language", "Go"), convey.ShouldBeTrue)
	})
}

func TestEndWithAny(t *testing.T) {
	convey.Convey("TestEndWithAny", t, func() {
		convey.So(str.EndWithAny("Go is the best language", []string{"go"}...), convey.ShouldBeFalse)
		convey.So(str.EndWithAny("Go is the best language", []string{"language"}...), convey.ShouldBeTrue)
		convey.So(str.EndWithAny("Go is the best language", []string{"go", "language"}...), convey.ShouldBeTrue)
		convey.So(str.EndWithAny("Go is the best language", "go", "language"), convey.ShouldBeTrue)
	})
}

func TestEqualIgnoreCase(t *testing.T) {
	convey.Convey("TestEqualIgnoreCase", t, func() {
		convey.So(str.EqualIgnoreCase("a", "A"), convey.ShouldBeTrue)
		convey.So(str.EqualIgnoreCase("a", "a"), convey.ShouldBeTrue)
	})
}

func TestIsEmpty(t *testing.T) {
	convey.Convey("TestIsEmpty", t, func() {
		convey.So(str.IsEmpty(""), convey.ShouldBeTrue)
		convey.So(str.IsEmpty(" "), convey.ShouldBeFalse)
		convey.So(str.IsEmpty("   "), convey.ShouldBeFalse)
	})
}

func TestIsBlank(t *testing.T) {
	convey.Convey("TestIsBlank", t, func() {
		convey.So(str.IsBlank(""), convey.ShouldBeTrue)
		convey.So(str.IsBlank(" "), convey.ShouldBeTrue)
		convey.So(str.IsBlank("   "), convey.ShouldBeTrue)
	})
}

func TestStartWithAny(t *testing.T) {
	convey.Convey("TestStartWithAny", t, func() {
		convey.So(str.StartWithAny("Go is the best language", []string{"Go"}...), convey.ShouldBeTrue)
		convey.So(str.StartWithAny("Go is the best language", []string{"go", "Go"}...), convey.ShouldBeTrue)
		convey.So(str.StartWithAny("Go is the best language", []string{"go"}...), convey.ShouldBeFalse)
		convey.So(str.StartWithAny("Go is the best language", "go"), convey.ShouldBeFalse)
	})
}
