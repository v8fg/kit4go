package datetime

import (
	"fmt"
	"strings"
	"time"
)

// LayoutWithFormatAndZoneOffset returns the layout string with the given format string and zone offset.
//
// The default format string 2006-01-02T15:04:05%v:00.
func LayoutWithFormatAndZoneOffset(format string, zoneOffset int) string {
	if len(format) == 0 {
		format = FormatLayoutDateTimeISO8601WithZone
	}
	if !strings.Contains(format, "%") {
		return format
	}

	var zone string
	zoneOffset = zoneOffset % 24
	if zoneOffset < 0 {
		zone = fmt.Sprintf("-%02d", -zoneOffset)
	} else {
		zone = fmt.Sprintf("+%02d", zoneOffset)
	}
	return fmt.Sprintf(format, zone)
}

// Format returns now time string, with the given layout, default 2006-01-02 15:04:05
func Format(layout string, t time.Time) string {
	if len(layout) == 0 {
		layout = DefaultLayoutDateTime
	}
	return t.Format(layout)
}

// FormatNowDate returns now time string, with layout: 2006-01-02
func FormatNowDate() string {
	return time.Now().Format(DefaultLayoutDate)
}

// FormatNowTime returns now time string, with layout: 15:04:05
func FormatNowTime() string {
	return time.Now().Format(DefaultLayoutTime)
}

// FormatNowDatetime returns now time string, with layout: 2006-01-02 15:04:05
func FormatNowDatetime() string {
	return time.Now().Format(DefaultLayoutDateTime)
}

// FormatNowWithLayout return the now time with the given layout.
func FormatNowWithLayout(layout string) string {
	return time.Now().Format(layout)
}
