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

// TimeStr2Unix parses the string time with the given layout and location, and returns the corresponding Unix time,
// the number of seconds elapsed since January 1, 1970 UTC.
func TimeStr2Unix(layout, value string, loc *time.Location) int64 {
	parseTime, _ := ParseInLocation(layout, value, loc)
	return parseTime.Unix()
}

// TimeStr2UnixMilli parses the string time with the given layout and location, and returns the corresponding Unix time,
// the number of milliseconds elapsed since January 1, 1970 UTC.
func TimeStr2UnixMilli(layout, value string, loc *time.Location) int64 {
	parseTime, _ := ParseInLocation(layout, value, loc)
	return parseTime.UnixMilli()
}

// UnixToDuration converts the seconds to the corresponding duration time.Duration.
func UnixToDuration(sec int64) time.Duration {
	return time.Duration(sec)
}

// UnixMilliToDuration converts the milliseconds to the corresponding duration time.Duration.
func UnixMilliToDuration(msec int64) time.Duration {
	return time.Duration(msec)
}

// DurationStrToDuration converts the duration string to the corresponding duration time.Duration.
func DurationStrToDuration(duration string) time.Duration {
	d, err := time.ParseDuration(duration)
	if err != nil {
		return 0
	}
	return d
}

// DurationStrToUnix converts the duration string to the corresponding seconds.
func DurationStrToUnix(duration string) float64 {
	d, err := time.ParseDuration(duration)
	if err != nil {
		return 0
	}
	return d.Seconds()
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

// FirstDateTimeOfWeek returns the start time of the first day in the same week as the given time, first day shall Monday.
func FirstDateTimeOfWeek(t time.Time) time.Time {
	// firstDay Monday
	return StartTime(t.AddDate(0, 0, int(-t.Weekday())+1))
}

// FirstDateTimeStrOfWeek returns the start time string for a given layout on the first day of the same week as the given time, first day shall Monday.
func FirstDateTimeStrOfWeek(layout string, t time.Time) string {
	if len(layout) == 0 {
		layout = DefaultLayoutDate
	}
	return FirstDateTimeOfWeek(t).Format(layout)
}

// LastDateTimeOfWeek returns the end time of the last day in the same week as the given time, first day shall Monday.
func LastDateTimeOfWeek(t time.Time) time.Time {
	// firstDay Monday
	return EndTime(t.AddDate(0, 0, int(time.Saturday+1-t.Weekday())%7))
}

// LastDateTimeStrOfWeek returns the end time string for a given layout on the last day of the same week as the given time, first day shall Monday.
func LastDateTimeStrOfWeek(layout string, t time.Time) string {
	if len(layout) == 0 {
		layout = DefaultLayoutDate
	}
	return LastDateTimeOfWeek(t).Format(layout)
}

// DeltaDateDays returns the real integer days between the end and start.
//
//	start: 2022-02-28 10:00:00
//	end: 2020-03-01 09:00:00
//	delta date day: 2
func DeltaDateDays(start, end time.Time) int {
	return int(EndTime(end).Sub(StartTime(start)).Hours()/24.0) + 1
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

// AddDurationStr returns the time t+d, d shall the valid duration string format.
//
//	Valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h".
func AddDurationStr(duration string, t time.Time) time.Time {
	d, _ := time.ParseDuration(duration)
	return t.Add(d)
}

// AddDays returns the time corresponding to adding the given number of days to t.
func AddDays(days int, t time.Time) time.Time {
	return t.AddDate(0, 0, days)
}

// AddDate returns the time corresponding to adding the given number of years, months, and days to t.
func AddDate(years, months, days int, t time.Time) time.Time {
	return t.AddDate(years, months, days)
}

// RangeTime returns the range time, between the start and end, with the given interval duration.
func RangeTime(start, end time.Time, interval time.Duration) []time.Time {
	if start.After(end) {
		start, end = end, start
	}

	delta := int(end.Sub(start)/interval) + 1
	if delta == 1 {
		return []time.Time{start}
	}

	ret := make([]time.Time, delta)
	for i := 0; i < delta; i++ {
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
	if delta == 1 {
		return []string{start.Format(layout)}
	}

	ret := make([]string, delta)
	for i := 0; i < delta; i++ {
		ret[i] = start.AddDate(0, 0, i).Format(layout)
	}
	return ret
}
