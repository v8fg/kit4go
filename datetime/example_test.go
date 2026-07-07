package datetime_test

import (
	"fmt"
	"time"

	"github.com/v8fg/kit4go/datetime"
)

// ExampleDurationStrToDuration demonstrates parsing a Go duration string into a
// time.Duration.
func ExampleDurationStrToDuration() {
	d, _ := datetime.DurationStrToDuration("500ms")
	fmt.Println(d)
	// Output: 500ms
}

// Example_monthAndWeekBoundaries shows the package's headline capability:
// timezone-aware month and week boundaries. The week first day is passed
// explicitly — pick per region/business: time.Monday for ISO/Europe/China,
// time.Sunday for the US and ad-tech/media, time.Saturday for MENA.
func Example_monthAndWeekBoundaries() {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	t := time.Date(2024, 2, 15, 10, 30, 0, 0, loc) // a Thursday

	// Month boundaries (2024-02 is a leap month → 29 days).
	fmt.Println("first day of month:", datetime.FirstDateTimeStrOfMonth(datetime.LayoutDateISO8601, t))
	fmt.Println("last day of month: ", datetime.LastDateTimeStrOfMonth(datetime.LayoutDateISO8601, t))

	// ISO week (Monday-first) — matches time.Time.ISOWeek numbering.
	fmt.Println("iso week:          ", datetime.FirstDateTimeOfISOWeek(t).Format(datetime.LayoutDateISO8601),
		"to", datetime.LastDateTimeOfISOWeek(t).Format(datetime.LayoutDateISO8601))

	// Same date, ad-tech/media week (Sunday-first) starts one day earlier.
	fmt.Println("sunday-first week: ", datetime.FirstDateTimeOfWeek(t, time.Sunday).Format(datetime.LayoutDateISO8601))

	// Output:
	// first day of month: 2024-02-01
	// last day of month:  2024-02-29
	// iso week:           2024-02-12 to 2024-02-18
	// sunday-first week:  2024-02-11
}

// Example_parseFormatRange demonstrates parsing a string to Unix time,
// formatting it back, and generating an inclusive daily range — a common
// reporting pipeline. Locations are honored end-to-end; here UTC is fixed so
// the output is deterministic.
func Example_parseFormatRange() {
	// String → Unix seconds, in a known location.
	unixSec, _ := datetime.TimeStr2Unix(datetime.LayoutDateISO8601, "2024-01-01", time.UTC)
	fmt.Println("unix:", unixSec)
	fmt.Println("str: ", datetime.Unix2TimeStr(unixSec, datetime.LayoutDateISO8601))

	// Inclusive date range spanning a month boundary (leap-year Feb).
	start := time.Date(2024, 2, 28, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)
	fmt.Println("range:", datetime.RangeDateStr(start, end, datetime.LayoutDateISO8601))

	// Output:
	// unix: 1704067200
	// str:  2024-01-01
	// range: [2024-02-28 2024-02-29 2024-03-01 2024-03-02]
}
