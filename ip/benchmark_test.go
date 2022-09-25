package ip_test

import (
	"bytes"
	"math"
	"net"
	"testing"

	"github.com/v8fg/kit4go/ip"
)

func BenchmarkBytesIPToIPv4Number(b *testing.B) {
	ipSet := [][]byte{
		{0},
		{0, 0},
		{1, 0, 0},
		{255, 255, 255, 255, 255, 255, 255, 255, 255, 255},
		{255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for index := range ipSet {
			_ = ip.BytesIPToIPv4Number(ipSet[index])
		}
	}
}

func BenchmarkBytesIPToNumber(b *testing.B) {
	ipSet := [][]byte{
		{0},
		{0, 0},
		{1, 0, 0},
		{255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for index := range ipSet {
			_ = ip.BytesIPToNumber(ipSet[index])
		}
	}
}

func BenchmarkBytesIPToStrIPv4(b *testing.B) {
	ipSet := [][]byte{
		{0},
		{0, 0},
		{1, 0, 0},
		{255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for index := range ipSet {
			_ = ip.BytesIPToStrIPv4(ipSet[index])
		}
	}
}

func BenchmarkInCIDRIPRange(b *testing.B) {
	ipSet := []net.IP{
		net.IPv4(0, 0, 0, 0),
		net.IPv4(10, 1, 0, 0),
		net.IPv4(192, 168, 2, 1),
		[]byte{},
		bytes.Repeat([]byte{255}, 16),
		bytes.Repeat([]byte{255}, 18),
	}

	cidr := "192.168.192.0/16"

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for index := range ipSet {
			_ = ip.InRangeCIDRIP(cidr, ipSet[index])
		}
	}
}

func BenchmarkInCIDRRange(b *testing.B) {
	ipSet := []string{
		"",
		"10.1.0.0",
		"192.168.2.1",
		"2001:db8::",
		"2408:8226:6a02:3822::",
	}

	cidr := "192.168.192.0/16"

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for index := range ipSet {
			_ = ip.InRangeCIDRStr(cidr, ipSet[index])
		}
	}
}

func BenchmarkInCIDRsOrIPs(b *testing.B) {
	cidrOrIPs := []string{
		"192.168.192.0/16",
		"192.168.192.0/24",
		"192.168.192.0/16",
		"::ffff:ffff/24",
		"2408:8226:6a02:3822::/48",
	}

	ipSet := []net.IP{
		net.IPv4(192, 168, 192, 0),
		net.IPv4(192, 169, 192, 0),
		append(bytes.Repeat([]byte{0}, 12), bytes.Repeat([]byte{255}, 4)...),
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for index := range ipSet {
			_, _ = ip.InCIDRsOrIPs(cidrOrIPs, ipSet[index])
		}
	}
}

func BenchmarkInIPNetRange(b *testing.B) {
	ipSet := []net.IP{
		net.IPv4(0, 0, 0, 0),
		net.IPv4(10, 1, 0, 0),
		net.IPv4(192, 168, 2, 1),
		[]byte{},
		bytes.Repeat([]byte{255}, 16),
		bytes.Repeat([]byte{255}, 18),
	}

	_, _, subNet := ip.ParseCIDR("192.168.192.0/16")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for index := range ipSet {
			_ = ip.InRangeIPNet(subNet, ipSet[index])
		}
	}
}

func BenchmarkInRangeIP(b *testing.B) {
	ipSet := []net.IP{
		net.IPv4(0, 0, 0, 0),
		net.IPv4(10, 1, 0, 0),
		net.IPv4(192, 168, 2, 1),
		[]byte{},
		bytes.Repeat([]byte{255}, 16),
		bytes.Repeat([]byte{255}, 18),
	}
	start := net.IPv4(192, 168, 0, 1)
	end := net.IPv4(192, 168, 192, 0)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for index := range ipSet {
			_ = ip.InRangeIP(start, end, ipSet[index])
		}
	}
}

func BenchmarkInRangeIPv6(b *testing.B) {
	ipSet := []net.IP{
		net.IPv4(0, 0, 0, 0),
		net.IPv4(10, 1, 0, 0),
		net.IPv4(192, 168, 2, 1),
		[]byte{},
		bytes.Repeat([]byte{255}, 16),
		bytes.Repeat([]byte{255}, 18),
	}
	start := net.IPv4(192, 168, 0, 1)
	end := net.IPv4(192, 168, 192, 0)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for index := range ipSet {
			_ = ip.InRangeIPv6(start, end, ipSet[index])
		}
	}
}

func BenchmarkInRange(b *testing.B) {
	ipSet := []string{
		"",
		"10.1.0.0",
		"192.168.2.1",
		"2001:db8::",
		"2408:8226:6a02:3822::",
	}
	start := "192.168.0.1"
	end := "192.168.192.0"

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for index := range ipSet {
			_ = ip.InRange(start, end, ipSet[index])
		}
	}
}

func BenchmarkNumberIPv4ToStr(b *testing.B) {
	ipSet := []uint32{
		0,
		1,
		65536,
		167837696,
		167903231,
		3221225985,
		math.MaxUint32,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for index := range ipSet {
			_ = ip.NumberIPv4ToStr(ipSet[index])
		}
	}
}

func BenchmarkToNumber(b *testing.B) {
	ipSet := []string{
		"10.1.0.0",
		"192.0.2.1",
		"2001:db8::",
		"2408:8226:6a02:3822::",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for index := range ipSet {
			_ = ip.ToNumber(ipSet[index])
		}
	}
}

func BenchmarkToNumberIPv4(b *testing.B) {
	ipSet := []string{
		"10.1.0.0",
		"192.0.2.1",
		"2001:db8::",
		"2408:8226:6a02:3822::",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for index := range ipSet {
			_ = ip.ToNumberIPv4(ipSet[index])
		}
	}
}

func BenchmarkVersionFlag(b *testing.B) {
	ipSet := []string{
		"10.1.0.0",
		"192.0.2.1",
		"2001:db8::",
		"2408:8226:6a02:3822::",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for index := range ipSet {
			_ = ip.VersionFlag(ipSet[index])
		}
	}
}

func BenchmarkVersionFlagByContains(b *testing.B) {
	ipSet := []string{
		"10.1.0.0",
		"192.0.2.1",
		"2001:db8::",
		"2408:8226:6a02:3822::",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for index := range ipSet {
			_ = ip.VersionFlagByContains(ipSet[index])
		}
	}
}

func BenchmarkFastInRangeMixedIPsOrCIDRs(b *testing.B) {
	type args struct {
		mixedIPsOrNetIPs [][]byte
		ip               net.IP
	}
	ipSet := []args{
		{mixedIPsOrNetIPs: [][]byte{{192, 168, 1, 1}}, ip: nil},
		{mixedIPsOrNetIPs: [][]byte{{192, 168, 1, 1}}, ip: []byte{192, 168, 16, 1}},
		{mixedIPsOrNetIPs: [][]byte{{192, 168, 1, 1}}, ip: nil},
		{mixedIPsOrNetIPs: [][]byte{{192, 168, 1, 1}}, ip: []byte{192, 168, 16, 1}},
		{mixedIPsOrNetIPs: [][]byte{{192, 168, 16, 1}}, ip: []byte{192, 168, 16, 1}},
		{mixedIPsOrNetIPs: [][]byte{{16, 192, 168, 16, 1}}, ip: []byte{192, 168, 16, 1}},
		{mixedIPsOrNetIPs: [][]byte{{32, 192, 168, 1, 1}}, ip: []byte{192, 168, 16, 1}},
		{mixedIPsOrNetIPs: [][]byte{{33, 192, 168, 1, 1}}, ip: []byte{192, 168, 16, 1}},
		{mixedIPsOrNetIPs: [][]byte{{32, 192, 168, 1, 1}, {16, 192, 168, 16, 1}}, ip: []byte{192, 168, 16, 1}},
		{mixedIPsOrNetIPs: [][]byte{net.IPv4(192, 168, 1, 1)}, ip: net.IPv4(192, 168, 16, 1)},
		{mixedIPsOrNetIPs: [][]byte{append([]byte{128}, net.IPv4(192, 168, 16, 1)...)}, ip: net.IPv4(192, 168, 16, 1)},
		{mixedIPsOrNetIPs: [][]byte{append([]byte{188}, net.IPv4(192, 168, 16, 1)...)}, ip: net.IPv4(192, 168, 16, 1)},
		{mixedIPsOrNetIPs: [][]byte{append([]byte{128, 128}, net.IPv4(192, 168, 16, 1)...)}, ip: net.IPv4(192, 168, 16, 1)},
		{mixedIPsOrNetIPs: [][]byte{append([]byte{18}, net.IPv4(192, 168, 3, 1)...)}, ip: net.IPv4(192, 168, 2, 1)},
		{mixedIPsOrNetIPs: [][]byte{append([]byte{22}, net.IPv4(192, 168, 3, 1)...)}, ip: net.IPv4(192, 168, 2, 1)},
		{mixedIPsOrNetIPs: [][]byte{
			append(bytes.Repeat([]byte{0}, 10), []byte{255, 255, 192, 168, 1, 1}...),
			append(bytes.Repeat([]byte{0}, 10), []byte{255, 255, 192, 168, 1, 1}...),
		}, ip: net.IPv4(192, 168, 32, 1)},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for index := range ipSet {
			_, _ = ip.FastInRangeMixedIPsOrCIDRs(ipSet[index].mixedIPsOrNetIPs, ipSet[index].ip)
		}
	}
}

// must assure the ip and mixedIPsOrNetIPs are the simplest format to improve the performance.
func BenchmarkFastInRangeMixedIPsOrCIDRsProd(b *testing.B) {
	type args struct {
		mixedIPsOrNetIPs [][]byte
		ip               net.IP
	}
	ipSet := []args{
		{mixedIPsOrNetIPs: [][]byte{{192, 168, 1, 1}}, ip: []byte{192, 168, 16, 1}},
		{mixedIPsOrNetIPs: [][]byte{{192, 168, 16, 1}}, ip: []byte{192, 168, 16, 1}},
		{mixedIPsOrNetIPs: [][]byte{{16, 192, 168, 16, 1}}, ip: []byte{192, 168, 16, 1}},
		{mixedIPsOrNetIPs: [][]byte{{32, 192, 168, 1, 1}}, ip: []byte{192, 168, 16, 1}},
		{mixedIPsOrNetIPs: [][]byte{{32, 192, 168, 1, 1}, {16, 192, 168, 16, 1}}, ip: []byte{192, 168, 16, 1}},
		{mixedIPsOrNetIPs: [][]byte{append([]byte{18}, net.IPv4(192, 168, 3, 1)...)}, ip: net.IPv4(192, 168, 2, 1)},
		{mixedIPsOrNetIPs: [][]byte{append([]byte{22}, net.IPv4(192, 168, 3, 1)...)}, ip: net.IPv4(192, 168, 2, 1)},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for index := range ipSet {
			_, _ = ip.FastInRangeMixedIPsOrCIDRs(ipSet[index].mixedIPsOrNetIPs, ipSet[index].ip)
		}
	}
}

func BenchmarkInRangeMixedIPsOrCIDRs(b *testing.B) {
	type args struct {
		mixedIPsOrNetIPs [][]byte
		ip               net.IP
	}
	ipSet := []args{
		{mixedIPsOrNetIPs: [][]byte{{192, 168, 1, 1}}, ip: nil},
		{mixedIPsOrNetIPs: [][]byte{{192, 168, 1, 1}}, ip: []byte{192, 168, 16, 1}},
		{mixedIPsOrNetIPs: [][]byte{{192, 168, 1, 1}}, ip: nil},
		{mixedIPsOrNetIPs: [][]byte{{192, 168, 1, 1}}, ip: []byte{192, 168, 16, 1}},
		{mixedIPsOrNetIPs: [][]byte{{192, 168, 16, 1}}, ip: []byte{192, 168, 16, 1}},
		{mixedIPsOrNetIPs: [][]byte{{16, 192, 168, 16, 1}}, ip: []byte{192, 168, 16, 1}},
		{mixedIPsOrNetIPs: [][]byte{{32, 192, 168, 1, 1}}, ip: []byte{192, 168, 16, 1}},
		{mixedIPsOrNetIPs: [][]byte{{33, 192, 168, 1, 1}}, ip: []byte{192, 168, 16, 1}},
		{mixedIPsOrNetIPs: [][]byte{{32, 192, 168, 1, 1}, {16, 192, 168, 16, 1}}, ip: []byte{192, 168, 16, 1}},
		{mixedIPsOrNetIPs: [][]byte{net.IPv4(192, 168, 1, 1)}, ip: net.IPv4(192, 168, 16, 1)},
		{mixedIPsOrNetIPs: [][]byte{append([]byte{128}, net.IPv4(192, 168, 16, 1)...)}, ip: net.IPv4(192, 168, 16, 1)},
		{mixedIPsOrNetIPs: [][]byte{append([]byte{188}, net.IPv4(192, 168, 16, 1)...)}, ip: net.IPv4(192, 168, 16, 1)},
		{mixedIPsOrNetIPs: [][]byte{append([]byte{128, 128}, net.IPv4(192, 168, 16, 1)...)}, ip: net.IPv4(192, 168, 16, 1)},
		{mixedIPsOrNetIPs: [][]byte{append([]byte{18}, net.IPv4(192, 168, 3, 1)...)}, ip: net.IPv4(192, 168, 2, 1)},
		{mixedIPsOrNetIPs: [][]byte{append([]byte{22}, net.IPv4(192, 168, 3, 1)...)}, ip: net.IPv4(192, 168, 2, 1)},
		{mixedIPsOrNetIPs: [][]byte{
			append(bytes.Repeat([]byte{0}, 10), []byte{255, 255, 192, 168, 1, 1}...),
			append(bytes.Repeat([]byte{0}, 10), []byte{255, 255, 192, 168, 1, 1}...),
		}, ip: net.IPv4(192, 168, 32, 1)},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for index := range ipSet {
			_, _ = ip.InRangeMixedIPsOrCIDRs(ipSet[index].mixedIPsOrNetIPs, ipSet[index].ip)
		}
	}
}
