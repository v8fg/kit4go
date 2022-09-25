package ip_test

import (
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
		convey.So(ip.GetIPAll(ip.FlagVAll, ip.TypeFlagIPIsLoopback|ip.TypeFlagIsLinkLocalUnicast), convey.ShouldBeNil)
		convey.So(ip.GetIPAll(ip.FlagVAll, ip.TypeFlagIPIsLoopback), convey.ShouldBeNil)
		convey.So(ip.GetIPAll(ip.FlagV4, ip.TypeFlagIPIsLoopback|ip.TypeFlagIsLinkLocalUnicast), convey.ShouldBeNil)
		convey.So(ip.GetIPAll(ip.FlagV4, ip.TypeFlagIPIsLoopback), convey.ShouldBeNil)
		convey.So(ip.GetIPAll(ip.FlagV6, ip.TypeFlagIPIsLoopback|ip.TypeFlagIsLinkLocalUnicast), convey.ShouldBeNil)
		convey.So(ip.GetIPAll(ip.FlagV6, ip.TypeFlagIPIsLoopback), convey.ShouldBeNil)

		// valid for mock net.InterfaceAddrs
		convey.So(ip.GetIPAll(ip.FlagVAll, ip.TypeFlagIPIsLoopback|ip.TypeFlagIsLinkLocalUnicast), convey.ShouldResemble, []string{"192.168.13.19", "192.168.52.87"})
		convey.So(ip.GetIPAll(ip.FlagVAll, ip.TypeFlagIPIsLoopback), convey.ShouldResemble, []string{"fe80::1", "fe80::aede:48ff:fe00:1122", "fe80::18f1:e8fa:6023:2707", "192.168.13.19", "192.168.52.87", "169.254.19.0"})
		convey.So(ip.GetIPAll(ip.FlagV4, ip.TypeFlagIPIsLoopback|ip.TypeFlagIsLinkLocalUnicast), convey.ShouldResemble, []string{"192.168.13.19", "192.168.52.87"})
		convey.So(ip.GetIPAll(ip.FlagV4, ip.TypeFlagIPIsLoopback), convey.ShouldResemble, []string{"192.168.13.19", "192.168.52.87", "169.254.19.0"})
		convey.So(ip.GetIPAll(ip.FlagV6, ip.TypeFlagIPIsLoopback|ip.TypeFlagIsLinkLocalUnicast), convey.ShouldBeNil)
		convey.So(ip.GetIPAll(ip.FlagV6, ip.TypeFlagIPIsLoopback), convey.ShouldResemble, []string{"fe80::1", "fe80::aede:48ff:fe00:1122", "fe80::18f1:e8fa:6023:2707"})

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

func TestGetPrivateIPAll(t *testing.T) {
	convey.Convey("GetPrivateIPAll", t, func() {
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
		convey.So(ip.GetPrivateIPAll(), convey.ShouldBeNil)

		// valid for mock net.InterfaceAddrs
		convey.So(ip.GetPrivateIPAll(), convey.ShouldResemble, []string{"192.168.13.19", "192.168.52.87"})
	})
}

func TestGetMacAddress(t *testing.T) {
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

func TestIsPrivateIP(t *testing.T) {
	convey.Convey("TestIsPrivateIP", t, func() {
		convey.So(ip.IsPrivate(net.IPv4(10, 0, 0, 0)), convey.ShouldBeTrue)
		convey.So(ip.IsPrivate(net.IPv4(172, 16, 0, 0)), convey.ShouldBeTrue)
		convey.So(ip.IsPrivate(net.IPv4(192, 168, 0, 0)), convey.ShouldBeTrue)
		convey.So(ip.IsPrivate(net.IPv4(199, 168, 1, 1)), convey.ShouldBeFalse)
		convey.So(ip.IsPrivate(net.IP{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}), convey.ShouldBeFalse) // IPv6loopback
	})
}

func TestIsPublicIP(t *testing.T) {
	convey.Convey("TestIsPublicIP", t, func() {
		convey.So(ip.IsPublic(net.IPv4(10, 0, 0, 0)), convey.ShouldBeFalse)
		convey.So(ip.IsPublic(net.IPv4(172, 16, 0, 0)), convey.ShouldBeFalse)
		convey.So(ip.IsPublic(net.IPv4(192, 168, 0, 0)), convey.ShouldBeFalse)
		convey.So(ip.IsPublic(net.IPv4(199, 168, 1, 1)), convey.ShouldBeTrue)
		convey.So(ip.IsPublic(net.IP{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}), convey.ShouldBeFalse) // IPv6loopback
	})
}

func mockServer(response []byte, contentType string, sleep time.Duration) *httptest.Server {
	testHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(http.StatusOK) // must after the Header.Set()

		if sleep <= time.Millisecond {
			sleep = time.Millisecond
		}
		time.Sleep(sleep) // to test the timeout
		_, err := w.Write(response)
		if err != nil {
			log.Printf("mockServer response err")
		}
	}
	return httptest.NewServer(http.HandlerFunc(testHandler))
}

func TestGetPublicIPByHTTPGet(t *testing.T) {
	// jsonRet := `{"city":"Beijing","country":"CN","hostname":"118.128.147.222.broad.bj.bj.dynamic.163data.com.cn","ip":"220.147.128.110","loc":"39.9075,116.3972","org":"AS4847 China Networks Inter-Exchange","region":"Beijing","timezone":"Asia/Shanghai"}`
	jsonRet := map[string]string{"city": "Beijing", "country": "CN", "hostname": "118.128.147.222.broad.bj.bj.dynamic.163data.com.cn", "ip": "220.147.128.110", "loc": "39.9075,116.3972", "org": "AS4847 China Networks Inter-Exchange", "region": "Beijing", "timezone": "Asia/Shanghai"}
	jsonResp, _ := json.Marshal(jsonRet)

	convey.Convey("GetPublicIPByHTTPGet", t, func() {
		var ipStr string
		var err error

		ts := mockServer(jsonResp, ip.HeaderContentTypeApplicationJson, 0)
		defer ts.Close()
		ipStr, err = ip.GetPublicIPByHTTPGet(ts.URL, true)
		convey.So(ipStr, convey.ShouldEqual, "220.147.128.110")
		convey.So(err, convey.ShouldBeNil)

		jsonResp = []byte("49.55.188.188")
		ts = mockServer(jsonResp, ip.HeaderContentTypeTextPlain, 0)
		defer ts.Close()
		ipStr, err = ip.GetPublicIPByHTTPGet(ts.URL, true)
		convey.So(ipStr, convey.ShouldEqual, "49.55.188.188")
		convey.So(err, convey.ShouldBeNil)

		ts = mockServer(jsonResp, ip.HeaderContentTypeTextHtml, 0)
		defer ts.Close()
		ipStr, err = ip.GetPublicIPByHTTPGet(ts.URL, true)
		convey.So(ipStr, convey.ShouldEqual, "49.55.188.188")
		convey.So(err, convey.ShouldBeNil)

		ts = mockServer(jsonResp, ip.HeaderContentTypeTextHtml, 0)
		defer ts.Close()
		ipStr, err = ip.GetPublicIPByHTTPGet("", true)
		convey.So(ipStr, convey.ShouldEqual, "")
		convey.So(err, convey.ShouldBeNil)
	})

}

func TestGetPublicIP(t *testing.T) {
	// jsonRet := `{"city":"Beijing","country":"CN","hostname":"118.128.147.222.broad.bj.bj.dynamic.163data.com.cn","ip":"220.147.128.110","loc":"39.9075,116.3972","org":"AS4847 China Networks Inter-Exchange","region":"Beijing","timezone":"Asia/Shanghai"}`
	jsonRet := map[string]string{"city": "Beijing", "country": "CN", "hostname": "118.128.147.222.broad.bj.bj.dynamic.163data.com.cn", "ip": "220.147.128.110", "loc": "39.9075,116.3972", "org": "AS4847 China Networks Inter-Exchange", "region": "Beijing", "timezone": "Asia/Shanghai"}
	jsonResp, _ := json.Marshal(jsonRet)

	convey.Convey("GetPublicIP", t, func() {
		var ipStr string

		ts := mockServer(jsonResp, ip.HeaderContentTypeApplicationJson, 0)
		defer ts.Close()
		ipStr = ip.GetPublicIP(0, ts.URL)
		convey.So(ipStr, convey.ShouldEqual, "220.147.128.110")

		ts = mockServer(jsonResp, ip.HeaderContentTypeApplicationJson, 0)
		defer ts.Close()
		ipStr = ip.GetPublicIP(time.Millisecond*500, ts.URL)
		convey.So(ipStr, convey.ShouldEqual, "220.147.128.110")

		jsonResp = []byte("49.55.188.188")
		ts = mockServer(jsonResp, ip.HeaderContentTypeTextPlain, 0)
		defer ts.Close()
		ipStr = ip.GetPublicIP(0, ts.URL)
		convey.So(ipStr, convey.ShouldEqual, "49.55.188.188")

		ts = mockServer(jsonResp, ip.HeaderContentTypeTextPlain, 0)
		defer ts.Close()
		ipStr = ip.GetPublicIP(time.Millisecond*500, ts.URL)
		convey.So(ipStr, convey.ShouldEqual, "49.55.188.188")

		ts = mockServer(jsonResp, ip.HeaderContentTypeTextHtml, 0)
		defer ts.Close()
		ipStr = ip.GetPublicIP(0, ts.URL)
		convey.So(ipStr, convey.ShouldEqual, "49.55.188.188")

		ts = mockServer(jsonResp, ip.HeaderContentTypeTextHtml, time.Millisecond*150)
		defer ts.Close()
		ipStr = ip.GetPublicIP(time.Millisecond*100, ts.URL)
		convey.So(ipStr, convey.ShouldEqual, "")

		ts = mockServer(jsonResp, ip.HeaderContentTypeTextHtml, 0)
		defer ts.Close()
		ipStr = ip.GetPublicIP(time.Millisecond*500, ts.URL)
		convey.So(ipStr, convey.ShouldEqual, "49.55.188.188")

		ts = mockServer(jsonResp, ip.HeaderContentTypeTextHtml, 0)
		defer ts.Close()
		ipStr = ip.GetPublicIP(time.Second*6, ts.URL)
		convey.So(ipStr, convey.ShouldEqual, "49.55.188.188")

		ts = mockServer(jsonResp, ip.HeaderContentTypeTextHtml, 0)
		defer ts.Close()
		ipStr = ip.GetPublicIP(time.Second * 6)
		convey.So(ipStr, convey.ShouldEqual, "")
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
