package uuid_test

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/rs/xid"
	"github.com/segmentio/ksuid"

	"github.com/v8fg/kit4go/uuid"
)

func ExampleRequestID() {
	testUIDHashLikeFormat := "10da441c38704f06a78c4dfef1c9acea"
	testUIDCanonicalFormat := "10da441c-3870-4f06-a78c-4dfef1c9acea"
	testUUID, _ := uuid.FromString(testUIDHashLikeFormat)
	testUUID2, _ := uuid.FromString(testUIDCanonicalFormat)
	fmt.Printf("[ExampleRequestID] %v\n", uuid.ToRequestID(testUUID))
	fmt.Printf("[ExampleRequestID] %v\n", uuid.ToRequestID(testUUID2))

	// output:
	// [ExampleRequestID] 10da441c38704f06a78c4dfef1c9acea
	// [ExampleRequestID] 10da441c38704f06a78c4dfef1c9acea

}

func ExampleRequestIDCanonicalFormat() {
	testUIDHashLikeFormat := "10da441c38704f06a78c4dfef1c9acea"
	testUIDCanonicalFormat := "10da441c-3870-4f06-a78c-4dfef1c9acea"
	testUUID, _ := uuid.FromString(testUIDHashLikeFormat)
	testUUID2, _ := uuid.FromString(testUIDCanonicalFormat)
	fmt.Printf("[ExampleRequestIDCanonicalFormat] %v\n", testUUID.String())
	fmt.Printf("[ExampleRequestIDCanonicalFormat] %v\n", testUUID2.String())

	// output:
	// [ExampleRequestIDCanonicalFormat] 10da441c-3870-4f06-a78c-4dfef1c9acea
	// [ExampleRequestIDCanonicalFormat] 10da441c-3870-4f06-a78c-4dfef1c9acea

}

func printKSUIDInspect(id ksuid.KSUID) {
	const inspectFormat = `
REPRESENTATION:
  String: %v
     Raw: %v
COMPONENTS:
       Time: %v
  Timestamp: %v
    Payload: %v
`
	fmt.Printf(inspectFormat,
		id.String(),
		strings.ToUpper(hex.EncodeToString(id.Bytes())),
		id.Time().UTC().String(),
		id.Timestamp(),
		strings.ToUpper(hex.EncodeToString(id.Payload())),
	)
}

func ExampleNewKSUID() {
	testKSUIDStr := "2FwgbLS72ILDWFEhMSFKCRJBN7M"
	testKSUID, _ := uuid.KSUIDParse(testKSUIDStr)
	printKSUIDInspect(testKSUID)

	// output:
	// REPRESENTATION:
	//   String: 2FwgbLS72ILDWFEhMSFKCRJBN7M
	//      Raw: 0FD1D076606A0AB5DC14C70CD26FD2B6ED2A2D9C
	// COMPONENTS:
	//        Time: 2022-10-10 13:30:30 +0000 UTC
	//   Timestamp: 265408630
	//     Payload: 606A0AB5DC14C70CD26FD2B6ED2A2D9C

}

func printXIDInspect(id xid.ID) {
	const inspectFormat = `
REPRESENTATION:
  String: %v
     Raw: %v
COMPONENTS:
       Time: %v
    Counter: %v
    Machine: %v
        Pid: %v
`
	fmt.Printf(inspectFormat,
		id.String(),
		strings.ToUpper(hex.EncodeToString(id.Bytes())),
		id.Time().UTC().String(),
		id.Counter(),
		strings.ToUpper(hex.EncodeToString(id.Machine())),
		id.Pid(),
	)
}

func ExampleNewXID() {
	testXIDStr := "cd1qnegnhc7lcditguhg"
	testXID, _ := uuid.XIDFromString(testXIDStr)
	printXIDInspect(testXID)

	// output:
	// REPRESENTATION:
	//   String: cd1qnegnhc7lcditguhg
	//      Raw: 6343ABBA178B0F56365D87A3
	// COMPONENTS:
	//        Time: 2022-10-10 05:20:58 +0000 UTC
	//     Counter: 6129571
	//     Machine: 178B0F
	//         Pid: 22070

}
