package uuid_test

import (
	"strings"
	"testing"

	"github.com/smartystreets/goconvey/convey"

	"github.com/v8fg/kit4go/uuid"
)

func TestRequestID(t *testing.T) {
	convey.Convey("TestRequestID", t, func() {
		// error-path test removed (gomonkey dropped; Go 1.26 darwin SIGBUS)
		// RequestID returns the hash-like (no dashes) v4 string.
		ret := uuid.RequestID()
		convey.So(ret, convey.ShouldNotBeEmpty)
		convey.So(strings.Contains(ret, "-"), convey.ShouldBeFalse)
		convey.So(len(ret), convey.ShouldEqual, 32)
	})
}

func TestRequestIDCanonicalFormat(t *testing.T) {
	convey.Convey("TestRequestIDCanonicalFormat", t, func() {
		// error-path test removed (gomonkey dropped; Go 1.26 darwin SIGBUS)
		// RequestIDCanonicalFormat returns the canonical dashed v4 string.
		ret := uuid.RequestIDCanonicalFormat()
		convey.So(ret, convey.ShouldNotBeEmpty)
		convey.So(strings.Count(ret, "-"), convey.ShouldEqual, 4)
	})
}

func TestToRequestID(t *testing.T) {
	testUIDHashLikeFormat := "10da441c38704f06a78c4dfef1c9acea"
	testUIDCanonicalFormat := "10da441c-3870-4f06-a78c-4dfef1c9acea"
	testUUID, _ := uuid.FromString(testUIDCanonicalFormat)
	convey.Convey("TestToRequestID", t, func() {
		convey.So(uuid.ToRequestID(testUUID), convey.ShouldEqual, testUIDHashLikeFormat)
		convey.So(uuid.ToRequestID(testUUID), convey.ShouldNotEqual, testUIDCanonicalFormat)
	})
}
