// Package datetime offers convenience helpers over the time package for common
// ad-tech / finance operations: timezone-aware formatting, day/month
// boundaries, deltas, ranges, and week boundaries.
//
// # Week first day is locale-dependent
//
// The package deliberately does NOT pick a default week-start day. Pass it
// explicitly via the firstDay parameter, choosing per business/region:
//   - ISO 8601, China, most of Europe, Australia → time.Monday
//   - US, Canada, the ad-tech / media industry (e.g. ad-platform reports) → time.Sunday
//   - MENA (e.g. Egypt, Saudi, UAE) → time.Saturday
//
// FirstDateTimeOfISOWeek / LastDateTimeOfISOWeek are the ISO-aligned conveniences,
// matching time.Time.ISOWeek (which numbers weeks Monday-first). The
// authoritative territory→firstDay map is Unicode CLDR weekData; that mapping is
// a domain concern and is NOT embedded here (it ages, and locale policy belongs
// to the application, not a primitive package).
package datetime

import (
	"time"
)

// NowTime returns now time.
func NowTime() time.Time {
	return time.Now()
}

// NowTimeInLocation returns now time with the given location.
//
//	time format: ISO8601:2004 2004-05-03T17:30:08+08:00
//	go   format: 2006-01-02T15:04:05+00:00
func NowTimeInLocation(loc *time.Location) time.Time {
	if loc == nil {
		loc = time.Local
	}
	return time.Now().In(loc)
}

// NowUnix returns now as a Unix time, the number of seconds elapsed since January 1, 1970 UTC.
func NowUnix() int64 {
	return time.Now().Unix()
}

// NowUnixMilli returns now as a Unix time, the number of milliseconds elapsed since January 1, 1970 UTC.
func NowUnixMilli() int64 {
	return time.Now().UnixMilli()
}

// NowUnixNano returns now as a Unix time, the number of nanoseconds elapsed since January 1, 1970 UTC.
func NowUnixNano() int64 {
	return time.Now().UnixNano()
}

// TimeStr2Unix parses the string time with the given layout and location, and
// returns the corresponding Unix time (seconds since 1970-01-01 UTC). It returns
// an error if the parse fails — check it: a zero time.Time would otherwise
// silently become unix -62135596800.
func TimeStr2Unix(layout, value string, loc *time.Location) (int64, error) {
	parseTime, err := ParseInLocation(layout, value, loc)
	if err != nil {
		return 0, err
	}
	return parseTime.Unix(), nil
}

// TimeStr2UnixMilli is the milliseconds variant of TimeStr2Unix.
func TimeStr2UnixMilli(layout, value string, loc *time.Location) (int64, error) {
	parseTime, err := ParseInLocation(layout, value, loc)
	if err != nil {
		return 0, err
	}
	return parseTime.UnixMilli(), nil
}

// UnixToDuration converts the seconds to the corresponding duration time.Duration.
func UnixToDuration(sec int64) time.Duration {
	return time.Duration(sec)
}

// UnixMilliToDuration converts the milliseconds to the corresponding duration time.Duration.
func UnixMilliToDuration(msec int64) time.Duration {
	return time.Duration(msec)
}

// DurationStrToDuration parses a duration string (e.g. "300ms", "1.5h") via
// time.ParseDuration and returns it. It is a thin named wrapper; new code may
// prefer time.ParseDuration directly.
func DurationStrToDuration(duration string) (time.Duration, error) {
	return time.ParseDuration(duration)
}

// DurationStrToUnix parses a duration string and returns its length in seconds.
func DurationStrToUnix(duration string) (float64, error) {
	d, err := time.ParseDuration(duration)
	if err != nil {
		return 0, err
	}
	return d.Seconds(), nil
}

// DurationToUnix converts the duration to the corresponding seconds.
func DurationToUnix(d time.Duration) float64 {
	return d.Seconds()
}

// Unix2TimeStr converts the Unix time, the number of seconds elapsed since January 1, 1970 UTC, to the string with the given layout.
func Unix2TimeStr(sec int64, layout string) string {
	return time.Unix(sec, 0).Format(layout)
}

// UnixMilli2TimeStr converts the Unix time, the number of milliseconds elapsed since January 1, 1970 UTC, to the string with the given layout.
func UnixMilli2TimeStr(msec int64, layout string) string {
	return time.UnixMilli(msec).Format(layout)
}

// StartTime returns the start time for the special day, ex: 2006-06-01T00:00:00.999+08:00.
func StartTime(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// StartTimeStr returns start time str for the special time.
func StartTimeStr(layout string, t time.Time) string {
	if len(layout) == 0 {
		layout = LayoutDateTime
	}
	return StartTime(t).Format(layout)
}

// EndTime returns the end time with the given time, ex: 2019-06-01T23:59:59.999+08:00.
func EndTime(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 1e9-1, t.Location())
}

// EndTimeStr returns end time str for the special time.
func EndTimeStr(layout string, t time.Time) string {
	if len(layout) == 0 {
		layout = LayoutDateTime
	}
	return EndTime(t).Format(layout)
}

// StartEndTimeStr returns the start and end time str with the given time.
func StartEndTimeStr(layout string, t time.Time) (start, end string) {
	if len(layout) == 0 {
		layout = LayoutDateTime
	}
	start = StartTime(t).Format(layout)
	end = EndTime(t).Format(layout)
	return
}

// FirstDateTimeOfMonth returns the start time of the first day in the same month as the given time.
func FirstDateTimeOfMonth(t time.Time) time.Time {
	t = t.AddDate(0, 0, -t.Day()+1)
	return StartTime(t)
}

// FirstDateTimeStrOfMonth returns the start time string for a given layout on the first day of the same month as the given time.
func FirstDateTimeStrOfMonth(layout string, t time.Time) string {
	if len(layout) == 0 {
		layout = DefaultLayoutDate
	}
	return FirstDateTimeOfMonth(t).Format(layout)
}

// LastDateTimeOfMonth returns the end time of the last day in the same month as the given time.
func LastDateTimeOfMonth(t time.Time) time.Time {
	t = t.AddDate(0, 0, -t.Day()+1)
	return EndTime(t).AddDate(0, 1, -1)
}

// LastDateTimeStrOfMonth returns the end time string for a given layout on the last day of the same month as the given time.
func LastDateTimeStrOfMonth(layout string, t time.Time) string {
	if len(layout) == 0 {
		layout = DefaultLayoutDate
	}
	return LastDateTimeOfMonth(t).Format(layout)
}

// FirstDateTimeOfWeek returns the start time (00:00:00) of the day that begins
// the week containing t, where a week starts on firstDay.
//
// firstDay uses Go's time.Weekday values (time.Sunday=0 … time.Saturday=6). The
// package intentionally does NOT choose a locale default — the caller picks per
// business/region (see the package doc: ISO/Europe/China → time.Monday; US &
// ad-tech/media → time.Sunday; MENA → time.Saturday). FirstDateTimeOfISOWeek is
// the ISO-aligned convenience.
//
// t's own Location is honored: t.Weekday() already reflects t.In(loc), so pass
// a location-aware time and the boundary lands in that timezone.
func FirstDateTimeOfWeek(t time.Time, firstDay time.Weekday) time.Time {
	offset := -((int(t.Weekday()) - int(firstDay) + 7) % 7)
	return StartTime(t.AddDate(0, 0, offset))
}

// FirstDateTimeStrOfWeek is the formatted variant of FirstDateTimeOfWeek.
// layout defaults to DefaultLayoutDate when empty.
func FirstDateTimeStrOfWeek(layout string, t time.Time, firstDay time.Weekday) string {
	if len(layout) == 0 {
		layout = DefaultLayoutDate
	}
	return FirstDateTimeOfWeek(t, firstDay).Format(layout)
}

// LastDateTimeOfWeek returns the end time (23:59:59.999999999) of the day that
// ends the week containing t (i.e. firstDay + 6).
func LastDateTimeOfWeek(t time.Time, firstDay time.Weekday) time.Time {
	return EndTime(FirstDateTimeOfWeek(t, firstDay).AddDate(0, 0, 6))
}

// LastDateTimeStrOfWeek is the formatted variant of LastDateTimeOfWeek.
// layout defaults to DefaultLayoutDate when empty.
func LastDateTimeStrOfWeek(layout string, t time.Time, firstDay time.Weekday) string {
	if len(layout) == 0 {
		layout = DefaultLayoutDate
	}
	return LastDateTimeOfWeek(t, firstDay).Format(layout)
}

// FirstDateTimeOfISOWeek returns the Monday starting the ISO 8601 week containing
// t. Aligns with time.Time.ISOWeek, which numbers weeks Monday-first.
func FirstDateTimeOfISOWeek(t time.Time) time.Time {
	return FirstDateTimeOfWeek(t, time.Monday)
}

// LastDateTimeOfISOWeek returns the Sunday ending the ISO 8601 week containing t.
func LastDateTimeOfISOWeek(t time.Time) time.Time {
	return LastDateTimeOfWeek(t, time.Monday)
}

// DeltaDateDays returns the signed integer day count from start to end, where
// each side is snapped to its midnight (00:00:00) boundary. The result is
// symmetric: swapping start and end flips only the sign, so forward and
// backward spans of the same pair agree in magnitude.
//
//	start: 2022-02-28 10:00:00
//	end:   2022-03-02 09:00:00
//	delta date day: 2
func DeltaDateDays(start, end time.Time) int {
	s := StartTime(start)
	e := StartTime(end)
	// Re-create each snapped calendar date as a UTC midnight and diff those.
	// Diffing the local midnights directly is DST-sensitive: a spring-forward
	// day is only 23h wall-clock, so two consecutive dates across it subtract
	// to 23h and int(23/24) wrongly yields 0. Re-expressed in UTC the gap is
	// exactly 24h per calendar day (UTC has no DST), so the count is exact and
	// still symmetric (end<start yields a negative day count).
	sy, sm, sd := s.Date()
	ey, em, ed := e.Date()
	sUTC := time.Date(sy, sm, sd, 0, 0, 0, 0, time.UTC)
	eUTC := time.Date(ey, em, ed, 0, 0, 0, 0, time.UTC)
	return int(eUTC.Sub(sUTC) / (24 * time.Hour))
}

// DeltaDays returns the days between the end and start.
func DeltaDays(start, end time.Time) float64 {
	return end.Sub(start).Hours() / 24.0
}

// DeltaHours returns the hours between the end and start.
func DeltaHours(start, end time.Time) float64 {
	return end.Sub(start).Hours()
}

// DeltaMinutes returns the minutes between the end and start.
func DeltaMinutes(start, end time.Time) float64 {
	return end.Sub(start).Minutes()
}

// DeltaSeconds returns the seconds between the end and start.
func DeltaSeconds(start, end time.Time) float64 {
	return end.Sub(start).Seconds()
}

// AddDuration returns the time t+d.
func AddDuration(d time.Duration, t time.Time) time.Time {
	return t.Add(d)
}

// AddDurationStr returns t plus the parsed duration string. It returns an error
// if the duration string is invalid (t is returned unchanged alongside it).
//
//	Valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h".
func AddDurationStr(duration string, t time.Time) (time.Time, error) {
	d, err := time.ParseDuration(duration)
	if err != nil {
		return t, err
	}
	return t.Add(d), nil
}

// AddDays returns the time corresponding to adding the given number of days to t.
func AddDays(days int, t time.Time) time.Time {
	return t.AddDate(0, 0, days)
}

// AddDate returns the time corresponding to adding the given number of years, months, and days to t.
func AddDate(years, months, days int, t time.Time) time.Time {
	return t.AddDate(years, months, days)
}

// maxRangeEntries bounds RangeTime/RangeDateStr output to resist untrusted
// span/interval inputs (DoS via a huge allocation). 10000 entries is plenty
// for any sane date/time range.
const maxRangeEntries = 10000

// RangeTime returns the range time, between the start and end, with the given interval duration.
// It returns nil if interval <= 0 (would divide by zero) or the implied entry
// count exceeds maxRangeEntries (would allocate unboundedly).
func RangeTime(start, end time.Time, interval time.Duration) []time.Time {
	if interval <= 0 {
		return nil
	}
	if start.After(end) {
		start, end = end, start
	}

	delta := int(end.Sub(start)/interval) + 1
	if delta <= 0 || delta > maxRangeEntries {
		return nil
	}
	if delta == 1 {
		return []time.Time{start}
	}

	ret := make([]time.Time, delta)
	for i := range delta {
		ret[i] = start.Add(interval * time.Duration(i))
	}
	return ret
}

// RangeDateStr returns the range time string, between the start and end, with the given layout.
func RangeDateStr(start, end time.Time, layout string) []string {
	if start.After(end) {
		start, end = end, start
	}

	if len(layout) == 0 {
		layout = LayoutDateISO8601
	}
	delta := int(EndTime(end).Sub(StartTime(start)).Hours()/24.0) + 1
	if delta <= 0 || delta > maxRangeEntries {
		return nil
	}
	if delta == 1 {
		return []string{start.Format(layout)}
	}

	ret := make([]string, delta)
	for i := range delta {
		ret[i] = start.AddDate(0, 0, i).Format(layout)
	}
	return ret
}
