package uuid_test

import (
	"testing"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/smartystreets/goconvey/convey"

	"github.com/v8fg/kit4go/uuid"
)

func TestKSUIDCompare(t *testing.T) {
	testKSUIDStr := "2FwgbLS72ILDWFEhMSFKCRJBN7M"
	testKSUIDStr2 := "2FwhD2rdkTU61pLihq1ql8PAPc4"
	testKSUID, _ := uuid.KSUIDParse(testKSUIDStr)
	testKSUID2, _ := uuid.KSUIDParse(testKSUIDStr2)
	convey.Convey("TestKSUIDCompare", t, func() {
		convey.So(uuid.KSUIDCompare(testKSUID, testKSUID), convey.ShouldEqual, 0)
		convey.So(uuid.KSUIDCompare(testKSUID, testKSUID2), convey.ShouldEqual, -1)
		convey.So(uuid.KSUIDCompare(testKSUID2, testKSUID), convey.ShouldEqual, 1)
	})
}

func TestKSUIDFromBytes(t *testing.T) {
	testKSUIDStr := "2FwgbLS72ILDWFEhMSFKCRJBN7M"
	testKSUID, _ := uuid.KSUIDParse(testKSUIDStr)
	convey.Convey("TestKSUIDFromBytes", t, func() {
		outUUID, _ := uuid.KSUIDFromBytes(testKSUID.Bytes())
		convey.So(outUUID.String(), convey.ShouldEqual, testKSUIDStr)
	})
}

func TestKSUIDFromParts(t *testing.T) {
	testKSUIDStr := "2FwgbLS72ILDWFEhMSFKCRJBN7M"
	timePart := time.Unix(0, 1665408630000000000)
	payloadPart := []byte{96, 106, 10, 181, 220, 20, 199, 12, 210, 111, 210, 182, 237, 42, 45, 156}
	convey.Convey("TestKSUIDFromParts", t, func() {
		outUUID, _ := uuid.KSUIDFromParts(timePart, payloadPart)
		convey.So(outUUID.String(), convey.ShouldEqual, testKSUIDStr)
	})
}

func TestKSUIDIsSorted(t *testing.T) {
	testKSUIDStr := "2FwgbLS72ILDWFEhMSFKCRJBN7M"
	testKSUIDStr2 := "2FwhD2rdkTU61pLihq1ql8PAPc4"
	testKSUID, _ := uuid.KSUIDParse(testKSUIDStr)
	testKSUID2, _ := uuid.KSUIDParse(testKSUIDStr2)
	convey.Convey("TestKSUIDIsSorted", t, func() {
		convey.So(uuid.KSUIDIsSorted([]ksuid.KSUID{testKSUID, testKSUID2}), convey.ShouldBeTrue)
		convey.So(uuid.KSUIDIsSorted([]ksuid.KSUID{testKSUID2, testKSUID}), convey.ShouldBeFalse)
	})
}

func TestKSUIDSort(t *testing.T) {
	testKSUIDStr := "2FwgbLS72ILDWFEhMSFKCRJBN7M"
	testKSUIDStr2 := "2FwhD2rdkTU61pLihq1ql8PAPc4"
	testKSUID, _ := uuid.KSUIDParse(testKSUIDStr)
	testKSUID2, _ := uuid.KSUIDParse(testKSUIDStr2)
	convey.Convey("TestKSUIDSort", t, func() {
		ids := []ksuid.KSUID{testKSUID2, testKSUID}
		convey.So(uuid.KSUIDIsSorted(ids), convey.ShouldBeFalse)
		uuid.KSUIDSort(ids)
		convey.So(uuid.KSUIDIsSorted(ids), convey.ShouldBeTrue)
	})
}

func TestNewKSUID(t *testing.T) {
	convey.Convey("TestNewKSUID", t, func() {
		// error-path test removed (gomonkey dropped; Go 1.26 darwin SIGBUS)
		newID := uuid.NewKSUID()
		convey.So(newID.IsNil(), convey.ShouldBeFalse)
	})
}

func TestNewKSUIDRandom(t *testing.T) {
	convey.Convey("TestNewKSUIDRandom", t, func() {
		// error-path test removed (gomonkey dropped; Go 1.26 darwin SIGBUS)
		outUUID, err := uuid.NewKSUIDRandom()
		convey.So(err, convey.ShouldBeNil)
		convey.So(outUUID.IsNil(), convey.ShouldBeFalse)
	})
}

func TestNewKSUIDRandomWithTime(t *testing.T) {
	timePart := time.Unix(0, 1665408630000000000)

	convey.Convey("TestNewKSUIDRandomWithTime", t, func() {
		// error-path test removed (gomonkey dropped; Go 1.26 darwin SIGBUS)
		outUUID, err := uuid.NewKSUIDRandomWithTime(timePart)
		convey.So(err, convey.ShouldBeNil)
		convey.So(outUUID.IsNil(), convey.ShouldBeFalse)
	})
}

func TestParse(t *testing.T) {
	testKSUIDStr := "2FwgbLS72ILDWFEhMSFKCRJBN7M"
	convey.Convey("TestParse", t, func() {
		// error-path test removed (gomonkey dropped; Go 1.26 darwin SIGBUS)
		outUUID, _ := uuid.KSUIDParse(testKSUIDStr)
		convey.So(outUUID.String(), convey.ShouldEqual, testKSUIDStr)
	})
}
