package uuid_test

import (
	"testing"

	"github.com/agiledragon/gomonkey"
	uid "github.com/satori/go.uuid"
	"github.com/smartystreets/goconvey/convey"

	"github.com/v8fg/kit4go/uuid"
)

func TestEqual(t *testing.T) {
	convey.Convey("TestEqual", t, func() {
		testUIDCanonicalFormat := "10da441c-3870-4f06-a78c-4dfef1c9acea"
		testUUIDv4, _ := uuid.FromString(testUIDCanonicalFormat)
		testUUIDHashLikeFormat := "10da441c38704f06a78c4dfef1c9acea"
		testUUIDv4ByHashLike, _ := uuid.FromString(testUUIDHashLikeFormat)
		convey.So(uuid.Equal(testUUIDv4, testUUIDv4ByHashLike), convey.ShouldBeTrue)

		testUUIDHashLikeFormat2 := "10da441c38704f06a78c4dfef1c9acef"
		testUUIDv4ByHashLike2, _ := uuid.FromString(testUUIDHashLikeFormat2)
		convey.So(uuid.Equal(testUUIDv4, testUUIDv4ByHashLike2), convey.ShouldBeFalse)
	})
}

func TestFromBytes(t *testing.T) {
	convey.Convey("TestFromBytes", t, func() {
		testUIDCanonicalFormat := "10da441c-3870-4f06-a78c-4dfef1c9acea"
		testUUIDv4, _ := uuid.FromString(testUIDCanonicalFormat)
		outUUID, _ := uuid.FromBytes(testUUIDv4.Bytes())
		convey.So(outUUID.String(), convey.ShouldEqual, testUIDCanonicalFormat)

		testUIDHashLikeFormat := "10da441c38704f06a78c4dfef1c9acea"
		testUUIDv4, _ = uuid.FromString(testUIDHashLikeFormat)
		outUUID, _ = uuid.FromBytes(testUUIDv4.Bytes())
		convey.So(outUUID.String(), convey.ShouldEqual, testUIDCanonicalFormat)
	})
}

func TestFromBytesOrNil(t *testing.T) {
	convey.Convey("TestFromBytesOrNil", t, func() {
		testUIDCanonicalFormat := "10da441c-3870-4f06-a78c-4dfef1c9acea"
		testUUIDv4, _ := uuid.FromString(testUIDCanonicalFormat)
		convey.So(uuid.FromBytesOrNil(testUUIDv4.Bytes()).String(), convey.ShouldEqual, testUIDCanonicalFormat)

		testUIDHashLikeFormat := "10da441c38704f06a78c4dfef1c9acea"
		testUUIDv4, _ = uuid.FromString(testUIDHashLikeFormat)
		convey.So(uuid.FromBytesOrNil(testUUIDv4.Bytes()).String(), convey.ShouldEqual, testUIDCanonicalFormat)
	})
}

func TestFromString(t *testing.T) {
	convey.Convey("TestFromString", t, func() {
		testUIDCanonicalFormat := "10da441c-3870-4f06-a78c-4dfef1c9acea"
		outUUID, _ := uuid.FromString(testUIDCanonicalFormat)
		convey.So(outUUID.String(), convey.ShouldEqual, testUIDCanonicalFormat)

		testUIDHashLikeFormat := "10da441c38704f06a78c4dfef1c9acea"
		outUUID, _ = uuid.FromString(testUIDHashLikeFormat)
		convey.So(outUUID.String(), convey.ShouldEqual, testUIDCanonicalFormat)
	})
}

func TestFromStringOrNil(t *testing.T) {
	convey.Convey("TestFromStringOrNil", t, func() {
		testUIDCanonicalFormat := "10da441c-3870-4f06-a78c-4dfef1c9acea"
		convey.So(uuid.FromStringOrNil(testUIDCanonicalFormat).String(), convey.ShouldEqual, testUIDCanonicalFormat)

		testUIDHashLikeFormat := "10da441c38704f06a78c4dfef1c9acea"
		convey.So(uuid.FromStringOrNil(testUIDHashLikeFormat).String(), convey.ShouldEqual, testUIDCanonicalFormat)
	})
}

func TestNewV1(t *testing.T) {
	testUUIDv1CanonicalFormat := "5e671b94-47c2-11ed-b757-acde48001122"
	testUUIDv1, _ := uuid.FromString(testUUIDv1CanonicalFormat)
	convey.Convey("TestNewV1", t, func() {
		outputs := []gomonkey.OutputCell{
			{Values: gomonkey.Params{testUUIDv1}, Times: 1},
		}
		af := gomonkey.ApplyFuncSeq(uid.NewV1, outputs)
		defer af.Reset()

		convey.So(uuid.NewV1().String(), convey.ShouldEqual, testUUIDv1CanonicalFormat)
	})
}

func TestNewV2(t *testing.T) {
	testUUIDv2CanonicalFormat := "00000000-47c2-21ed-91bc-acde48001122"
	testUUIDv2, _ := uuid.FromString(testUUIDv2CanonicalFormat)
	convey.Convey("TestNewV2", t, func() {
		outputs := []gomonkey.OutputCell{
			{Values: gomonkey.Params{testUUIDv2}, Times: 1},
		}
		af := gomonkey.ApplyFuncSeq(uid.NewV2, outputs)
		defer af.Reset()

		convey.So(uuid.NewV2(byte(188)).String(), convey.ShouldEqual, testUUIDv2CanonicalFormat)
	})
}

func TestNewV3(t *testing.T) {
	testUIDCanonicalFormat := "10da441c-3870-4f06-a78c-4dfef1c9acea"
	testUUID, _ := uuid.FromString(testUIDCanonicalFormat)
	testUUIDv3CanonicalFormat := "2e42a1d8-6abc-3d81-be7d-3a390faa2624"
	testUUIDv3, _ := uuid.FromString(testUUIDv3CanonicalFormat)
	convey.Convey("TestNewV3", t, func() {
		outputs := []gomonkey.OutputCell{
			{Values: gomonkey.Params{testUUIDv3}, Times: 1},
		}
		af := gomonkey.ApplyFuncSeq(uid.NewV3, outputs)
		defer af.Reset()

		convey.So(uuid.NewV3(testUUID, "xwi88").String(), convey.ShouldEqual, testUUIDv3CanonicalFormat)
	})
}

func TestNewV4(t *testing.T) {
	testUIDCanonicalFormat := "10da441c-3870-4f06-a78c-4dfef1c9acea"
	testUUID, _ := uuid.FromString(testUIDCanonicalFormat)
	convey.Convey("TestNewV4", t, func() {
		outputs := []gomonkey.OutputCell{
			{Values: gomonkey.Params{testUUID}, Times: 1},
		}
		af := gomonkey.ApplyFuncSeq(uid.NewV4, outputs)
		defer af.Reset()

		convey.So(uuid.NewV4().String(), convey.ShouldEqual, testUIDCanonicalFormat)
	})
}

func TestNewV5(t *testing.T) {
	testUIDCanonicalFormat := "10da441c-3870-4f06-a78c-4dfef1c9acea"
	testUUID, _ := uuid.FromString(testUIDCanonicalFormat)
	testUUIDv5CanonicalFormat := "87b0df4e-1b06-5bea-b9e2-aec9b809f1db"
	testUUIDv5, _ := uuid.FromString(testUUIDv5CanonicalFormat)
	convey.Convey("TestNewV5", t, func() {
		outputs := []gomonkey.OutputCell{
			{Values: gomonkey.Params{testUUIDv5}, Times: 1},
		}
		af := gomonkey.ApplyFuncSeq(uid.NewV5, outputs)
		defer af.Reset()

		convey.So(uuid.NewV5(testUUID, "xwi88").String(), convey.ShouldEqual, testUUIDv5CanonicalFormat)
	})
}
