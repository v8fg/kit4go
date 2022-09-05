package datetime

import (
	"time"
)

func NowTime() time.Time {
	return time.Now()
}

// NowTimeInLocation get now time use the given location
// time format: ISO8601:2004 2004-05-03T17:30:08+08:00
// go format: 2006-01-02T15:04:05+00:00
func NowTimeInLocation(loc *time.Location) time.Time {
	if loc == nil {
		loc = time.Local
	}
	return time.Now().In(loc)
}

// NowUnix return unix second
func NowUnix() int64 {
	return time.Now().Unix()
}

func NowUnixMilli() int64 {
	return time.Now().UnixMilli()
}

func NowUnixNano() int64 {
	return time.Now().UnixNano()
}

func TimeStr2Unix(layout, value string, loc *time.Location) int64 {
	parseTime, _ := ParseInLocation(layout, value, loc)
	return parseTime.Unix()
}

func TimeStr2UnixMilli(layout, value string, loc *time.Location) int64 {
	parseTime, _ := ParseInLocation(layout, value, loc)
	return parseTime.UnixMilli()
}

func Unix2TimeStr(sec int64, layout string) string {
	return time.Unix(sec, 0).Format(layout)
}

func UnixMilli2TimeStr(msec int64, layout string) string {
	return time.UnixMilli(msec).Format(layout)
}

// StartTime get the start time for the special day, ex: 2006-06-01T00:00:00.999+08:00
func StartTime(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// StartTimeStr get start time str for the special time
func StartTimeStr(layout string, t time.Time) string {
	if len(layout) == 0 {
		layout = LayoutDateTime
	}
	return StartTime(t).Format(layout)
}

// EndTime get the end time for the special day, ex: 2019-06-01T23:59:59.999+08:00
func EndTime(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 1e9-1, t.Location())
}

// EndTimeStr get end time str for the special time
func EndTimeStr(layout string, t time.Time) string {
	if len(layout) == 0 {
		layout = LayoutDateTime
	}
	return EndTime(t).Format(layout)
}

// StartEndTimeStr get start and end time str for the special day
func StartEndTimeStr(layout string, t time.Time) (start, end string) {
	if len(layout) == 0 {
		layout = LayoutDateTime
	}
	start = StartTime(t).Format(layout)
	end = EndTime(t).Format(layout)
	return
}

func FirstDateTimeOfMonth(t time.Time) time.Time {
	t = t.AddDate(0, 0, -t.Day()+1)
	return StartTime(t)
}

func FirstDateTimeStrOfMonth(layout string, t time.Time) string {
	if len(layout) == 0 {
		layout = DefaultLayoutDate
	}
	return FirstDateTimeOfMonth(t).Format(layout)
}

func LastDateTimeOfMonth(t time.Time) time.Time {
	t = t.AddDate(0, 0, -t.Day()+1)
	return EndTime(t).AddDate(0, 1, -1)
}

func LastDateTimeStrOfMonth(layout string, t time.Time) string {
	if len(layout) == 0 {
		layout = DefaultLayoutDate
	}
	return LastDateTimeOfMonth(t).Format(layout)
}

func FirstDateTimeOfWeek(t time.Time) time.Time {
	// firstDay Monday
	return StartTime(t.AddDate(0, 0, int(-t.Weekday())+1))
}

func FirstDateTimeStrOfWeek(layout string, t time.Time) string {
	if len(layout) == 0 {
		layout = DefaultLayoutDate
	}
	return FirstDateTimeOfWeek(t).Format(layout)
}

func LastDateTimeOfWeek(t time.Time) time.Time {
	// firstDay Monday
	return EndTime(t.AddDate(0, 0, int(time.Saturday+1-t.Weekday())%7))
}

func LastDateTimeStrOfWeek(layout string, t time.Time) string {
	if len(layout) == 0 {
		layout = DefaultLayoutDate
	}
	return LastDateTimeOfWeek(t).Format(layout)
}

// DeltaDateDay return the real days between the end and start.
//
// start: 2022-02-28 10:00:00, end: 2020-03-01 09:00:00, delta date day: 2
func DeltaDateDay(start, end time.Time) int {
	return int(EndTime(end).Sub(StartTime(start)).Hours()/24.0) + 1
}

// DeltaDays return the days for end - start
func DeltaDays(start, end time.Time) float64 {
	return end.Sub(start).Hours() / 24.0
}

// DeltaHours return the hours for end - start
func DeltaHours(start, end time.Time) float64 {
	return end.Sub(start).Hours()
}

// DeltaMinutes return the minutes for end - start
func DeltaMinutes(start, end time.Time) float64 {
	return end.Sub(start).Minutes()
}

// DeltaSeconds return the seconds for end - start
func DeltaSeconds(start, end time.Time) float64 {
	return end.Sub(start).Seconds()
}

// AddDuration add duration for the special time
func AddDuration(d time.Duration, t time.Time) time.Time {
	return t.Add(d)
}

// AddDurationStr add duration str for the special time
// Valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h".
func AddDurationStr(duration string, t time.Time) time.Time {
	d, _ := time.ParseDuration(duration)
	return t.Add(d)
}

// AddDays add days for the special time
func AddDays(days int, t time.Time) time.Time {
	return t.AddDate(0, 0, days)
}

// AddDate add date for the special time
func AddDate(years, months, days int, t time.Time) time.Time {
	return t.AddDate(years, months, days)
}

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
