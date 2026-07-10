package datetime_test

import (
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/v8fg/kit4go/datetime"
)

// TestMain pins time.Local to UTC for the package's tests so that layout-driven
// parses (which default to Local) are deterministic and timezone-independent.
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

// TestAddDate covers AddDate for forward and backward day offsets.
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

// TestAddDays covers AddDays for positive and negative day deltas.
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

// TestAddDuration covers AddDuration with forward and backward durations.
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

// TestAddDurationStr covers AddDurationStr across valid duration strings; the
// error path is exercised separately in TestParseErrors.
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
			if got, _ := datetime.AddDurationStr(tt.args.duration, tt.args.t); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("AddDurationStr() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestDeltaDateDay covers DeltaDateDays signed, symmetric day counting. Each
// side snaps to midnight, so a same-day pair is 0 and a next-day pair is 1;
// swapping the arguments flips only the sign.
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
			name: "same-day", args: args{
				start: time.Date(2022, 2, 28, 0, 0, 0, 0, time.UTC),
				end:   time.Date(2022, 2, 28, 1, 0, 0, 0, time.UTC),
			}, want: 0,
		},
		{
			name: "next-day", args: args{
				start: time.Date(2022, 2, 28, 0, 0, 0, 0, time.UTC),
				end:   time.Date(2022, 3, 1, 23, 59, 59, 999999999, time.UTC),
			}, want: 1,
		},
		{
			name: "five-day-forward", args: args{
				start: time.Date(2022, 6, 15, 0, 0, 0, 0, time.UTC),
				end:   time.Date(2022, 6, 20, 0, 0, 0, 0, time.UTC),
			}, want: 5,
		},
		{
			name: "five-day-backward", args: args{
				start: time.Date(2022, 6, 20, 0, 0, 0, 0, time.UTC),
				end:   time.Date(2022, 6, 15, 0, 0, 0, 0, time.UTC),
			}, want: -5,
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

// TestDeltaDateDaysSymmetric guards the asymmetry bug: the old formula applied
// EndTime(end) on one side and StartTime(start) + 1 on the other, so forward
// 6/15->6/20 gave 6 while backward 6/20->6/15 gave -3 (magnitudes disagreed).
// With both ends snapped to midnight and no +1, the pair must be exact
// negatives. This test fails on the old code.
func TestDeltaDateDaysSymmetric(t *testing.T) {
	start := time.Date(2022, 6, 15, 10, 30, 0, 0, time.UTC)
	end := time.Date(2022, 6, 20, 22, 0, 0, 0, time.UTC)
	fwd := datetime.DeltaDateDays(start, end)
	bwd := datetime.DeltaDateDays(end, start)
	if fwd != -bwd {
		t.Errorf("DeltaDateDays not symmetric: forward=%d, backward=%d (want exact negatives)", fwd, bwd)
	}
	if fwd != 5 {
		t.Errorf("DeltaDateDays(forward 5-day span) = %d, want 5", fwd)
	}
	if bwd != -5 {
		t.Errorf("DeltaDateDays(backward 5-day span) = %d, want -5", bwd)
	}
}

// TestDeltaDateDays_DST guards the DST fix. The old formula diffed local
// midnights and divided by 24h, so a spring-forward day (23h wall-clock) made
// consecutive dates across it count as 0 days (and a 2-day span over it as 1).
// Re-expressing each date as UTC midnight makes the count exact.
func TestDeltaDateDays_DST(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("time zone database unavailable, skipping DST test: %v", err)
	}
	// US 2022 spring-forward was Mar 13: the Mar 13 calendar day is 23h.
	cases := []struct {
		start, end time.Time
		want       int
	}{
		{time.Date(2022, 3, 13, 10, 0, 0, 0, loc), time.Date(2022, 3, 14, 9, 0, 0, 0, loc), 1}, // across the 23h day
		{time.Date(2022, 3, 12, 10, 0, 0, 0, loc), time.Date(2022, 3, 14, 9, 0, 0, 0, loc), 2}, // 2-day span over DST
		{time.Date(2022, 11, 6, 10, 0, 0, 0, loc), time.Date(2022, 11, 7, 9, 0, 0, 0, loc), 1}, // fall-back 25h day
	}
	for _, c := range cases {
		if got := datetime.DeltaDateDays(c.start, c.end); got != c.want {
			t.Errorf("DeltaDateDays(%s -> %s) = %d, want %d (DST)",
				c.start.Format("2006-01-02"), c.end.Format("2006-01-02"), got, c.want)
		}
	}
}

// TestDeltaDays covers DeltaDays returning fractional day spans.
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

// TestDeltaHours covers DeltaHours for sub-day and cross-day spans.
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

// TestDeltaMinutes covers DeltaMinutes for sub-hour and cross-hour spans.
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

// TestDeltaSeconds covers DeltaSeconds for sub-minute and cross-hour spans.
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

// TestEndTime covers EndTime producing the 23:59:59.999999999 boundary.
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

// TestEndTimeStr covers EndTimeStr, including the empty-layout fallback to
// DefaultLayoutDateTime and the date-only layout.
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

// TestFirstDateTimeOfMonth covers FirstDateTimeOfMonth returning day-1 00:00:00.
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

// TestFirstDateTimeStrOfMonth covers FirstDateTimeStrOfMonth, including the
// empty-layout fallback to DefaultLayoutDate.
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

// TestFirstDateTimeOfWeek covers FirstDateTimeOfWeek across all three
// locale-aligned first days: Monday (ISO/Europe/China), Sunday (US/ad-tech),
// and Saturday (MENA), including the Sunday-stays-in-current-week case that the
// parameterization fixed.
func TestFirstDateTimeOfWeek(t *testing.T) {
	tests := []struct {
		name     string
		t        time.Time
		firstDay time.Weekday
		want     time.Time
	}{
		// Monday-first (ISO 8601 / Europe / China) — the old hardcoded behavior.
		{"mon/sat→prev monday", time.Date(2022, 1, 1, 0, 0, 5, 0, time.UTC), time.Monday, time.Date(2021, 12, 27, 0, 0, 0, 0, time.UTC)},
		{"mon/tue→same monday", time.Date(2022, 2, 1, 0, 0, 5, 0, time.UTC), time.Monday, time.Date(2022, 1, 31, 0, 0, 0, 0, time.UTC)},
		{"mon/monday→same day", time.Date(2022, 2, 28, 0, 0, 5, 0, time.UTC), time.Monday, time.Date(2022, 2, 28, 0, 0, 0, 0, time.UTC)},
		// Sunday-first (US / ad-tech). Sunday must file into the current week,
		// not next week (the bug the parameterization fixed).
		{"sun/sunday→same day", time.Date(2022, 1, 2, 0, 0, 5, 0, time.UTC), time.Sunday, time.Date(2022, 1, 2, 0, 0, 0, 0, time.UTC)},
		{"sun/saturday→prev sunday", time.Date(2022, 1, 1, 0, 0, 5, 0, time.UTC), time.Sunday, time.Date(2021, 12, 26, 0, 0, 0, 0, time.UTC)},
		// Saturday-first (MENA).
		{"sat/friday→prev saturday", time.Date(2022, 1, 7, 0, 0, 5, 0, time.UTC), time.Saturday, time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := datetime.FirstDateTimeOfWeek(tt.t, tt.firstDay)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FirstDateTimeOfWeek(%v, %v) = %v, want %v", tt.t, tt.firstDay, got, tt.want)
			}
		})
	}
}

// TestFirstDateTimeStrOfWeek covers FirstDateTimeStrOfWeek for the default
// (empty) layout under Monday-first and an explicit layout under Sunday-first.
func TestFirstDateTimeStrOfWeek(t *testing.T) {
	// 2022-02-01 (Tuesday); Monday-first week starts 2022-01-31.
	got := datetime.FirstDateTimeStrOfWeek("", time.Date(2022, 2, 1, 0, 0, 5, 0, time.UTC), time.Monday)
	if got != "2022-01-31" {
		t.Errorf("FirstDateTimeStrOfWeek default layout = %q, want 2022-01-31", got)
	}
	// Sunday-first: the same Tuesday rolls back to Sunday 2022-01-30.
	got = datetime.FirstDateTimeStrOfWeek(datetime.DefaultLayoutDateTime, time.Date(2022, 2, 1, 0, 0, 5, 0, time.UTC), time.Sunday)
	if got != "2022-01-30 00:00:00" {
		t.Errorf("FirstDateTimeStrOfWeek sunday-first = %q, want 2022-01-30 00:00:00", got)
	}
}

// TestFirstDateTimeOfISOWeek covers FirstDateTimeOfISOWeek landing on the
// Monday of the ISO week for a Saturday input that crosses a year boundary.
func TestFirstDateTimeOfISOWeek(t *testing.T) {
	// 2022-01-01 (Saturday) → ISO week starts Monday 2021-12-27.
	got := datetime.FirstDateTimeOfISOWeek(time.Date(2022, 1, 1, 0, 0, 5, 0, time.UTC))
	if want := time.Date(2021, 12, 27, 0, 0, 0, 0, time.UTC); !reflect.DeepEqual(got, want) {
		t.Errorf("FirstDateTimeOfISOWeek = %v, want %v", got, want)
	}
}

// TestLastDateTimeOfMonth covers LastDateTimeOfMonth across leap (29-day) and
// non-leap (28/30-day) month ends.
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

// TestLastDateTimeStrOfMonth covers LastDateTimeStrOfMonth, including the
// empty-layout fallback and leap-year February.
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

// TestLastDateTimeOfWeek covers LastDateTimeOfWeek under Monday-first
// (Mon..Sun) and Sunday-first (Sun..Sat) weeks, including the same-day end.
func TestLastDateTimeOfWeek(t *testing.T) {
	tests := []struct {
		name     string
		t        time.Time
		firstDay time.Weekday
		want     time.Time
	}{
		// Monday-first: week runs Mon..Sun.
		{"mon/sat→ends sunday", time.Date(2022, 1, 1, 0, 0, 5, 0, time.UTC), time.Monday, time.Date(2022, 1, 2, 23, 59, 59, 999999999, time.UTC)},
		{"mon/tue→ends sunday", time.Date(2022, 2, 1, 0, 0, 5, 0, time.UTC), time.Monday, time.Date(2022, 2, 6, 23, 59, 59, 999999999, time.UTC)},
		// Sunday-first: week runs Sun..Sat.
		{"sun/sunday→ends saturday", time.Date(2022, 1, 2, 0, 0, 5, 0, time.UTC), time.Sunday, time.Date(2022, 1, 8, 23, 59, 59, 999999999, time.UTC)},
		{"sun/saturday→ends saturday", time.Date(2022, 1, 1, 0, 0, 5, 0, time.UTC), time.Sunday, time.Date(2022, 1, 1, 23, 59, 59, 999999999, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := datetime.LastDateTimeOfWeek(tt.t, tt.firstDay)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("LastDateTimeOfWeek(%v, %v) = %v, want %v", tt.t, tt.firstDay, got, tt.want)
			}
		})
	}
}

// TestLastDateTimeStrOfWeek covers LastDateTimeStrOfWeek for an explicit
// datetime layout and the empty-layout fallback to DefaultLayoutDate.
func TestLastDateTimeStrOfWeek(t *testing.T) {
	// 2022-02-01 (Tuesday); Monday-first week ends Sunday 2022-02-06.
	got := datetime.LastDateTimeStrOfWeek(datetime.DefaultLayoutDateTime, time.Date(2022, 2, 1, 0, 0, 5, 0, time.UTC), time.Monday)
	if got != "2022-02-06 23:59:59" {
		t.Errorf("LastDateTimeStrOfWeek monday-first = %q, want 2022-02-06 23:59:59", got)
	}
	// Empty layout falls back to DefaultLayoutDate ("2006-01-02"), the same
	// default path the FirstDateTimeStrOfMonth sibling covers but this function
	// previously did not.
	got = datetime.LastDateTimeStrOfWeek("", time.Date(2022, 2, 1, 0, 0, 5, 0, time.UTC), time.Monday)
	if got != "2022-02-06" {
		t.Errorf("LastDateTimeStrOfWeek empty-layout = %q, want 2022-02-06", got)
	}
}

// TestLastDateTimeOfISOWeek covers LastDateTimeOfISOWeek landing on the Sunday
// ending the ISO week for a Saturday input that crosses a year boundary.
func TestLastDateTimeOfISOWeek(t *testing.T) {
	// 2022-01-01 (Saturday) → ISO week ends Sunday 2022-01-02.
	got := datetime.LastDateTimeOfISOWeek(time.Date(2022, 1, 1, 0, 0, 5, 0, time.UTC))
	if want := time.Date(2022, 1, 2, 23, 59, 59, 999999999, time.UTC); !reflect.DeepEqual(got, want) {
		t.Errorf("LastDateTimeOfISOWeek = %v, want %v", got, want)
	}
}

// TestNowTime verifies NowTime returns a non-zero wall-clock time.
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

// TestNowTimeInLocation verifies NowTimeInLocation falls back to Local when the
// location is nil.
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

// TestNowUnix verifies NowUnix returns a current Unix-seconds value.
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

// TestNowUnixMilli verifies NowUnixMilli returns a current Unix-millis value.
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

// TestNowUnixNano verifies NowUnixNano returns a current Unix-nanos value.
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

// TestRangeDateStr covers RangeDateStr happy paths: empty-layout fallback,
// single-day spans, and swapped start/end ordering. The DoS-prevention guards
// are covered separately in TestRangeDateStrGuards.
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

// TestRangeDateStrGuards covers the DoS-prevention guards: an entry count that
// exceeds maxRangeEntries returns nil rather than allocating unboundedly.
// (The delta<=0 half of the same guard is unreachable in practice —
// EndTime(end) >= StartTime(start) by construction so delta is always >= 1 —
// but the >maxRangeEntries branch is exercised here.)
func TestRangeDateStrGuards(t *testing.T) {
	start := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	// 10001 calendar days -> delta 10001 > maxRangeEntries (10000) -> nil.
	end := start.AddDate(0, 0, 10000)
	if got := datetime.RangeDateStr(start, end, ""); got != nil {
		t.Errorf("RangeDateStr over-limit = len %d, want nil (DoS guard)", len(got))
	}
	// Boundary sanity: exactly maxRangeEntries (10000) is allowed, not nil.
	endOK := start.AddDate(0, 0, 9999)
	if got := datetime.RangeDateStr(start, endOK, ""); len(got) != 10000 {
		t.Errorf("RangeDateStr at-limit = len %d, want 10000", len(got))
	}
}

// TestRangeTime covers RangeTime inclusive stepping across minute/hour/day
// intervals and both start<end and start>end orderings. The interval<=0 and
// over-limit guards are covered separately in TestRangeTimeGuards.
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

// TestRangeTimeGuards covers the input-guard branches that the happy-path
// table never reaches:
//   - interval <= 0 (would divide by zero) -> nil,
//   - entry count exceeding maxRangeEntries -> nil (DoS guard).
//
// The delta<=0 half of the second guard is defensive: after the start>end
// swap, end>=start and interval>0 hold, so delta is always >= 1 and that
// disjunct is unreachable; only the >maxRangeEntries branch is hit here.
func TestRangeTimeGuards(t *testing.T) {
	start := time.Date(2022, 3, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2022, 3, 2, 0, 0, 0, 0, time.UTC)

	if got := datetime.RangeTime(start, end, 0); got != nil {
		t.Errorf("RangeTime interval=0 = %v, want nil", got)
	}
	if got := datetime.RangeTime(start, end, -time.Second); got != nil {
		t.Errorf("RangeTime interval<0 = %v, want nil", got)
	}

	// 10001 one-second entries -> delta 10001 > maxRangeEntries (10000) -> nil.
	longEnd := start.Add(10000 * time.Second)
	if got := datetime.RangeTime(start, longEnd, time.Second); got != nil {
		t.Errorf("RangeTime over-limit = len %d, want nil (DoS guard)", len(got))
	}
	// Boundary sanity: exactly maxRangeEntries (10000) is allowed, not nil.
	okEnd := start.Add(9999 * time.Second)
	if got := datetime.RangeTime(start, okEnd, time.Second); len(got) != 10000 {
		t.Errorf("RangeTime at-limit = len %d, want 10000", len(got))
	}
}

// TestStartEndTimeStr covers StartEndTimeStr returning the day's start and end
// strings, including the empty-layout fallback to DefaultLayoutDateTime.
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

// TestStartTime covers StartTime producing the 00:00:00 boundary.
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

// TestStartTimeStr covers StartTimeStr, including the empty-layout fallback to
// DefaultLayoutDateTime and the date-only layout.
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

// TestTimeStr2Unix covers TimeStr2Unix happy path; the invalid-value error
// path is exercised separately in TestParseErrors.
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
			if got, _ := datetime.TimeStr2Unix(tt.args.layout, tt.args.value, tt.args.loc); got != tt.want {
				t.Errorf("TimeStr2Unix() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestTimeStr2UnixMilli covers TimeStr2UnixMilli across second, millisecond,
// and explicit millisecond-layout inputs.
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
			if got, _ := datetime.TimeStr2UnixMilli(tt.args.layout, tt.args.value, tt.args.loc); got != tt.want {
				t.Errorf("TimeStr2UnixMilli() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestUnix2TimeStr covers Unix2TimeStr for datetime and date-only layouts.
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

// TestUnixMilli2TimeStr covers UnixMilli2TimeStr for datetime, date-only, and
// millisecond layouts.
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

// TestUnixToDuration covers UnixToDuration converting second counts to durations.
func TestUnixToDuration(t *testing.T) {
	type args struct {
		sec int64
	}
	tests := []struct {
		name string
		args args
		want time.Duration
	}{
		{name: "864000s", args: args{864000 * int64(time.Second)}, want: 240 * time.Hour},
		{name: "36000s", args: args{36000 * int64(time.Second)}, want: 10 * time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := datetime.UnixToDuration(tt.args.sec); got != tt.want {
				t.Errorf("UnixToDuration() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestUnixMilliToDuration covers UnixMilliToDuration converting millisecond
// counts to durations.
func TestUnixMilliToDuration(t *testing.T) {
	type args struct {
		msec int64
	}
	tests := []struct {
		name string
		args args
		want time.Duration
	}{
		{name: "86400000ms", args: args{86400000 * int64(time.Millisecond)}, want: 24 * time.Hour},
		{name: "3600000ms", args: args{3600000 * int64(time.Millisecond)}, want: time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := datetime.UnixMilliToDuration(tt.args.msec); got != tt.want {
				t.Errorf("UnixMilliToDuration() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestDurationStrToDuration covers DurationStrToDuration for valid Go duration
// strings and the rejected "1d" form (time.ParseDuration does not support days).
func TestDurationStrToDuration(t *testing.T) {
	type args struct {
		duration string
	}
	tests := []struct {
		name string
		args args
		want time.Duration
	}{
		{name: "1d", args: args{"1d"}, want: 0},
		{name: "1h", args: args{"1h"}, want: time.Hour},
		{name: "1h", args: args{"24h"}, want: 24 * time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, _ := datetime.DurationStrToDuration(tt.args.duration); got != tt.want {
				t.Errorf("DurationStrToDuration() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestDurationStrToUnix covers DurationStrToUnix converting valid duration
// strings to seconds and rejecting the unsupported "1d" form.
func TestDurationStrToUnix(t *testing.T) {
	type args struct {
		duration string
	}
	tests := []struct {
		name string
		args args
		want float64
	}{
		{name: "1d", args: args{"1d"}, want: 0},
		{name: "1h", args: args{"1h"}, want: 3600},
		{name: "1h", args: args{"24h"}, want: 86400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, _ := datetime.DurationStrToUnix(tt.args.duration); got != tt.want {
				t.Errorf("DurationStrToUnix() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestDurationToUnix covers DurationToUnix returning seconds for hour and day
// durations.
func TestDurationToUnix(t *testing.T) {
	type args struct {
		d time.Duration
	}
	tests := []struct {
		name string
		args args
		want float64
	}{
		{name: "1h", args: args{time.Hour}, want: 3600},
		{name: "1h", args: args{24 * time.Hour}, want: 86400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := datetime.DurationToUnix(tt.args.d); got != tt.want {
				t.Errorf("DurationToUnix() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestParseErrors verifies the formerly-silent parsers now surface failures as
// errors instead of in-band sentinels (unix -62135596800 / duration 0).
func TestParseErrors(t *testing.T) {
	if _, err := datetime.TimeStr2Unix("2006-01-02", "not-a-date", time.UTC); err == nil {
		t.Error("TimeStr2Unix should error on an invalid value")
	}
	if _, err := datetime.TimeStr2UnixMilli("2006-01-02", "not-a-date", time.UTC); err == nil {
		t.Error("TimeStr2UnixMilli should error on an invalid value")
	}
	if _, err := datetime.AddDurationStr("nope", time.Now()); err == nil {
		t.Error("AddDurationStr should error on an invalid duration")
	}
	if _, err := datetime.DurationStrToDuration("nope"); err == nil {
		t.Error("DurationStrToDuration should error on an invalid duration")
	}
	if _, err := datetime.DurationStrToUnix("nope"); err == nil {
		t.Error("DurationStrToUnix should error on an invalid duration")
	}
}
