package ip_test

import (
	"encoding/json"
	"errors"
	"net"
	"testing"

	"github.com/agiledragon/gomonkey"
	"github.com/smartystreets/goconvey/convey"

	"github.com/v8fg/kit4go/ip"
)

func TestGetIPAll(t *testing.T) {
	convey.Convey("TestGetIPAll", t, func() {
		outputs := []gomonkey.OutputCell{
			{Values: gomonkey.Params{nil, errors.New("nil")}, Times: 6},
			{Values: gomonkey.Params{[]net.Addr{
				&net.IPNet{IP: net.ParseIP("fe80::1"), Mask: net.CIDRMask(64, 128)},
				&net.IPNet{IP: net.ParseIP("fe80::aede:48ff:fe00:1122"), Mask: net.CIDRMask(64, 128)},
				&net.IPNet{IP: net.ParseIP("fe80::18f1:e8fa:6023:2707"), Mask: net.CIDRMask(64, 128)},
				&net.IPNet{IP: net.ParseIP("192.168.13.19"), Mask: net.CIDRMask(112, 128)},
				&net.IPNet{IP: net.ParseIP("192.168.52.87"), Mask: net.CIDRMask(128, 128)},
				&net.IPNet{IP: net.ParseIP("169.254.19.0"), Mask: net.CIDRMask(112, 128)},
			}, nil}, Times: 6},
		}
		af := gomonkey.ApplyFuncSeq(net.InterfaceAddrs, outputs)
		defer af.Reset()

		// nil for mock net.InterfaceAddrs
		convey.So(ip.GetIPAll(ip.FlagVInValid, true), convey.ShouldBeNil)
		convey.So(ip.GetIPAll(ip.FlagVInValid, false), convey.ShouldBeNil)
		convey.So(ip.GetIPAll(ip.FlagV4, true), convey.ShouldBeNil)
		convey.So(ip.GetIPAll(ip.FlagV4, false), convey.ShouldBeNil)
		convey.So(ip.GetIPAll(ip.FlagV6, true), convey.ShouldBeNil)
		convey.So(ip.GetIPAll(ip.FlagV6, false), convey.ShouldBeNil)

		// valid for mock net.InterfaceAddrs
		convey.So(ip.GetIPAll(ip.FlagVInValid, true), convey.ShouldResemble, []string{"192.168.13.19", "192.168.52.87"})
		convey.So(ip.GetIPAll(ip.FlagVInValid, false), convey.ShouldResemble, []string{"fe80::1", "fe80::aede:48ff:fe00:1122", "fe80::18f1:e8fa:6023:2707", "192.168.13.19", "192.168.52.87", "169.254.19.0"})
		convey.So(ip.GetIPAll(ip.FlagV4, true), convey.ShouldResemble, []string{"192.168.13.19", "192.168.52.87"})
		convey.So(ip.GetIPAll(ip.FlagV4, false), convey.ShouldResemble, []string{"192.168.13.19", "192.168.52.87", "169.254.19.0"})
		convey.So(ip.GetIPAll(ip.FlagV6, true), convey.ShouldBeNil)
		convey.So(ip.GetIPAll(ip.FlagV6, false), convey.ShouldResemble, []string{"fe80::1", "fe80::aede:48ff:fe00:1122", "fe80::18f1:e8fa:6023:2707"})

	})
}

func TestGetIPSet(t *testing.T) {
	convey.Convey("TestGetIPSet", t, func() {
		outputs := []gomonkey.OutputCell{
			{Values: gomonkey.Params{nil, errors.New("nil")}, Times: 1},
			{Values: gomonkey.Params{[]net.Addr{
				&net.IPNet{IP: net.ParseIP("fe80::1"), Mask: net.CIDRMask(64, 128)},
				&net.IPNet{IP: net.ParseIP("fe80::aede:48ff:fe00:1122"), Mask: net.CIDRMask(64, 128)},
				&net.IPNet{IP: net.ParseIP("fe80::18f1:e8fa:6023:2707"), Mask: net.CIDRMask(64, 128)},
				&net.IPNet{IP: net.ParseIP("192.168.13.19"), Mask: net.CIDRMask(112, 128)},
				&net.IPNet{IP: net.ParseIP("192.168.52.87"), Mask: net.CIDRMask(128, 128)},
				&net.IPNet{IP: net.ParseIP("169.254.19.0"), Mask: net.CIDRMask(112, 128)},
			}, nil}, Times: 1},
		}
		af := gomonkey.ApplyFuncSeq(net.InterfaceAddrs, outputs)
		defer af.Reset()

		// nil for mock net.InterfaceAddrs
		convey.So(ip.GetIPSet(), convey.ShouldBeNil)

		// valid for mock net.InterfaceAddrs
		convey.So(ip.GetIPSet(), convey.ShouldResemble, []string{"192.168.13.19", "192.168.52.87"})

	})

}

func TestGetIPSetWithLinkLocalUnicast(t *testing.T) {
	convey.Convey("TestGetIPSetWithLinkLocalUnicast", t, func() {
		outputs := []gomonkey.OutputCell{
			{Values: gomonkey.Params{nil, errors.New("nil")}, Times: 1},
			{Values: gomonkey.Params{[]net.Addr{
				&net.IPNet{IP: net.ParseIP("fe80::1"), Mask: net.CIDRMask(64, 128)},
				&net.IPNet{IP: net.ParseIP("fe80::aede:48ff:fe00:1122"), Mask: net.CIDRMask(64, 128)},
				&net.IPNet{IP: net.ParseIP("fe80::18f1:e8fa:6023:2707"), Mask: net.CIDRMask(64, 128)},
				&net.IPNet{IP: net.ParseIP("192.168.13.19"), Mask: net.CIDRMask(112, 128)},
				&net.IPNet{IP: net.ParseIP("192.168.52.87"), Mask: net.CIDRMask(128, 128)},
				&net.IPNet{IP: net.ParseIP("169.254.19.0"), Mask: net.CIDRMask(112, 128)},
			}, nil}, Times: 1},
		}
		af := gomonkey.ApplyFuncSeq(net.InterfaceAddrs, outputs)
		defer af.Reset()

		// nil for mock net.InterfaceAddrs
		convey.So(ip.GetIPSetWithLinkLocalUnicast(), convey.ShouldBeNil)

		// valid for mock net.InterfaceAddrs
		convey.So(ip.GetIPSetWithLinkLocalUnicast(), convey.ShouldResemble, []string{"fe80::1", "fe80::aede:48ff:fe00:1122", "fe80::18f1:e8fa:6023:2707", "192.168.13.19", "192.168.52.87", "169.254.19.0"})
	})

}

func TestGetIPv4Set(t *testing.T) {
	convey.Convey("TestGetIPv4Set", t, func() {
		outputs := []gomonkey.OutputCell{
			{Values: gomonkey.Params{nil, errors.New("nil")}, Times: 1},
			{Values: gomonkey.Params{[]net.Addr{
				&net.IPNet{IP: net.ParseIP("fe80::1"), Mask: net.CIDRMask(64, 128)},
				&net.IPNet{IP: net.ParseIP("fe80::aede:48ff:fe00:1122"), Mask: net.CIDRMask(64, 128)},
				&net.IPNet{IP: net.ParseIP("fe80::18f1:e8fa:6023:2707"), Mask: net.CIDRMask(64, 128)},
				&net.IPNet{IP: net.ParseIP("192.168.13.19"), Mask: net.CIDRMask(112, 128)},
				&net.IPNet{IP: net.ParseIP("192.168.52.87"), Mask: net.CIDRMask(128, 128)},
				&net.IPNet{IP: net.ParseIP("169.254.19.0"), Mask: net.CIDRMask(112, 128)},
			}, nil}, Times: 1},
		}
		af := gomonkey.ApplyFuncSeq(net.InterfaceAddrs, outputs)
		defer af.Reset()

		// nil for mock net.InterfaceAddrs
		convey.So(ip.GetIPv4Set(), convey.ShouldBeNil)

		// valid for mock net.InterfaceAddrs
		convey.So(ip.GetIPv4Set(), convey.ShouldResemble, []string{"192.168.13.19", "192.168.52.87"})
	})

}

func TestGetIPv6Set(t *testing.T) {
	convey.Convey("TestGetIPv6Set", t, func() {
		outputs := []gomonkey.OutputCell{
			{Values: gomonkey.Params{nil, errors.New("nil")}, Times: 1},
			{Values: gomonkey.Params{[]net.Addr{
				&net.IPNet{IP: net.ParseIP("fe80::1"), Mask: net.CIDRMask(64, 128)},
				&net.IPNet{IP: net.ParseIP("fe80::aede:48ff:fe00:1122"), Mask: net.CIDRMask(64, 128)},
				&net.IPNet{IP: net.ParseIP("fe80::18f1:e8fa:6023:2707"), Mask: net.CIDRMask(64, 128)},
				&net.IPNet{IP: net.ParseIP("192.168.13.19"), Mask: net.CIDRMask(112, 128)},
				&net.IPNet{IP: net.ParseIP("192.168.52.87"), Mask: net.CIDRMask(128, 128)},
				&net.IPNet{IP: net.ParseIP("169.254.19.0"), Mask: net.CIDRMask(112, 128)},
			}, nil}, Times: 1},
		}
		af := gomonkey.ApplyFuncSeq(net.InterfaceAddrs, outputs)
		defer af.Reset()

		// nil for mock net.InterfaceAddrs
		convey.So(ip.GetIPv6Set(), convey.ShouldBeNil)

		// valid for mock net.InterfaceAddrs
		convey.So(ip.GetIPv6Set(), convey.ShouldBeNil)
	})

}

func TestGetLocalIP(t *testing.T) {
	convey.Convey("GetLocalIP", t, func() {
		outputs := []gomonkey.OutputCell{
			{Values: gomonkey.Params{nil, errors.New("nil")}, Times: 1},
			{Values: gomonkey.Params{[]net.Addr{
				&net.IPNet{IP: net.ParseIP("fe80::1"), Mask: net.CIDRMask(64, 128)},
				&net.IPNet{IP: net.ParseIP("fe80::aede:48ff:fe00:1122"), Mask: net.CIDRMask(64, 128)},
				&net.IPNet{IP: net.ParseIP("fe80::18f1:e8fa:6023:2707"), Mask: net.CIDRMask(64, 128)},
				&net.IPNet{IP: net.ParseIP("192.168.13.19"), Mask: net.CIDRMask(112, 128)},
				&net.IPNet{IP: net.ParseIP("192.168.52.87"), Mask: net.CIDRMask(128, 128)},
				&net.IPNet{IP: net.ParseIP("169.254.19.0"), Mask: net.CIDRMask(112, 128)},
			}, nil}, Times: 2},
		}
		af := gomonkey.ApplyFuncSeq(net.InterfaceAddrs, outputs)
		defer af.Reset()

		// nil for mock net.InterfaceAddrs
		convey.So(ip.GetLocalIP(), convey.ShouldEqual, "")

		// valid for mock net.InterfaceAddrs
		convey.So(ip.GetLocalIP(), convey.ShouldEqual, "192.168.13.19")
		convey.So(ip.GetLocalIP(), convey.ShouldEqual, "192.168.13.19")

	})
}

func TestGetLocalIPRealTime(t *testing.T) {
	convey.Convey("GetLocalIPRealTime", t, func() {
		outputs := []gomonkey.OutputCell{
			{Values: gomonkey.Params{nil, errors.New("nil")}, Times: 1},
			{Values: gomonkey.Params{[]net.Addr{
				&net.IPNet{IP: net.ParseIP("fe80::1"), Mask: net.CIDRMask(64, 128)},
				&net.IPNet{IP: net.ParseIP("fe80::aede:48ff:fe00:1122"), Mask: net.CIDRMask(64, 128)},
				&net.IPNet{IP: net.ParseIP("fe80::18f1:e8fa:6023:2707"), Mask: net.CIDRMask(64, 128)},
				&net.IPNet{IP: net.ParseIP("192.168.13.19"), Mask: net.CIDRMask(112, 128)},
				&net.IPNet{IP: net.ParseIP("192.168.52.87"), Mask: net.CIDRMask(128, 128)},
				&net.IPNet{IP: net.ParseIP("169.254.19.0"), Mask: net.CIDRMask(112, 128)},
			}, nil}, Times: 1},
		}
		af := gomonkey.ApplyFuncSeq(net.InterfaceAddrs, outputs)
		defer af.Reset()

		// nil for mock net.InterfaceAddrs
		convey.So(ip.GetLocalIPRealTime(), convey.ShouldEqual, "")

		// valid for mock net.InterfaceAddrs
		convey.So(ip.GetLocalIPRealTime(), convey.ShouldEqual, "192.168.13.19")
	})
}

func TestGetPrivateIP(t *testing.T) {
	convey.Convey("TestGetPrivateIP", t, func() {
		outputs := []gomonkey.OutputCell{
			{Values: gomonkey.Params{nil, errors.New("nil")}, Times: 1},
			{Values: gomonkey.Params{[]net.Addr{
				&net.IPNet{IP: net.ParseIP("fe80::1"), Mask: net.CIDRMask(64, 128)},
				&net.IPNet{IP: net.ParseIP("fe80::aede:48ff:fe00:1122"), Mask: net.CIDRMask(64, 128)},
				&net.IPNet{IP: net.ParseIP("fe80::18f1:e8fa:6023:2707"), Mask: net.CIDRMask(64, 128)},
				&net.IPNet{IP: net.ParseIP("192.168.13.19"), Mask: net.CIDRMask(112, 128)},
				&net.IPNet{IP: net.ParseIP("192.168.52.87"), Mask: net.CIDRMask(128, 128)},
				&net.IPNet{IP: net.ParseIP("169.254.19.0"), Mask: net.CIDRMask(112, 128)},
			}, nil}, Times: 1},
		}
		af := gomonkey.ApplyFuncSeq(net.InterfaceAddrs, outputs)
		defer af.Reset()

		// nil for mock net.InterfaceAddrs
		convey.So(ip.GetPrivateIP(), convey.ShouldEqual, "")

		// valid for mock net.InterfaceAddrs
		convey.So(ip.GetPrivateIP(), convey.ShouldEqual, "192.168.13.19")
	})

}

func TestGetMacAddress(t *testing.T) {
	ip.GetMacAddress()
	localInterfacesJsonStr := `[{"Index":1,"MTU":16384,"Name":"lo0","HardwareAddr":null,"Flags":21},{"Index":2,"MTU":1280,"Name":"gif0","HardwareAddr":null,"Flags":24},{"Index":3,"MTU":1280,"Name":"stf0","HardwareAddr":null,"Flags":0},{"Index":4,"MTU":1500,"Name":"en5","HardwareAddr":"rN5IABEi","Flags":19},{"Index":5,"MTU":1500,"Name":"ap1","HardwareAddr":"8hiYT/yD","Flags":18},{"Index":6,"MTU":1500,"Name":"en0","HardwareAddr":"8BiYT/yD","Flags":19},{"Index":7,"MTU":1500,"Name":"awdl0","HardwareAddr":"YvfUz2Xt","Flags":19},{"Index":8,"MTU":1500,"Name":"llw0","HardwareAddr":"YvfUz2Xt","Flags":19},{"Index":9,"MTU":1500,"Name":"en3","HardwareAddr":"giHPC2QF","Flags":19},{"Index":10,"MTU":1500,"Name":"en4","HardwareAddr":"giHPC2QE","Flags":19},{"Index":11,"MTU":1500,"Name":"en1","HardwareAddr":"giHPC2QB","Flags":19},{"Index":12,"MTU":1500,"Name":"en2","HardwareAddr":"giHPC2QA","Flags":19},{"Index":13,"MTU":1500,"Name":"bridge0","HardwareAddr":"giHPC2QB","Flags":19}]`
	var localInterface []net.Interface
	_ = json.Unmarshal([]byte(localInterfacesJsonStr), &localInterface)

	convey.Convey("TestGetMacAddress", t, func() {
		outputs := []gomonkey.OutputCell{
			{Values: gomonkey.Params{nil, errors.New("nil")}, Times: 1},
			{Values: gomonkey.Params{localInterface, nil}, Times: 1},
		}
		af := gomonkey.ApplyFuncSeq(net.Interfaces, outputs)
		defer af.Reset()

		// nil for mock net.Interfaces
		convey.So(ip.GetMacAddress(), convey.ShouldBeNil)

		// valid for mock net.Interfaces
		convey.So(ip.GetMacAddress(), convey.ShouldResemble, []string{"ac:de:48:00:11:22", "f2:18:98:4f:fc:83", "f0:18:98:4f:fc:83", "62:f7:d4:cf:65:ed", "62:f7:d4:cf:65:ed", "82:21:cf:0b:64:05", "82:21:cf:0b:64:04", "82:21:cf:0b:64:01", "82:21:cf:0b:64:00", "82:21:cf:0b:64:01"})
	})

}

func BenchmarkGetLocalIP(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = ip.GetLocalIP()
	}
}

func BenchmarkGetLocalIPRealTime(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = ip.GetLocalIPRealTime()
	}
}
