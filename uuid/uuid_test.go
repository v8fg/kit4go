package uuid_test

import (
	"testing"

	"github.com/agiledragon/gomonkey"
	"github.com/smartystreets/goconvey/convey"

	"github.com/v8fg/kit4go/uuid"
)

func TestRequestID(t *testing.T) {
	testUIDHashLikeFormat := "10da441c38704f06a78c4dfef1c9acea"
	testUIDCanonicalFormat := "10da441c-3870-4f06-a78c-4dfef1c9acea"
	testUUID, _ := uuid.FromString(testUIDCanonicalFormat)
	convey.Convey("TestRequestID", t, func() {
		outputs := []gomonkey.OutputCell{
			{Values: gomonkey.Params{testUUID}, Times: 1},
		}
		af := gomonkey.ApplyFuncSeq(uuid.NewV4, outputs)
		defer af.Reset()

		convey.So(uuid.RequestID(), convey.ShouldEqual, testUIDHashLikeFormat)
	})
}

func TestRequestIDCanonicalFormat(t *testing.T) {
	testUIDHashLikeFormat := "10da441c38704f06a78c4dfef1c9acea"
	testUIDCanonicalFormat := "10da441c-3870-4f06-a78c-4dfef1c9acea"
	testUUID, _ := uuid.FromString(testUIDHashLikeFormat)
	convey.Convey("TestRequestIDCanonicalFormat", t, func() {
		outputs := []gomonkey.OutputCell{
			{Values: gomonkey.Params{testUUID}, Times: 1},
		}
		af := gomonkey.ApplyFuncSeq(uuid.NewV4, outputs)
		defer af.Reset()

		convey.So(uuid.RequestIDCanonicalFormat(), convey.ShouldEqual, testUIDCanonicalFormat)
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
