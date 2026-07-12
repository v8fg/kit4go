package datetime_test

import (
	"testing"
	"time"

	"github.com/v8fg/kit4go/datetime"
)

// FuzzDeltaDateDaysAddDateRoundTrip encodes the calendar-day invariant: for a
// base time and an integer day count, DeltaDateDays(t, AddDate(0,0,days,t)) == days.
// AddDate uses time.AddDate (calendar), and DeltaDateDays re-expresses each date
// as UTC midnight before diffing (the R34 DST-immunity fix), so the diff is the
// exact calendar-day delta regardless of wall-clock or DST. Guards against
// DST/boundary regressions in the date math. E10 invariant-encoding fuzz target.
//
// (Go fuzzing has no time.Time parameter type, so the base instant is built from
// a bounded unix-seconds int64, pinned to UTC for a deterministic calendar date.)
func FuzzDeltaDateDaysAddDateRoundTrip(f *testing.F) {
	f.Add(int64(0), 0)
	f.Add(int64(0), 1)
	f.Add(int64(1700000000), 31)
	f.Add(int64(0), -10)
	f.Fuzz(func(t *testing.T, unixSec int64, days int) {
		// Bound the instant to ~50 years from the epoch (UTC) so AddDate with a
		// bounded day count cannot overflow.
		sec := unixSec % (50 * 365 * 86400)
		if sec < 0 {
			sec += 50 * 365 * 86400
		}
		ts := time.Unix(sec, 0).UTC()
		d := days % 10000
		got := datetime.DeltaDateDays(ts, datetime.AddDate(0, 0, d, ts))
		if got != d {
			t.Errorf("DeltaDateDays(%v, AddDate(0,0,%d)) = %d, want %d", ts, d, got, d)
		}
	})
}
