package ip_test

import (
	"testing"

	"github.com/v8fg/kit4go/ip"
)

func TestFlagString(t *testing.T) {
	type args struct {
		ipVFlag ip.Flag
	}
	tests := []struct {
		name              string
		args              args
		wantIpVFlagString string
	}{
		{name: "", args: args{ip.Flag(-2)}, wantIpVFlagString: ""},
		{name: "", args: args{ip.FlagVInValid}, wantIpVFlagString: "invalid"},
		{name: "", args: args{ip.FlagV4}, wantIpVFlagString: "ipv4"},
		{name: "", args: args{ip.FlagV6}, wantIpVFlagString: "ipv6"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotIpVersionFlag := tt.args.ipVFlag.String(); gotIpVersionFlag != tt.wantIpVFlagString {
				t.Errorf("VersionFlag() = %v, want %v", gotIpVersionFlag, tt.wantIpVFlagString)
			}
		})
	}
}

func TestToCIDRStr(t *testing.T) {
	type args struct {
		fuzzyIPV4 string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{name: "", args: args{"192.168.1.1"}, want: "192.168.1.1/32"},
		{name: "", args: args{"192.168.1.*"}, want: "192.168.1.0/24"},
		{name: "", args: args{"192.168.*.*"}, want: "192.168.0.0/16"},
		{name: "", args: args{"192.*.*.*"}, want: "192.0.0.0/8"},
		{name: "", args: args{"192.*.*"}, want: "192.0.0.0/8"},
		{name: "", args: args{"192.*"}, want: "192.0.0.0/8"},
		{name: "", args: args{"255.*"}, want: "255.0.0.0/8"},
		{name: "", args: args{"256.*"}, want: ""},
		{name: "", args: args{"*.*"}, want: "0.0.0.0/0"},
		{name: "", args: args{"*.*.*"}, want: "0.0.0.0/0"},
		{name: "", args: args{"*.*.*.*"}, want: "0.0.0.0/0"},
		{name: "", args: args{""}, want: ""},
		{name: "", args: args{"0"}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ip.ToCIDRStr(tt.args.fuzzyIPV4); got != tt.want {
				t.Errorf("ToCIDRStr() = %v, want %v", got, tt.want)
			}
		})
	}
}
