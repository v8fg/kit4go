package ip_test

import (
	"bytes"
	"net"
	"testing"

	"github.com/v8fg/kit4go/ip"
)

func TestInRange(t *testing.T) {
	type args struct {
		start string
		end   string
		ip    string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{name: "", args: args{"", "", ""}, want: false},
		{name: "", args: args{"", "", "192.168.1.1"}, want: false},
		{name: "", args: args{"", "192.168.192.0", "192.168.1.1"}, want: true},
		{name: "", args: args{"192.168.1.0", "", "192.168.1.1"}, want: false},
		{name: "", args: args{"192.168.0.1", "192.168.192.0", "192.168.1.1"}, want: true},
		{name: "", args: args{"192.168.0.1", "192.168.192.0", "192.169.1.1"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ip.InRange(tt.args.start, tt.args.end, tt.args.ip); got != tt.want {
				t.Errorf("InRange() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInRangeCIDRIP(t *testing.T) {
	type args struct {
		cidr string
		ip   net.IP
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{name: "", args: args{"", nil}, want: false},
		{name: "", args: args{"", []byte{192, 168, 1, 1}}, want: false},
		{name: "", args: args{"", net.IPv4(192, 168, 1, 1)}, want: false},
		{name: "", args: args{"192.168.192.0", net.IPv4(192, 168, 1, 1)}, want: false},
		{name: "", args: args{"192.168.0.1", net.IPv4(192, 168, 1, 1)}, want: false},
		{name: "", args: args{"192.168.0.1/0", net.IPv4(192, 168, 1, 1)}, want: true},
		{name: "", args: args{"192.168.0.1/16", net.IPv4(192, 168, 1, 1)}, want: true},
		{name: "", args: args{"192.168.0.1/24", net.IPv4(192, 168, 1, 1)}, want: false},
		{name: "", args: args{"192.168.1.1/32", net.IPv4(192, 168, 1, 1)}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ip.InRangeCIDRIP(tt.args.cidr, tt.args.ip); got != tt.want {
				t.Errorf("InRangeCIDRIP() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInRangeIP(t *testing.T) {
	type args struct {
		start net.IP
		end   net.IP
		ip    net.IP
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{name: "", args: args{nil, nil, nil}, want: false},
		{name: "", args: args{nil, nil, []byte{192, 168, 1, 1}}, want: false},
		{name: "", args: args{nil, []byte{192, 168, 192, 0}, []byte{192, 168, 1, 1}}, want: true},
		{name: "", args: args{[]byte{192, 168, 1, 1}, []byte{192, 168, 192, 0}, []byte{192, 168, 1, 1}}, want: true},
		{name: "", args: args{[]byte{192, 168, 1, 1}, []byte{192, 168, 192, 0}, []byte{192, 169, 1, 1}}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ip.InRangeIP(tt.args.start, tt.args.end, tt.args.ip); got != tt.want {
				t.Errorf("InRangeIP() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInRangeIPv6(t *testing.T) {
	type args struct {
		start net.IP
		end   net.IP
		ip    net.IP
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{name: "", args: args{nil, nil, nil}, want: false},
		{name: "", args: args{nil, nil, []byte{192, 168, 1, 1}}, want: false},
		{name: "", args: args{nil, []byte{192, 168, 192, 0}, []byte{192, 168, 1, 1}}, want: true},
		{name: "", args: args{[]byte{192, 168, 1, 1}, []byte{192, 168, 192, 0}, []byte{192, 168, 1, 1}}, want: true},
		{name: "", args: args{[]byte{192, 168, 1, 1}, []byte{192, 168, 192, 0}, []byte{192, 169, 1, 1}}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ip.InRangeIPv6(tt.args.start, tt.args.end, tt.args.ip); got != tt.want {
				t.Errorf("InRangeIPv6() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInRangeIPNet(t *testing.T) {
	type args struct {
		subNet *net.IPNet
		ip     net.IP
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{name: "", args: args{nil, nil}, want: false},
		{name: "", args: args{nil, []byte{192, 168, 1, 1}}, want: false},
		{name: "", args: args{&net.IPNet{IP: []byte{192, 168, 192, 0}}, []byte{192, 168, 1, 1}}, want: false},
		{name: "", args: args{&net.IPNet{IP: []byte{192, 168, 192, 0}, Mask: net.IPMask{255, 255, 255, 255}}, []byte{192, 168, 1, 1}}, want: false},
		{name: "", args: args{&net.IPNet{IP: []byte{192, 168, 192, 0}, Mask: net.IPMask{255, 255, 0, 255}}, []byte{192, 168, 1, 1}}, want: false},
		{name: "", args: args{&net.IPNet{IP: []byte{192, 168, 192, 0}, Mask: net.IPMask{255, 255, 0, 0}}, []byte{192, 168, 1, 1}}, want: true},
		{name: "", args: args{&net.IPNet{
			IP:   append(bytes.Repeat([]byte{0}, 10), []byte{255, 255, 192, 168, 192, 0}...),
			Mask: append(bytes.Repeat([]byte{0}, 10), []byte{255, 255, 0, 0, 0, 0}...)},
			[]byte{192, 168, 1, 1}}, want: true},
		{name: "", args: args{&net.IPNet{
			IP:   append(bytes.Repeat([]byte{0}, 10), []byte{255, 255, 192, 168, 192, 0}...),
			Mask: append(bytes.Repeat([]byte{255}, 12), []byte{255, 255, 255, 0}...)},
			[]byte{192, 168, 1, 1}}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ip.InRangeIPNet(tt.args.subNet, tt.args.ip); got != tt.want {
				t.Errorf("InRangeIPNet() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFastInRangeMixedIPsOrCIDRs(t *testing.T) {
	type args struct {
		mixedIPsOrNetIPs [][]byte
		ip               net.IP
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{name: "", args: args{mixedIPsOrNetIPs: [][]byte{{192, 168, 1, 1}}, ip: nil}, want: false},
		{name: "", args: args{mixedIPsOrNetIPs: [][]byte{{192, 168, 1, 1}}, ip: []byte{192, 168, 16, 1}}, want: false},
		{name: "", args: args{mixedIPsOrNetIPs: [][]byte{{192, 168, 16, 1}}, ip: []byte{192, 168, 16, 1}}, want: true},
		{name: "", args: args{mixedIPsOrNetIPs: [][]byte{{16, 192, 168, 16, 1}}, ip: []byte{192, 168, 16, 1}}, want: true},
		{name: "", args: args{mixedIPsOrNetIPs: [][]byte{{32, 192, 168, 1, 1}}, ip: []byte{192, 168, 16, 1}}, want: false},
		{name: "", args: args{mixedIPsOrNetIPs: [][]byte{{33, 192, 168, 1, 1}}, ip: []byte{192, 168, 16, 1}}, want: false},
		{name: "", args: args{mixedIPsOrNetIPs: [][]byte{{32, 192, 168, 1, 1}, {16, 192, 168, 16, 1}}, ip: []byte{192, 168, 16, 1}}, want: true},
		{name: "", args: args{mixedIPsOrNetIPs: [][]byte{net.IPv4(192, 168, 1, 1)}, ip: net.IPv4(192, 168, 16, 1)}, want: false},
		{name: "", args: args{mixedIPsOrNetIPs: [][]byte{append([]byte{128}, net.IPv4(192, 168, 16, 1)...)}, ip: net.IPv4(192, 168, 16, 1)}, want: true},
		{name: "", args: args{mixedIPsOrNetIPs: [][]byte{append([]byte{188}, net.IPv4(192, 168, 16, 1)...)}, ip: net.IPv4(192, 168, 16, 1)}, want: false},
		{name: "", args: args{mixedIPsOrNetIPs: [][]byte{append([]byte{128, 128}, net.IPv4(192, 168, 16, 1)...)}, ip: net.IPv4(192, 168, 16, 1)}, want: false},
		{name: "", args: args{mixedIPsOrNetIPs: [][]byte{append([]byte{18}, net.IPv4(192, 168, 3, 1)...)}, ip: net.IPv4(192, 168, 2, 1)}, want: true},
		{name: "", args: args{mixedIPsOrNetIPs: [][]byte{append([]byte{22}, net.IPv4(192, 168, 3, 1)...)}, ip: net.IPv4(192, 168, 2, 1)}, want: true},
		{name: "", args: args{
			mixedIPsOrNetIPs: [][]byte{
				append(bytes.Repeat([]byte{0}, 10), []byte{255, 255, 192, 168, 1, 1}...),
				append(bytes.Repeat([]byte{0}, 10), []byte{255, 255, 192, 168, 1, 1}...),
			},
			ip: net.IPv4(192, 168, 32, 1)}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, got := ip.FastInRangeMixedIPsOrCIDRs(tt.args.mixedIPsOrNetIPs, tt.args.ip); got != tt.want {
				t.Errorf("FastInRangeMixedIPsOrCIDRs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCIDRContains(t *testing.T) {
	type args struct {
		ones         int
		ipNet        net.IP
		ip           net.IP
		formatIPToV4 bool
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{name: "", args: args{0, net.IPv4(192, 168, 3, 1), nil, false}, want: false},
		{name: "", args: args{0, net.IPv4(192, 168, 3, 1), net.IPv4(192, 168, 3, 1), false}, want: false},
		{name: "", args: args{0, net.IPv4(192, 168, 3, 1), net.IPv4(192, 168, 3, 1), true}, want: true},
		{name: "", args: args{0, []byte{192, 168, 3, 1}, net.IPv4(192, 168, 3, 1), true}, want: true},
		{name: "", args: args{0, []byte{192, 168, 3, 1}, []byte{192, 168, 3, 1}, true}, want: true},
		{name: "", args: args{0, net.IPv4(192, 168, 3, 1), []byte{192, 168, 3, 1}, true}, want: true},
		{name: "", args: args{16, []byte{192, 168, 3, 1}, []byte{192, 168, 1, 1}, true}, want: true},
		{name: "", args: args{24, []byte{192, 168, 3, 1}, []byte{192, 168, 1, 1}, true}, want: false},
		{name: "", args: args{32, []byte{192, 168, 3, 1}, []byte{192, 168, 1, 1}, true}, want: false},
		{name: "", args: args{32, []byte{192, 168, 1, 1}, []byte{192, 168, 1, 1}, true}, want: true},
		{name: "", args: args{32, []byte{192, 168, 3, 1}, []byte{192, 168, 2, 1}, true}, want: false},
		{name: "", args: args{18, []byte{192, 168, 3, 1}, []byte{192, 168, 2, 1}, true}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ip.CIDRContains(tt.args.ones, tt.args.ipNet, tt.args.ip, tt.args.formatIPToV4); got != tt.want {
				t.Errorf("CIDRContains() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInRangeMixedIPsOrCIDRs(t *testing.T) {
	type args struct {
		mixedIPsOrNetIPs [][]byte
		ip               net.IP
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{name: "", args: args{mixedIPsOrNetIPs: [][]byte{{192, 168, 1, 1}}, ip: nil}, want: false},
		{name: "", args: args{mixedIPsOrNetIPs: [][]byte{{192, 168, 1, 1}}, ip: []byte{192, 168, 16, 1}}, want: false},
		{name: "", args: args{mixedIPsOrNetIPs: [][]byte{{192, 168, 16, 1}}, ip: []byte{192, 168, 16, 1}}, want: true},
		{name: "", args: args{mixedIPsOrNetIPs: [][]byte{{16, 192, 168, 16, 1}}, ip: []byte{192, 168, 16, 1}}, want: true},
		{name: "", args: args{mixedIPsOrNetIPs: [][]byte{{32, 192, 168, 1, 1}}, ip: []byte{192, 168, 16, 1}}, want: false},
		{name: "", args: args{mixedIPsOrNetIPs: [][]byte{{33, 192, 168, 1, 1}}, ip: []byte{192, 168, 16, 1}}, want: false},
		{name: "", args: args{mixedIPsOrNetIPs: [][]byte{{32, 192, 168, 1, 1}, {16, 192, 168, 16, 1}}, ip: []byte{192, 168, 16, 1}}, want: true},
		{name: "", args: args{mixedIPsOrNetIPs: [][]byte{net.IPv4(192, 168, 1, 1)}, ip: net.IPv4(192, 168, 16, 1)}, want: false},
		{name: "", args: args{mixedIPsOrNetIPs: [][]byte{append([]byte{128}, net.IPv4(192, 168, 16, 1)...)}, ip: net.IPv4(192, 168, 16, 1)}, want: true},
		{name: "", args: args{mixedIPsOrNetIPs: [][]byte{append([]byte{188}, net.IPv4(192, 168, 16, 1)...)}, ip: net.IPv4(192, 168, 16, 1)}, want: false},
		{name: "", args: args{mixedIPsOrNetIPs: [][]byte{append([]byte{128, 128}, net.IPv4(192, 168, 16, 1)...)}, ip: net.IPv4(192, 168, 16, 1)}, want: false},
		{name: "", args: args{
			mixedIPsOrNetIPs: [][]byte{
				append(bytes.Repeat([]byte{0}, 10), []byte{255, 255, 192, 168, 1, 1}...),
				append(bytes.Repeat([]byte{0}, 10), []byte{255, 255, 192, 168, 1, 1}...),
			},
			ip: net.IPv4(192, 168, 32, 1)}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, got := ip.InRangeMixedIPsOrCIDRs(tt.args.mixedIPsOrNetIPs, tt.args.ip); got != tt.want {
				t.Errorf("InRangeMixedIPsOrCIDRs() = %v, want %v", got, tt.want)
			}
		})
	}
}
