// Package datetime constants: ISO 8601:2004 layout formats.
// For the reference time semantics see src/time/format.go.
package datetime

// Layout format constants without a zone component, using Go's reference
// time (2006-01-02 15:04:05) and ISO 8601:2004 (see src/time/format.go).
const (
	// LayoutTime is the 24-hour clock layout, e.g. 15:04:05.
	LayoutTime = "15:04:05" // 8bit
	// LayoutTimeShort is the compact time layout, e.g. 150405.
	LayoutTimeShort = "150405" // 6bit
	// LayoutDateISO8601 is the ISO 8601 calendar-date layout, e.g. 2006-01-02.
	LayoutDateISO8601 = "2006-01-02" // 10bit
	// LayoutDateISO8601Short is the compact calendar-date layout, e.g. 20060102.
	LayoutDateISO8601Short = "20060102" //  8bit
	// LayoutDateTime is the date-and-time layout, e.g. 2006-01-02 15:04:05.
	LayoutDateTime = "2006-01-02 15:04:05" // 19bit
	// LayoutDateTimeLong is the date-and-time layout with milliseconds,
	// e.g. 2006-01-02 15:04:05.000.
	LayoutDateTimeLong = "2006-01-02 15:04:05.000" // 23bit
	// LayoutDateTimeShort is the compact date-and-time layout, e.g. 20060102150405.
	LayoutDateTimeShort = "20060102150405" // 14bit

	// DefaultLayoutTime is the package default for time-only formatting.
	DefaultLayoutTime = LayoutTime
	// DefaultLayoutDate is the package default for date-only formatting.
	DefaultLayoutDate = LayoutDateISO8601
	// DefaultLayoutDateTime is the package default for date-and-time formatting.
	DefaultLayoutDateTime = LayoutDateTime
	// DefaultLayoutDateTimeMsec is the package default for date-and-time
	// formatting with milliseconds.
	DefaultLayoutDateTimeMsec = LayoutDateTimeLong
)

// Layout format templates that carry a zone offset placeholder (%v). They are
// intended for use with fmt.Sprintf or LayoutWithFormatAndZoneOffset, which
// substitute the placeholder with the desired UTC offset such as +08, +0800,
// or +08:00.
const (
	// FormatLayoutTimeISO8601WithZone is a time layout template whose zone
	// placeholder expands to +xx:00, e.g. 15:04:05+08:00.
	FormatLayoutTimeISO8601WithZone = "15:04:05%v:00" // 14bit zone +xx:00
	// FormatLayoutTimeISO8601WithZoneMid is a time layout template whose zone
	// placeholder expands to +xx00, e.g. 150405+0800.
	FormatLayoutTimeISO8601WithZoneMid = "150405%v00" // 11bit zone +xx00
	// FormatLayoutTimeISO8601WithZoneShort is a time layout template whose zone
	// placeholder expands to +xx, e.g. 150405+08.
	FormatLayoutTimeISO8601WithZoneShort = "150405%v" //  9bit zone +xx
	// FormatLayoutDateTimeISO8601WithZone is a datetime layout template whose
	// zone placeholder expands to +xx:00, e.g. 2006-01-02T15:04:05+08:00.
	FormatLayoutDateTimeISO8601WithZone = "2006-01-02T15:04:05%v:00" // 25bit zone +xx:00
	// FormatLayoutDateTimeISO8601WithZoneMid is a datetime layout template whose
	// zone placeholder expands to +xx00, e.g. 2006-01-02T15:04:05+0800.
	FormatLayoutDateTimeISO8601WithZoneMid = "2006-01-02T15:04:05%v00" // 24bit zone +xx:00
	// FormatLayoutDateTimeISO8601WithZoneShort is a datetime layout template
	// whose zone placeholder expands to +xx, e.g. 2006-01-02T15:04:05+08.
	FormatLayoutDateTimeISO8601WithZoneShort = "2006-01-02T15:04:05%v" // 22bit zone +xx:00
	// FormatLayoutDateTimeISO8601ShortWithZone is a compact datetime layout
	// template whose zone placeholder expands to +xx00, e.g. 20060102T150405+0800.
	FormatLayoutDateTimeISO8601ShortWithZone = "20060102T150405%v:00" // 21bit zone +xx00
	// FormatLayoutDateTimeISO8601ShortWithZoneMid is a compact datetime layout
	// template whose zone placeholder expands to +xx00, e.g. 20060102T150405+0800.
	FormatLayoutDateTimeISO8601ShortWithZoneMid = "20060102T150405%v00" // 20bit zone +xx00
	// FormatLayoutDateTimeISO8601ShortWithZoneShort is a compact datetime layout
	// template whose zone placeholder expands to +xx, e.g. 20060102T150405+08.
	FormatLayoutDateTimeISO8601ShortWithZoneShort = "20060102T150405%v" // 18bit zone +xx
)

// Layout formats with the UTC zone offset (+00:00, +0000, or +00).
const (
	// LayoutDateTimeISO8601Zone is the full ISO 8601 datetime layout fixed to
	// UTC, e.g. 2006-01-02T15:04:05.000+00:00.
	LayoutDateTimeISO8601Zone = "2006-01-02T15:04:05.000+00:00" // 29bit zone +00:00
)

// China (UTC+08:00) time and datetime layouts, ISO 8601:2004. The zone renders
// as +08:00, +0800, or +08 depending on the variant.
const (
	// LayoutTimeISO8601ZoneP8 is the China-zone time layout with milliseconds,
	// e.g. 15:04:05.000+08:00.
	LayoutTimeISO8601ZoneP8 = "15:04:05.000+08:00" // 14bit zone +08:00
	// LayoutDateTimeISO8601ZoneP8 is the China-zone datetime layout with
	// milliseconds, e.g. 2006-01-02T15:04:05.000+08:00.
	LayoutDateTimeISO8601ZoneP8 = "2006-01-02T15:04:05.000+08:00" // 25bit zone +08:00
	// LayoutDateTimeISO8601ZoneP8Mid is the China-zone datetime layout with
	// milliseconds and a compact offset, e.g. 2006-01-02T15:04:05.000+0800.
	LayoutDateTimeISO8601ZoneP8Mid = "2006-01-02T15:04:05.000+0800" // 24bit zone +08:00
)
