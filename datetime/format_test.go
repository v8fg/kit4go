package datetime_test

import (
	"testing"
	"time"

	"github.com/v8fg/kit4go/datetime"
)

func TestFormat(t *testing.T) {
	type args struct {
		layout string
		t      time.Time
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{name: "", args: args{layout: "",
			t: time.Date(2020, 2, 1, 0, 0, 5, 0, time.Local),
		}, want: "2020-02-01 00:00:05"},
		{name: "", args: args{layout: datetime.DefaultLayoutDate,
			t: time.Date(2020, 2, 1, 0, 0, 5, 0, time.Local),
		}, want: "2020-02-01"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := datetime.Format(tt.args.layout, tt.args.t); got != tt.want {
				t.Errorf("Format() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFormatNowDate(t *testing.T) {
	nowDate := datetime.FormatNowDate()
	tests := []struct {
		name string
		want string
	}{
		{name: "", want: nowDate},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nowDate; got != tt.want {
				t.Errorf("FormatNowDate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFormatNowDatetime(t *testing.T) {
	nowDateTime := datetime.FormatNowDatetime()
	tests := []struct {
		name string
		want string
	}{
		{name: "", want: nowDateTime},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nowDateTime; got != tt.want {
				t.Errorf("FormatNowDatetime() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFormatNowTime(t *testing.T) {
	nowTime := datetime.FormatNowTime()
	tests := []struct {
		name string
		want string
	}{
		{name: "", want: nowTime},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nowTime; got != tt.want {
				t.Errorf("FormatNowTime() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFormatNowWithLayout(t *testing.T) {
	layout := datetime.DefaultLayoutDateTime
	now := datetime.FormatNowWithLayout(layout)
	type args struct {
		layout string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{name: "", args: args{layout: layout}, want: now},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := now; got != tt.want {
				t.Errorf("FormatNowWithLayout() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLayoutWithFormatAndZoneOffset(t *testing.T) {
	type args struct {
		format     string
		zoneOffset int
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{name: "", args: args{format: "", zoneOffset: 8}, want: "2006-01-02T15:04:05+08:00"},
		{name: "", args: args{format: "", zoneOffset: -8}, want: "2006-01-02T15:04:05-08:00"},
		{name: "LayoutTime", args: args{format: datetime.LayoutTime, zoneOffset: 8}, want: "15:04:05"},
		{name: "LayoutTimeShort", args: args{format: datetime.LayoutTimeShort, zoneOffset: 8}, want: "150405"},
		{name: "LayoutDateISO8601", args: args{format: datetime.LayoutDateISO8601, zoneOffset: 8}, want: "2006-01-02"},
		{name: "LayoutDateISO8601Short", args: args{format: datetime.LayoutDateISO8601Short, zoneOffset: 8}, want: "20060102"},
		{name: "LayoutDateTime", args: args{format: datetime.LayoutDateTime, zoneOffset: 8}, want: "2006-01-02 15:04:05"},
		{name: "LayoutDateTimeLong", args: args{format: datetime.LayoutDateTimeLong, zoneOffset: 8}, want: "2006-01-02 15:04:05.000"},
		{name: "LayoutDateTimeShort", args: args{format: datetime.LayoutDateTimeShort, zoneOffset: 8}, want: "20060102150405"},
		{name: "FormatLayoutTimeISO8601WithZone", args: args{format: datetime.FormatLayoutTimeISO8601WithZone, zoneOffset: 8}, want: "15:04:05+08:00"},
		{name: "FormatLayoutTimeISO8601WithZoneMid", args: args{format: datetime.FormatLayoutTimeISO8601WithZoneMid, zoneOffset: 8}, want: "150405+0800"},
		{name: "FormatLayoutTimeISO8601WithZoneShort", args: args{format: datetime.FormatLayoutTimeISO8601WithZoneShort, zoneOffset: 8}, want: "150405+08"},
		{name: "FormatLayoutDateTimeISO8601WithZone", args: args{format: datetime.FormatLayoutDateTimeISO8601WithZone, zoneOffset: 8}, want: "2006-01-02T15:04:05+08:00"},
		{name: "FormatLayoutDateTimeISO8601WithZoneMid", args: args{format: datetime.FormatLayoutDateTimeISO8601WithZoneMid, zoneOffset: 8}, want: "2006-01-02T15:04:05+0800"},
		{name: "FormatLayoutDateTimeISO8601WithZoneShort", args: args{format: datetime.FormatLayoutDateTimeISO8601WithZoneShort, zoneOffset: 8}, want: "2006-01-02T15:04:05+08"},
		{name: "FormatLayoutDateTimeISO8601ShortWithZone", args: args{format: datetime.FormatLayoutDateTimeISO8601ShortWithZone, zoneOffset: 8}, want: "20060102T150405+08:00"},
		{name: "FormatLayoutDateTimeISO8601ShortWithZoneMid", args: args{format: datetime.FormatLayoutDateTimeISO8601ShortWithZoneMid, zoneOffset: 8}, want: "20060102T150405+0800"},
		{name: "FormatLayoutDateTimeISO8601ShortWithZoneShort", args: args{format: datetime.FormatLayoutDateTimeISO8601ShortWithZoneShort, zoneOffset: 8}, want: "20060102T150405+08"},
		{name: "DefaultLayoutDateTime", args: args{format: datetime.DefaultLayoutDateTime, zoneOffset: 8}, want: "2006-01-02 15:04:05"},
		{name: "LayoutDateTimeISO8601Zone", args: args{format: datetime.LayoutDateTimeISO8601Zone, zoneOffset: 8}, want: "2006-01-02T15:04:05.000+00:00"},
		{name: "LayoutDateTimeISO8601Zone", args: args{format: datetime.LayoutTimeISO8601ZoneP8, zoneOffset: 0}, want: "15:04:05.000+08:00"},
		{name: "LayoutDateTimeISO8601Zone", args: args{format: datetime.LayoutDateTimeISO8601ZoneP8, zoneOffset: 0}, want: "2006-01-02T15:04:05.000+08:00"},
		{name: "LayoutDateTimeISO8601Zone", args: args{format: datetime.LayoutDateTimeISO8601ZoneP8Mid, zoneOffset: 0}, want: "2006-01-02T15:04:05.000+0800"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := datetime.LayoutWithFormatAndZoneOffset(tt.args.format, tt.args.zoneOffset); got != tt.want {
				t.Errorf("LayoutWithFormatAndZoneOffset() = %v, want %v", got, tt.want)
			}
		})
	}
}
