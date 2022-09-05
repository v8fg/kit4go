/*Package datetime constants, layout format with ISO8601:2004
more info ref: src/time/format.go
*/

package datetime

// layout format without zone
const (
	LayoutTime             = "15:04:05"                // 8bit
	LayoutTimeShort        = "150405"                  // 6bit
	LayoutDateISO8601      = "2006-01-02"              // 10bit
	LayoutDateISO8601Short = "20060102"                //  8bit
	LayoutDateTime         = "2006-01-02 15:04:05"     // 19bit
	LayoutDateTimeLong     = "2006-01-02 15:04:05.000" // 23bit
	LayoutDateTimeShort    = "20060102150405"          // 14bit

	DefaultLayoutTime         = LayoutTime
	DefaultLayoutDate         = LayoutDateISO8601
	DefaultLayoutDateTime     = LayoutDateTime
	DefaultLayoutDateTimeMsec = LayoutDateTimeLong
)

// layout format with zone
const (
	FormatLayoutTimeISO8601WithZone               = "15:04:05%v:00"            // 14bit zone +xx:00
	FormatLayoutTimeISO8601WithZoneMid            = "150405%v00"               // 11bit zone +xx00
	FormatLayoutTimeISO8601WithZoneShort          = "150405%v"                 //  9bit zone +xx
	FormatLayoutDateTimeISO8601WithZone           = "2006-01-02T15:04:05%v:00" // 25bit zone +xx:00
	FormatLayoutDateTimeISO8601WithZoneMid        = "2006-01-02T15:04:05%v00"  // 24bit zone +xx:00
	FormatLayoutDateTimeISO8601WithZoneShort      = "2006-01-02T15:04:05%v"    // 22bit zone +xx:00
	FormatLayoutDateTimeISO8601ShortWithZone      = "20060102T150405%v:00"     // 21bit zone +xx00
	FormatLayoutDateTimeISO8601ShortWithZoneMid   = "20060102T150405%v00"      // 20bit zone +xx00
	FormatLayoutDateTimeISO8601ShortWithZoneShort = "20060102T150405%v"        // 18bit zone +xx
)

// LayoutDateTimeISO8601Zone
// layout format with zone UTC
// zone +00:00 | +0000 | +00
const (
	LayoutDateTimeISO8601Zone = "2006-01-02T15:04:05.000+00:00" // 29bit zone +00:00
)

// layout format with special zone 08
// zone +08:00 | +0800 | +08
// China time and datetime layout format with ISO8601:2004
const (
	LayoutTimeISO8601ZoneP8        = "15:04:05.000+08:00"            // 14bit zone +08:00
	LayoutDateTimeISO8601ZoneP8    = "2006-01-02T15:04:05.000+08:00" // 25bit zone +08:00
	LayoutDateTimeISO8601ZoneP8Mid = "2006-01-02T15:04:05.000+0800"  // 24bit zone +08:00
)
