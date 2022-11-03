package str_test

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"

	"github.com/v8fg/kit4go/str"
)

func init() {
	convey.SetDefaultFailureMode(convey.FailureContinues)
}

func TestBytesToString(t *testing.T) {
	convey.Convey("TestBytesToString", t, func() {
		convey.So(str.StringToBytes(""), convey.ShouldBeNil)
		convey.So(str.StringToBytes(" "), convey.ShouldResemble, []byte{32})
		convey.So(str.StringToBytes("Go"), convey.ShouldResemble, []byte{71, 111})
	})
}

func TestStringToBytes(t *testing.T) {
	convey.Convey("TestStringToBytes", t, func() {
		convey.So(str.BytesToString(nil), convey.ShouldBeEmpty)
		convey.So(str.BytesToString([]byte{32}), convey.ShouldEqual, " ")
		convey.So(str.BytesToString([]byte{71, 111}), convey.ShouldEqual, "Go")
	})
}
