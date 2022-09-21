package datetime_test

import (
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/v8fg/kit4go/datetime"
)

func TestMain(m *testing.M) {
	// set the local to UTC, avoid the invalid parse.
	oldLocal := time.Local
	time.Local = time.UTC
	m.Run()

	defer func() {
		time.Local = oldLocal
		os.Exit(0)
	}()
}

func TestAddDate(t *testing.T) {
	type args struct {
		years  int
		months int
		days   int
		t      time.Time
	}
	tests := []struct {
		name string
		args args
		want time.Time
	}{
		{
			name: "", args: args{years: 0, months: 0, days: 1, t: time.Date(2022, 2, 28, 0, 0, 0, 0, time.UTC)},
			want: time.Date(2022, 2, 28, 0, 0, 0, 0, time.UTC).Add(time.Hour * 24),
		},
		{
			name: "", args: args{years: 0, months: 0, days: -1, t: time.Date(2022, 2, 28, 0, 0, 0, 0, time.UTC)},
			want: time.Date(2022, 2, 28, 0, 0, 0, 0, time.UTC).Add(-time.Hour * 24),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := datetime.AddDate(tt.args.years, tt.args.months, tt.args.days, tt.args.t); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("AddDate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddDays(t *testing.T) {
	type args struct {
		days int
		t    time.Time
	}
	tests := []struct {
		name string
		args args
		want time.Time
	}{
		{
			name: "", args: args{days: 1, t: time.Date(2022, 2, 28, 0, 0, 0, 0, time.UTC)},
			want: time.Date(2022, 2, 28, 0, 0, 0, 0, time.UTC).Add(time.Hour * 24),
		},
		{
			name: "", args: args{days: -1, t: time.Date(2022, 2, 28, 0, 0, 0, 0, time.UTC)},
			want: time.Date(2022, 2, 28, 0, 0, 0, 0, time.UTC).Add(-time.Hour * 24),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := datetime.AddDays(tt.args.days, tt.args.t); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("AddDays() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddDuration(t *testing.T) {
	type args struct {
		d time.Duration
		t time.Time
	}
	tests := []struct {
		name string
		args args
		want time.Time
	}{
		{
			name: "", args: args{d: time.Hour * 24, t: time.Date(2022, 2, 28, 0, 0, 0, 0, time.UTC)},
			want: time.Date(2022, 2, 28, 0, 0, 0, 0, time.UTC).Add(time.Hour * 24),
		},
		{
			name: "", args: args{d: -time.Hour * 24, t: time.Date(2022, 2, 28, 0, 0, 0, 0, time.UTC)},
			want: time.Date(2022, 2, 28, 0, 0, 0, 0, time.UTC).Add(-time.Hour * 24),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := datetime.AddDuration(tt.args.d, tt.args.t); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("AddDuration() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddDurationStr(t *testing.T) {
	type args struct {
		duration string
		t        time.Time
	}
	tests := []struct {
		name string
		args args
		want time.Time
	}{
		{
			name: "", args: args{duration: "48h", t: time.Date(2022, 2, 28, 0, 0, 0, 0, time.UTC)},
			want: time.Date(2022, 2, 28, 0, 0, 0, 0, time.UTC).Add(time.Hour * 48),
		},
		{
			name: "", args: args{duration: "24h", t: time.Date(2022, 2, 28, 0, 0, 0, 0, time.UTC)},
			want: time.Date(2022, 2, 28, 0, 0, 0, 0, time.UTC).Add(time.Hour * 24),
		},
		{
			name: "", args: args{duration: "-24h", t: time.Date(2022, 2, 28, 0, 0, 0, 0, time.UTC)},
			want: time.Date(2022, 2, 28, 0, 0, 0, 0, time.UTC).Add(-time.Hour * 24),
		},
		{
			name: "", args: args{duration: "-96h", t: time.Date(2022, 2, 28, 0, 0, 0, 0, time.UTC)},
			want: time.Date(2022, 2, 28, 0, 0, 0, 0, time.UTC).Add(-time.Hour * 96),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := datetime.AddDurationStr(tt.args.duration, tt.args.t); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("AddDurationStr() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDeltaDateDay(t *testing.T) {
	type args struct {
		start time.Time
		end   time.Time
	}
	tests := []struct {
		name string
		args args
		want int
	}{
		{
			name: "", args: args{
				start: time.Date(2022, 2, 28, 0, 0, 0, 0, time.UTC),
				end:   time.Date(2022, 2, 28, 1, 0, 0, 0, time.UTC),
			}, want: 1,
		},
		{
			name: "", args: args{
				start: time.Date(2022, 2, 28, 0, 0, 0, 0, time.UTC),
				end:   time.Date(2022, 3, 1, 23, 59, 59, 999999999, time.UTC),
			}, want: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := datetime.DeltaDateDays(tt.args.start, tt.args.end); got != tt.want {
				t.Errorf("DeltaDateDays() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDeltaDays(t *testing.T) {
	type args struct {
		start time.Time
		end   time.Time
	}
	tests := []struct {
		name string
		args args
		want float64
	}{
		{
			name: "", args: args{
				start: time.Date(2022, 2, 28, 0, 0, 0, 0, time.UTC),
				end:   time.Date(2022, 2, 28, 12, 0, 0, 0, time.UTC),
			}, want: 0.5,
		},
		{
			name: "", args: args{
				start: time.Date(2022, 2, 28, 23, 0, 0, 0, time.UTC),
				end:   time.Date(2022, 3, 1, 5, 0, 0, 0, time.UTC),
			}, want: 0.25,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := datetime.DeltaDays(tt.args.start, tt.args.end); got != tt.want {
				t.Errorf("DeltaDays() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDeltaHours(t *testing.T) {
	type args struct {
		start time.Time
		end   time.Time
	}
	tests := []struct {
		name string
		args args
		want float64
	}{
		{
			name: "", args: args{
				start: time.Date(2022, 2, 28, 0, 0, 0, 0, time.UTC),
				end:   time.Date(2022, 2, 28, 12, 0, 0, 0, time.UTC),
			}, want: 12,
		},
		{
			name: "", args: args{
				start: time.Date(2022, 2, 28, 23, 0, 0, 0, time.UTC),
				end:   time.Date(2022, 3, 1, 5, 0, 0, 0, time.UTC),
			}, want: 6,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := datetime.DeltaHours(tt.args.start, tt.args.end); got != tt.want {
				t.Errorf("DeltaHours() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDeltaMinutes(t *testing.T) {
	type args struct {
		start time.Time
		end   time.Time
	}
	tests := []struct {
		name string
		args args
		want float64
	}{
		{
			name: "", args: args{
				start: time.Date(2022, 2, 28, 0, 0, 0, 0, time.UTC),
				end:   time.Date(2022, 2, 28, 0, 12, 0, 0, time.UTC),
			}, want: 12,
		},
		{
			name: "", args: args{
				start: time.Date(2022, 3, 1, 0, 33, 0, 0, time.UTC),
				end:   time.Date(2022, 3, 1, 1, 9, 0, 0, time.UTC),
			}, want: 36,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := datetime.DeltaMinutes(tt.args.start, tt.args.end); got != tt.want {
				t.Errorf("DeltaMinutes() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDeltaSeconds(t *testing.T) {
	type args struct {
		start time.Time
		end   time.Time
	}
	tests := []struct {
		name string
		args args
		want float64
	}{
		{
			name: "", args: args{
				start: time.Date(2022, 2, 28, 0, 0, 5, 0, time.UTC),
				end:   time.Date(2022, 2, 28, 0, 1, 55, 0, time.UTC),
			}, want: 110,
		},
		{
			name: "", args: args{
				start: time.Date(2022, 3, 1, 0, 3, 0, 0, time.UTC),
				end:   time.Date(2022, 3, 1, 1, 3, 36, 0, time.UTC),
			}, want: 3636,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := datetime.DeltaSeconds(tt.args.start, tt.args.end); got != tt.want {
				t.Errorf("DeltaSeconds() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEndTime(t *testing.T) {
	type args struct {
		t time.Time
	}
	tests := []struct {
		name string
		args args
		want time.Time
	}{
		{
			name: "", args: args{
				t: time.Date(2022, 2, 28, 0, 0, 5, 0, time.UTC)},
			want: time.Date(2022, 2, 28, 23, 59, 59, 999999999, time.UTC),
		},
		{
			name: "", args: args{
				t: time.Date(2022, 3, 1, 23, 0, 5, 0, time.UTC)},
			want: time.Date(2022, 3, 1, 23, 59, 59, 999999999, time.UTC),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := datetime.EndTime(tt.args.t); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("EndTime() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEndTimeStr(t *testing.T) {
	type args struct {
		layout string
		t      time.Time
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "", args: args{
				t:      time.Date(2022, 2, 28, 0, 0, 5, 0, time.UTC),
				layout: ""},
			want: "2022-02-28 23:59:59",
		},
		{
			name: "", args: args{
				t:      time.Date(2022, 2, 28, 0, 0, 5, 0, time.UTC),
				layout: datetime.DefaultLayoutDateTime},
			want: "2022-02-28 23:59:59",
		},
		{
			name: "", args: args{
				t:      time.Date(2022, 3, 1, 23, 0, 5, 0, time.UTC),
				layout: datetime.DefaultLayoutDateTime},
			want: "2022-03-01 23:59:59",
		},
		{
			name: "", args: args{
				t:      time.Date(2022, 3, 1, 23, 0, 5, 0, time.UTC),
				layout: datetime.DefaultLayoutDate},
			want: "2022-03-01",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := datetime.EndTimeStr(tt.args.layout, tt.args.t); got != tt.want {
				t.Errorf("EndTimeStr() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFirstDateTimeOfMonth(t *testing.T) {
	type args struct {
		t time.Time
	}
	tests := []struct {
		name string
		args args
		want time.Time
	}{
		{
			name: "", args: args{
				t: time.Date(2022, 2, 28, 0, 0, 5, 0, time.UTC)},
			want: time.Date(2022, 2, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := datetime.FirstDateTimeOfMonth(tt.args.t); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FirstDateTimeOfMonth() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFirstDateTimeStrOfMonth(t *testing.T) {
	type args struct {
		layout string
		t      time.Time
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "", args: args{
				layout: "",
				t:      time.Date(2022, 2, 1, 0, 0, 5, 0, time.UTC)},
			want: "2022-02-01",
		},
		{
			name: "", args: args{
				layout: datetime.DefaultLayoutDate,
				t:      time.Date(2022, 2, 1, 0, 0, 5, 0, time.UTC)},
			want: "2022-02-01",
		},
		{
			name: "", args: args{
				layout: datetime.DefaultLayoutDate,
				t:      time.Date(2022, 2, 28, 0, 0, 5, 0, time.UTC)},
			want: "2022-02-01",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := datetime.FirstDateTimeStrOfMonth(tt.args.layout, tt.args.t); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FirstDateTimeStrOfMonth() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFirstDateTimeOfWeek(t *testing.T) {
	type args struct {
		t time.Time
	}
	tests := []struct {
		name string
		args args
		want time.Time
	}{
		{
			name: "", args: args{
				t: time.Date(2022, 1, 1, 0, 0, 5, 0, time.UTC)},
			want: time.Date(2021, 12, 27, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "", args: args{
				t: time.Date(2022, 2, 1, 0, 0, 5, 0, time.UTC)},
			want: time.Date(2022, 1, 31, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "", args: args{
				t: time.Date(2022, 2, 28, 0, 0, 5, 0, time.UTC)},
			want: time.Date(2022, 2, 28, 0, 0, 0, 0, time.UTC),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := datetime.FirstDateTimeOfWeek(tt.args.t); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FirstDateTimeOfWeek() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFirstDateTimeStrOfWeek(t *testing.T) {
	type args struct {
		layout string
		t      time.Time
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "", args: args{layout: "",
				t: time.Date(2022, 1, 1, 0, 0, 5, 0, time.UTC)},
			want: "2021-12-27",
		},
		{
			name: "", args: args{layout: datetime.DefaultLayoutDate,
				t: time.Date(2022, 1, 1, 0, 0, 5, 0, time.UTC)},
			want: "2021-12-27",
		},
		{
			name: "", args: args{layout: datetime.DefaultLayoutDateTime,
				t: time.Date(2022, 2, 1, 0, 0, 5, 0, time.UTC)},
			want: "2022-01-31 00:00:00",
		},
		{
			name: "", args: args{layout: datetime.DefaultLayoutDate,
				t: time.Date(2022, 2, 28, 0, 0, 5, 0, time.UTC)},
			want: "2022-02-28",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := datetime.FirstDateTimeStrOfWeek(tt.args.layout, tt.args.t); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FirstDateTimeStrOfWeek() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLastDateTimeOfMonth(t *testing.T) {
	type args struct {
		t time.Time
	}
	tests := []struct {
		name string
		args args
		want time.Time
	}{
		{
			name: "", args: args{
				t: time.Date(2020, 2, 1, 0, 0, 5, 0, time.UTC)},
			want: time.Date(2020, 2, 29, 23, 59, 59, 999999999, time.UTC),
		},
		{
			name: "", args: args{
				t: time.Date(2022, 2, 1, 0, 0, 5, 0, time.UTC)},
			want: time.Date(2022, 2, 28, 23, 59, 59, 999999999, time.UTC),
		},
		{
			name: "", args: args{
				t: time.Date(2022, 6, 1, 0, 0, 5, 0, time.UTC)},
			want: time.Date(2022, 6, 30, 23, 59, 59, 999999999, time.UTC),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := datetime.LastDateTimeOfMonth(tt.args.t); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("LastDateTimeOfMonth() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLastDateTimeStrOfMonth(t *testing.T) {
	type args struct {
		layout string
		t      time.Time
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "", args: args{layout: "",
				t: time.Date(2020, 2, 1, 0, 0, 5, 0, time.UTC)},
			want: "2020-02-29",
		},
		{
			name: "", args: args{layout: datetime.DefaultLayoutDate,
				t: time.Date(2020, 2, 1, 0, 0, 5, 0, time.UTC)},
			want: "2020-02-29",
		},
		{
			name: "", args: args{layout: datetime.DefaultLayoutDate,
				t: time.Date(2022, 1, 1, 0, 0, 5, 0, time.UTC)},
			want: "2022-01-31",
		},
		{
			name: "", args: args{layout: datetime.DefaultLayoutDate,
				t: time.Date(2022, 2, 1, 0, 0, 5, 0, time.UTC)},
			want: "2022-02-28",
		},
		{
			name: "", args: args{layout: datetime.DefaultLayoutDate,
				t: time.Date(2022, 6, 1, 0, 0, 5, 0, time.UTC)},
			want: "2022-06-30",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := datetime.LastDateTimeStrOfMonth(tt.args.layout, tt.args.t); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("LastDateTimeStrOfMonth() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLastDateTimeOfWeek(t *testing.T) {
	type args struct {
		t time.Time
	}
	tests := []struct {
		name string
		args args
		want time.Time
	}{
		{
			name: "", args: args{
				t: time.Date(2022, 1, 1, 0, 0, 5, 0, time.UTC)},
			want: time.Date(2022, 1, 2, 23, 59, 59, 999999999, time.UTC),
		},
		{
			name: "", args: args{
				t: time.Date(2022, 2, 1, 0, 0, 5, 0, time.UTC)},
			want: time.Date(2022, 2, 6, 23, 59, 59, 999999999, time.UTC),
		},
		{
			name: "", args: args{
				t: time.Date(2022, 2, 28, 0, 0, 5, 0, time.UTC)},
			want: time.Date(2022, 3, 6, 23, 59, 59, 999999999, time.UTC),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := datetime.LastDateTimeOfWeek(tt.args.t); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("LastDateTimeOfWeek() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLastDateTimeStrOfWeek(t *testing.T) {
	type args struct {
		layout string
		t      time.Time
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "", args: args{layout: "",
				t: time.Date(2022, 1, 1, 0, 0, 5, 0, time.UTC)},
			want: "2022-01-02",
		},
		{
			name: "", args: args{layout: datetime.DefaultLayoutDate,
				t: time.Date(2022, 1, 1, 0, 0, 5, 0, time.UTC)},
			want: "2022-01-02",
		},
		{
			name: "", args: args{layout: datetime.DefaultLayoutDateTime,
				t: time.Date(2022, 2, 1, 0, 0, 5, 0, time.UTC)},
			want: "2022-02-06 23:59:59",
		},
		{
			name: "", args: args{layout: datetime.DefaultLayoutDateTime,
				t: time.Date(2022, 2, 28, 0, 0, 5, 0, time.UTC)},
			want: "2022-03-06 23:59:59",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := datetime.LastDateTimeStrOfWeek(tt.args.layout, tt.args.t); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("LastDateTimeStrOfWeek() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNowTime(t *testing.T) {
	now := datetime.NowTime()

	tests := []struct {
		name string
		want time.Time
	}{
		{name: "", want: now},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := now; !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NowTime() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNowTimeInLocation(t *testing.T) {
	now := datetime.NowTimeInLocation(nil)

	type args struct {
		loc *time.Location
	}
	tests := []struct {
		name string
		args args
		want time.Time
	}{
		{
			name: "", args: args{loc: nil}, want: now,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := now; !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NowTimeInLocation() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNowUnix(t *testing.T) {
	nowUnix := datetime.NowUnix()

	tests := []struct {
		name string
		want int64
	}{
		{
			name: "", want: nowUnix,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nowUnix; got != tt.want {
				t.Errorf("NowUnix() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNowUnixMilli(t *testing.T) {
	nowUnixMilli := datetime.NowUnixMilli()

	tests := []struct {
		name string
		want int64
	}{
		{
			name: "", want: nowUnixMilli,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nowUnixMilli; got != tt.want {
				t.Errorf("NowUnixMilli() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNowUnixNano(t *testing.T) {
	nowUnixNano := datetime.NowUnixNano()

	tests := []struct {
		name string
		want int64
	}{
		{
			name: "", want: nowUnixNano,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nowUnixNano; got != tt.want {
				t.Errorf("NowUnixNano() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRangeDateStr(t *testing.T) {
	type args struct {
		start  time.Time
		end    time.Time
		layout string
	}
	tests := []struct {
		name string
		args args
		want []string
	}{
		{
			name: "", args: args{
				start:  time.Date(2022, 2, 25, 0, 0, 0, 0, time.UTC),
				end:    time.Date(2022, 3, 1, 0, 0, 0, 0, time.UTC),
				layout: ""},
			want: []string{"2022-02-25", "2022-02-26", "2022-02-27", "2022-02-28", "2022-03-01"},
		},
		{
			name: "", args: args{
				start:  time.Date(2022, 3, 1, 0, 0, 0, 0, time.UTC),
				end:    time.Date(2022, 3, 1, 0, 0, 0, 0, time.UTC),
				layout: ""},
			want: []string{"2022-03-01"},
		},
		{
			name: "", args: args{
				start:  time.Date(2022, 2, 25, 0, 0, 0, 0, time.UTC),
				end:    time.Date(2022, 3, 1, 0, 0, 0, 0, time.UTC),
				layout: datetime.LayoutDateISO8601},
			want: []string{"2022-02-25", "2022-02-26", "2022-02-27", "2022-02-28", "2022-03-01"},
		},
		{
			name: "", args: args{
				start:  time.Date(2022, 3, 5, 12, 0, 0, 0, time.UTC),
				end:    time.Date(2022, 3, 1, 23, 0, 0, 0, time.UTC),
				layout: datetime.LayoutDateISO8601},
			want: []string{"2022-03-01", "2022-03-02", "2022-03-03", "2022-03-04", "2022-03-05"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := datetime.RangeDateStr(tt.args.start, tt.args.end, tt.args.layout); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("RangeDateStr() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRangeTime(t *testing.T) {
	type args struct {
		start    time.Time
		end      time.Time
		interval time.Duration
	}
	tests := []struct {
		name string
		args args
		want []time.Time
	}{
		{
			name: "minute", args: args{
				end:      time.Date(2022, 3, 1, 0, 0, 0, 0, time.UTC),
				start:    time.Date(2022, 3, 1, 0, 0, 0, 0, time.UTC),
				interval: time.Minute},
			want: []time.Time{
				time.Date(2022, 3, 1, 0, 0, 0, 0, time.UTC),
			}},
		{
			name: "minute", args: args{
				end:      time.Date(2022, 3, 1, 0, 0, 0, 0, time.UTC),
				start:    time.Date(2022, 3, 1, 0, 4, 0, 0, time.UTC),
				interval: time.Minute},
			want: []time.Time{
				time.Date(2022, 3, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2022, 3, 1, 0, 1, 0, 0, time.UTC),
				time.Date(2022, 3, 1, 0, 2, 0, 0, time.UTC),
				time.Date(2022, 3, 1, 0, 3, 0, 0, time.UTC),
				time.Date(2022, 3, 1, 0, 4, 0, 0, time.UTC),
			}},
		{
			name: "minute", args: args{
				start:    time.Date(2022, 3, 1, 0, 0, 0, 0, time.UTC),
				end:      time.Date(2022, 3, 1, 0, 4, 0, 0, time.UTC),
				interval: time.Minute},
			want: []time.Time{
				time.Date(2022, 3, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2022, 3, 1, 0, 1, 0, 0, time.UTC),
				time.Date(2022, 3, 1, 0, 2, 0, 0, time.UTC),
				time.Date(2022, 3, 1, 0, 3, 0, 0, time.UTC),
				time.Date(2022, 3, 1, 0, 4, 0, 0, time.UTC),
			}},
		{
			name: "hour", args: args{
				start:    time.Date(2022, 3, 1, 2, 0, 0, 0, time.UTC),
				end:      time.Date(2022, 3, 1, 5, 0, 0, 0, time.UTC),
				interval: time.Hour},
			want: []time.Time{
				time.Date(2022, 3, 1, 2, 0, 0, 0, time.UTC),
				time.Date(2022, 3, 1, 3, 0, 0, 0, time.UTC),
				time.Date(2022, 3, 1, 4, 0, 0, 0, time.UTC),
				time.Date(2022, 3, 1, 5, 0, 0, 0, time.UTC),
			}},
		{
			name: "day", args: args{
				start:    time.Date(2022, 3, 1, 0, 0, 0, 0, time.UTC),
				end:      time.Date(2022, 3, 3, 0, 0, 0, 0, time.UTC),
				interval: time.Hour * 24},
			want: []time.Time{
				time.Date(2022, 3, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2022, 3, 2, 0, 0, 0, 0, time.UTC),
				time.Date(2022, 3, 3, 0, 0, 0, 0, time.UTC),
			}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := datetime.RangeTime(tt.args.start, tt.args.end, tt.args.interval); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("RangeTime() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStartEndTimeStr(t *testing.T) {
	type args struct {
		layout string
		t      time.Time
	}
	tests := []struct {
		name      string
		args      args
		wantStart string
		wantEnd   string
	}{
		{
			name: "", args: args{layout: "",
				t: time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC)},
			wantStart: "2022-01-01 00:00:00",
			wantEnd:   "2022-01-01 23:59:59",
		},
		{
			name: "", args: args{layout: datetime.DefaultLayoutDateTime,
				t: time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC)},
			wantStart: "2022-01-01 00:00:00",
			wantEnd:   "2022-01-01 23:59:59",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStart, gotEnd := datetime.StartEndTimeStr(tt.args.layout, tt.args.t)
			if gotStart != tt.wantStart {
				t.Errorf("StartEndTimeStr() gotStart = %v, want %v", gotStart, tt.wantStart)
			}
			if gotEnd != tt.wantEnd {
				t.Errorf("StartEndTimeStr() gotEnd = %v, want %v", gotEnd, tt.wantEnd)
			}
		})
	}
}

func TestStartTime(t *testing.T) {
	type args struct {
		t time.Time
	}
	tests := []struct {
		name string
		args args
		want time.Time
	}{
		{
			name: "", args: args{
				t: time.Date(2022, 2, 28, 0, 0, 5, 0, time.UTC)},
			want: time.Date(2022, 2, 28, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "", args: args{
				t: time.Date(2022, 3, 1, 23, 0, 5, 0, time.UTC)},
			want: time.Date(2022, 3, 1, 0, 0, 0, 0, time.UTC),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := datetime.StartTime(tt.args.t); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("StartTime() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStartTimeStr(t *testing.T) {
	type args struct {
		layout string
		t      time.Time
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "", args: args{
				t:      time.Date(2022, 2, 28, 0, 0, 5, 0, time.UTC),
				layout: ""},
			want: "2022-02-28 00:00:00",
		},
		{
			name: "", args: args{
				t:      time.Date(2022, 2, 28, 0, 0, 5, 0, time.UTC),
				layout: datetime.DefaultLayoutDateTime},
			want: "2022-02-28 00:00:00",
		},
		{
			name: "", args: args{
				t:      time.Date(2022, 3, 1, 23, 0, 5, 0, time.UTC),
				layout: datetime.DefaultLayoutDateTime},
			want: "2022-03-01 00:00:00",
		},
		{
			name: "", args: args{
				t:      time.Date(2022, 3, 1, 23, 0, 5, 0, time.UTC),
				layout: datetime.DefaultLayoutDate},
			want: "2022-03-01",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := datetime.StartTimeStr(tt.args.layout, tt.args.t); got != tt.want {
				t.Errorf("StartTimeStr() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTimeStr2Unix(t *testing.T) {
	loc := time.UTC

	type args struct {
		layout string
		value  string
		loc    *time.Location
	}
	tests := []struct {
		name string
		args args
		want int64
	}{
		{name: "", args: args{
			layout: datetime.DefaultLayoutDateTime,
			value:  "2022-01-01 00:00:00", loc: loc},
			want: 1640995200,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := datetime.TimeStr2Unix(tt.args.layout, tt.args.value, tt.args.loc); got != tt.want {
				t.Errorf("TimeStr2Unix() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTimeStr2UnixMilli(t *testing.T) {
	loc := time.UTC

	type args struct {
		layout string
		value  string
		loc    *time.Location
	}
	tests := []struct {
		name string
		args args
		want int64
	}{
		{name: "", args: args{layout: datetime.DefaultLayoutDateTime,
			value: "2022-01-01 00:00:00", loc: loc},
			want: 1640995200000,
		},
		{name: "", args: args{layout: datetime.DefaultLayoutDateTime,
			value: "2022-01-01 00:00:00.555", loc: loc},
			want: 1640995200555,
		},
		{name: "", args: args{layout: datetime.DefaultLayoutDateTimeMsec,
			value: "2022-01-01 00:00:00.555", loc: loc},
			want: 1640995200555,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := datetime.TimeStr2UnixMilli(tt.args.layout, tt.args.value, tt.args.loc); got != tt.want {
				t.Errorf("TimeStr2UnixMilli() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUnix2TimeStr(t *testing.T) {
	type args struct {
		sec    int64
		layout string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "", args: args{
				sec: 1640966400, layout: datetime.DefaultLayoutDateTime},
			want: "2021-12-31 16:00:00",
		},
		{
			name: "", args: args{
				sec: 1640966400, layout: datetime.DefaultLayoutDate},
			want: "2021-12-31",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := datetime.Unix2TimeStr(tt.args.sec, tt.args.layout); got != tt.want {
				t.Errorf("Unix2TimeStr() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUnixMilli2TimeStr(t *testing.T) {
	type args struct {
		msec   int64
		layout string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "", args: args{
				msec: 1640966400000, layout: datetime.DefaultLayoutDateTime},
			want: "2021-12-31 16:00:00",
		},
		{
			name: "", args: args{
				msec: 1640966400000, layout: datetime.DefaultLayoutDate},
			want: "2021-12-31",
		},
		{
			name: "", args: args{
				msec: 1640966400555, layout: datetime.DefaultLayoutDateTimeMsec},
			want: "2021-12-31 16:00:00.555",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := datetime.UnixMilli2TimeStr(tt.args.msec, tt.args.layout); got != tt.want {
				t.Errorf("UnixMilli2TimeStr() = %v, want %v", got, tt.want)
			}
		})
	}
}
