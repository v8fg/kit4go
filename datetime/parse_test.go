package datetime_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/v8fg/kit4go/datetime"
)

var local = time.Local

func TestParseTime(t *testing.T) {
	type args struct {
		layout string
		value  string
	}
	tests := []struct {
		name    string
		args    args
		want    time.Time
		wantErr bool
	}{
		{name: "", args: args{layout: "2006-01-02 15:04:05", value: "2022-01-01 21:00:00"},
			want:    time.Date(2022, 1, 1, 21, 0, 0, 0, local),
			wantErr: false,
		},
		{name: "", args: args{layout: "2006-01-02 15:04:05", value: "2022-01-01 21:00:00.000"},
			want:    time.Date(2022, 1, 1, 21, 0, 0, 0, local),
			wantErr: false,
		},
		{name: "", args: args{layout: "2006-01-02 15:04:05", value: "2022-01-01 21:00:00.004"},
			want:    time.Date(2022, 1, 1, 21, 0, 0, int(time.Millisecond*4), local),
			wantErr: false,
		},
		{name: "", args: args{layout: "2006-01-02 15:04", value: "2022-01-01 21:00"},
			want:    time.Date(2022, 1, 1, 21, 0, 0, 0, local),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := datetime.Parse(tt.args.layout, tt.args.value)
			t.Logf("got:%v", got)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Parse() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseTimeMostOne(t *testing.T) {
	type args struct {
		layouts []string
		value   string
	}
	tests := []struct {
		name    string
		args    args
		want    time.Time
		wantErr bool
	}{
		{name: "", args: args{
			layouts: []string{"2006-01-02", "15:04:05", "2006-01-02 15:04:05", "2006-01-02 15:04:05.000"},
			value:   "2022-01-01 21:00:00"},
			want:    time.Date(2022, 1, 1, 21, 0, 0, 0, local),
			wantErr: false,
		},
		{name: "", args: args{
			layouts: []string{"2006-01-02", "15:04:05", "2006-01-02 15", "2006-01-02 15:04:05"},
			value:   "2022-01-01 21:00:00"},
			want:    time.Date(2022, 1, 1, 21, 0, 0, 0, local),
			wantErr: false,
		},
		{name: "", args: args{
			layouts: []string{"2006-01-02", "15:04:05", "2006-01-02 15", "2006-01-02 15:04:05"},
			value:   "2022-01-01 21:00"},
			want:    time.Date(2022, 1, 1, 21, 0, 0, 0, local),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := datetime.ParseMostOne(tt.args.layouts, tt.args.value)
			if (err != nil) && tt.wantErr {
				t.Logf("ParseMostOne() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseMostOne() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseMostOne() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseTimeMostOneWithLocation(t *testing.T) {
	type args struct {
		layouts []string
		value   string
		loc     *time.Location
	}
	tests := []struct {
		name    string
		args    args
		want    time.Time
		wantErr bool
	}{
		{name: "", args: args{
			layouts: []string{"2006-01-02", "15:04:05", "2006-01-02 15:04:05", "2006-01-02 15:04:05.000"},
			value:   "2022-01-01 21:00:00"},
			want:    time.Date(2022, 1, 1, 21, 0, 0, 0, local),
			wantErr: false,
		},
		{name: "", args: args{
			layouts: []string{"2006-01-02", "15:04:05", "2006-01-02 15", "2006-01-02 15:04:05"},
			value:   "2022-01-01 21:00:00"},
			want:    time.Date(2022, 1, 1, 21, 0, 0, 0, local),
			wantErr: false,
		},
		{name: "", args: args{
			layouts: []string{"2006-01-02", "15:04:05", "2006-01-02 15", "2006-01-02 15:04:05"},
			value:   "2022-01-01 21:00"},
			want:    time.Date(2022, 1, 1, 21, 0, 0, 0, local),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := datetime.ParseMostOneInLocation(tt.args.layouts, tt.args.value, tt.args.loc)
			if (err != nil) && tt.wantErr {
				t.Logf("ParseMostOneInLocation() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseMostOneInLocation() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseMostOneInLocation() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseTimeWithLocation(t *testing.T) {
	type args struct {
		layout string
		value  string
		loc    *time.Location
	}
	tests := []struct {
		name    string
		args    args
		want    time.Time
		wantErr bool
	}{
		{name: "", args: args{layout: "2006-01-02 15:04:05", value: "2022-01-01 21:00:00"},
			want:    time.Date(2022, 1, 1, 21, 0, 0, 0, local),
			wantErr: false,
		},
		{name: "", args: args{layout: "2006-01-02 15:04:05", value: "2022-01-01 21:00:00.000"},
			want:    time.Date(2022, 1, 1, 21, 0, 0, 0, local),
			wantErr: false,
		},
		{name: "", args: args{layout: "2006-01-02 15:04:05", value: "2022-01-01 21:00:00.004"},
			want:    time.Date(2022, 1, 1, 21, 0, 0, int(time.Millisecond*4), local),
			wantErr: false,
		},
		{name: "", args: args{layout: "2006-01-02 15:04", value: "2022-01-01 21:00"},
			want:    time.Date(2022, 1, 1, 21, 0, 0, 0, local),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := datetime.ParseInLocation(tt.args.layout, tt.args.value, tt.args.loc)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseInLocation() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseInLocation() got = %v, want %v", got, tt.want)
			}
		})
	}
}
