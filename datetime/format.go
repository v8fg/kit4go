package datetime

import (
	"fmt"
	"strings"
	"time"
)

// LayoutWithFormatAndZoneOffset default 2006-01-02T15:04:05%v:00
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

// Format default layout: 2006-01-02 15:04:05
func Format(layout string, t time.Time) string {
	if len(layout) == 0 {
		layout = DefaultLayoutDateTime
	}
	return t.Format(layout)
}

// FormatNowDate default layout: 2006-01-02
func FormatNowDate() string {
	return time.Now().Format(DefaultLayoutDate)
}

// FormatNowTime default layout: 15:04:05
func FormatNowTime() string {
	return time.Now().Format(DefaultLayoutTime)
}

// FormatNowDatetime default layout: 2006-01-02 15:04:05
func FormatNowDatetime() string {
	return time.Now().Format(DefaultLayoutDateTime)
}

func FormatNowWithLayout(layout string) string {
	return time.Now().Format(layout)
}
