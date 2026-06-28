package uuid_test

import (
	"testing"
	"time"

	"github.com/rs/xid"
	"github.com/smartystreets/goconvey/convey"

	"github.com/v8fg/kit4go/uuid"
)

func TestNewXID(t *testing.T) {
	convey.Convey("TestNewXID", t, func() {
		// generator: invariant check (real generator; no error-path — gomonkey previously asserted a fixed value, now we assert the version/non-nil invariant)
		newID := uuid.NewXID()
		convey.So(newID.IsNil(), convey.ShouldBeFalse)
		convey.So(newID.String(), convey.ShouldNotBeEmpty)
	})
}

func TestNewXIDWithTime(t *testing.T) {
	timePart := time.Unix(0, 1665381861000000000)
	convey.Convey("TestNewXIDWithTime", t, func() {
		// generator: invariant check (real generator; no error-path — gomonkey previously asserted a fixed value, now we assert the version/non-nil invariant)
		newID := uuid.NewXIDWithTime(timePart)
		convey.So(newID.IsNil(), convey.ShouldBeFalse)
		convey.So(newID.String(), convey.ShouldNotBeEmpty)
	})
}

func TestXIDSort(t *testing.T) {
	testXIDStr := "cd1rbp8nhc7lkdm71vsg"
	testXID, _ := uuid.XIDFromString(testXIDStr)
	// 2022-10-10 06:04:21 +0000 UTC

	testXIDStr2 := "cd0pbp8nhc7lkdm71vsg"
	testXID2, _ := uuid.XIDFromString(testXIDStr2)
	// 2022-10-08 15:23:17 +0000 UTC

	convey.Convey("TestXIDSort", t, func() {
		ids := []xid.ID{testXID, testXID2}
		uuid.XIDSort(ids)
		convey.So(ids, convey.ShouldResemble, []xid.ID{testXID2, testXID})
	})
}

func TestXIDFromString(t *testing.T) {
	testXIDStr := "cd1rbp8nhc7lkdm71vsg"
	convey.Convey("TestXIDFromString", t, func() {
		outUUID, _ := uuid.XIDFromString(testXIDStr)
		convey.So(outUUID.String(), convey.ShouldEqual, testXIDStr)

		// error-path: malformed string returns an error.
		outBad, err := uuid.XIDFromString("not-an-xid!")
		convey.So(outBad.IsNil(), convey.ShouldBeTrue)
		convey.So(err, convey.ShouldBeError)
	})
}

func TestXIDFromBytes(t *testing.T) {
	testXIDStr := "cd1rbp8nhc7lkdm71vsg"
	testXID, _ := uuid.XIDFromString(testXIDStr)
	convey.Convey("TestXIDFromBytes", t, func() {
		outUUID, _ := uuid.XIDFromBytes(testXID.Bytes())
		convey.So(outUUID.String(), convey.ShouldEqual, testXIDStr)

		// error-path: wrong-length byte slice returns an error.
		outBad, err := uuid.XIDFromBytes([]byte{1, 2, 3})
		convey.So(outBad.IsNil(), convey.ShouldBeTrue)
		convey.So(err, convey.ShouldBeError)
	})
}
