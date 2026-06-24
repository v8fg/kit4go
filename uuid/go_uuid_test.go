package uuid_test

import (
	"testing"

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
	convey.Convey("TestNewV1", t, func() {
		// error-path test removed (gomonkey dropped; Go 1.26 darwin SIGBUS)
		newUUID := uuid.NewV1()
		convey.So(newUUID, convey.ShouldNotResemble, uid.Nil)
		convey.So(newUUID.Version(), convey.ShouldEqual, uid.V1)
	})
}

func TestNewV2(t *testing.T) {
	convey.Convey("TestNewV2", t, func() {
		// error-path test removed (gomonkey dropped; Go 1.26 darwin SIGBUS)
		newUUID := uuid.NewV2(byte(188))
		convey.So(newUUID, convey.ShouldNotResemble, uid.Nil)
		convey.So(newUUID.Version(), convey.ShouldEqual, uid.V2)
	})
}

func TestNewV3(t *testing.T) {
	testUIDCanonicalFormat := "10da441c-3870-4f06-a78c-4dfef1c9acea"
	testUUID, _ := uuid.FromString(testUIDCanonicalFormat)
	convey.Convey("TestNewV3", t, func() {
		// error-path test removed (gomonkey dropped; Go 1.26 darwin SIGBUS)
		newUUID := uuid.NewV3(testUUID, "xwi88")
		convey.So(newUUID, convey.ShouldNotResemble, uid.Nil)
		convey.So(newUUID.Version(), convey.ShouldEqual, uid.V3)
	})
}

func TestNewV4(t *testing.T) {
	convey.Convey("TestNewV4", t, func() {
		// error-path test removed (gomonkey dropped; Go 1.26 darwin SIGBUS)
		newUUID := uuid.NewV4()
		convey.So(newUUID, convey.ShouldNotResemble, uid.Nil)
		convey.So(newUUID.Version(), convey.ShouldEqual, uid.V4)
	})
}

func TestNewV5(t *testing.T) {
	testUIDCanonicalFormat := "10da441c-3870-4f06-a78c-4dfef1c9acea"
	testUUID, _ := uuid.FromString(testUIDCanonicalFormat)
	convey.Convey("TestNewV5", t, func() {
		// error-path test removed (gomonkey dropped; Go 1.26 darwin SIGBUS)
		newUUID := uuid.NewV5(testUUID, "xwi88")
		convey.So(newUUID, convey.ShouldNotResemble, uid.Nil)
		convey.So(newUUID.Version(), convey.ShouldEqual, uid.V5)
	})
}
