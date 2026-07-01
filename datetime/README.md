# datetime: time conversions

Conversions between unix timestamps, durations, and formatted strings, plus
start/end-of-period helpers. Pure standard library.

## Usage

### now

- `NowTime() time.Time`, `NowTimeInLocation(loc *time.Location) time.Time`.
- `NowUnix() int64` seconds.
- `NowUnixMilli() int64`, `NowUnixNano() int64`.

### string <-> unix

- `TimeStr2Unix(layout, value string, loc *time.Location) int64`.
- `TimeStr2UnixMilli(layout, value string, loc *time.Location) int64`.
- `Unix2TimeStr(sec int64, layout string) string`.
- `UnixMilli2TimeStr(msec int64, layout string) string`.

### duration

- `UnixToDuration(sec int64) time.Duration`.
- `UnixMilliToDuration(msec int64) time.Duration`.
- `DurationStrToDuration(duration string) time.Duration` parse "1h2m3s"-style.
- `DurationStrToUnix(duration string) float64`.
- `DurationToUnix(d time.Duration) float64`.

### period bounds

- `StartTime(t time.Time) time.Time` midnight of `t`'s day.
- `EndTime(t time.Time) time.Time` end of `t`'s day.
- `StartTimeStr(layout string, t time.Time) string`.
- `EndTimeStr(layout string, t time.Time) string`.
- `StartEndTimeStr(layout string, t time.Time) (start, end string)`.

## Example

```go
import "github.com/v8fg/kit4go/datetime"

datetime.NowUnixMilli()
datetime.Unix2TimeStr(now, "2006-01-02 15:04:05")
datetime.DurationStrToDuration("500ms")
```
