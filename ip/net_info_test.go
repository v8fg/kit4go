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

	"github.com/smartystreets/goconvey/convey"

	"github.com/v8fg/kit4go/ip"
)

// withAddrLookup temporarily replaces the package-level DefaultAddrLookup for
// fn (defer-restored) and asserts the mock expectations were met. fn must run
// within a convey.Convey context.
func withAddrLookup(t *testing.T, mockLookup *ip.MockAddrLookup, fn func()) {
	t.Helper()
	orig := ip.DefaultAddrLookup
	ip.DefaultAddrLookup = mockLookup
	defer func() { ip.DefaultAddrLookup = orig }()
	fn()
	if !mockLookup.Mock.AssertExpectations(t) {
		t.Fail()
	}
}

// fakeAddrs is the deterministic address set used by the error-path tests,
// mirroring the original gomonkey mock data so assertions stay stable.
var fakeAddrs = []net.Addr{
	&net.IPNet{IP: net.ParseIP("fe80::1"), Mask: net.CIDRMask(64, 128)},
	&net.IPNet{IP: net.ParseIP("fe80::aede:48ff:fe00:1122"), Mask: net.CIDRMask(64, 128)},
	&net.IPNet{IP: net.ParseIP("fe80::18f1:e8fa:6023:2707"), Mask: net.CIDRMask(64, 128)},
	&net.IPNet{IP: net.ParseIP("192.168.13.19"), Mask: net.CIDRMask(112, 128)},
	&net.IPNet{IP: net.ParseIP("192.168.52.87"), Mask: net.CIDRMask(128, 128)},
	&net.IPNet{IP: net.ParseIP("169.254.19.0"), Mask: net.CIDRMask(112, 128)},
}

func TestGetIPAll(t *testing.T) {
	convey.Convey("TestGetIPAll", t, func() {
		// happy-path: exercise all flag/type combinations without mocking.
		_ = ip.GetIPAll(ip.FlagVAll, ip.TypeFlagIPIsLoopback|ip.TypeFlagIsLinkLocalUnicast)
		_ = ip.GetIPAll(ip.FlagVAll, ip.TypeFlagIPIsLoopback)
		_ = ip.GetIPAll(ip.FlagV4, ip.TypeFlagIPIsLoopback|ip.TypeFlagIsLinkLocalUnicast)
		_ = ip.GetIPAll(ip.FlagV4, ip.TypeFlagIPIsLoopback)
		_ = ip.GetIPAll(ip.FlagV6, ip.TypeFlagIPIsLoopback|ip.TypeFlagIsLinkLocalUnicast)
		_ = ip.GetIPAll(ip.FlagV6, ip.TypeFlagIPIsLoopback)

		// error-path: InterfaceAddrs returns an error -> GetIPAll returns nil.
		convey.Convey("InterfaceAddrsError", func() {
			mockLookup := new(ip.MockAddrLookup)
			mockLookup.EXPECT().InterfaceAddrs().Return(nil, errors.New("nil")).Once()
			withAddrLookup(t, mockLookup, func() {
				convey.So(ip.GetIPAll(ip.FlagVAll, ip.TypeFlagIPIsLoopback|ip.TypeFlagIsLinkLocalUnicast), convey.ShouldBeNil)
			})
		})

		// deterministic happy-path via mocked fake addrs.
		convey.Convey("FakeAddrs", func() {
			mockLookup := new(ip.MockAddrLookup)
			mockLookup.EXPECT().InterfaceAddrs().Return(fakeAddrs, nil).Times(6)
			withAddrLookup(t, mockLookup, func() {
				convey.So(ip.GetIPAll(ip.FlagVAll, ip.TypeFlagIPIsLoopback|ip.TypeFlagIsLinkLocalUnicast), convey.ShouldResemble, []string{"192.168.13.19", "192.168.52.87"})
				convey.So(ip.GetIPAll(ip.FlagVAll, ip.TypeFlagIPIsLoopback), convey.ShouldResemble, []string{"fe80::1", "fe80::aede:48ff:fe00:1122", "fe80::18f1:e8fa:6023:2707", "192.168.13.19", "192.168.52.87", "169.254.19.0"})
				convey.So(ip.GetIPAll(ip.FlagV4, ip.TypeFlagIPIsLoopback|ip.TypeFlagIsLinkLocalUnicast), convey.ShouldResemble, []string{"192.168.13.19", "192.168.52.87"})
				convey.So(ip.GetIPAll(ip.FlagV4, ip.TypeFlagIPIsLoopback), convey.ShouldResemble, []string{"192.168.13.19", "192.168.52.87", "169.254.19.0"})
				convey.So(ip.GetIPAll(ip.FlagV6, ip.TypeFlagIPIsLoopback|ip.TypeFlagIsLinkLocalUnicast), convey.ShouldBeNil)
				convey.So(ip.GetIPAll(ip.FlagV6, ip.TypeFlagIPIsLoopback), convey.ShouldResemble, []string{"fe80::1", "fe80::aede:48ff:fe00:1122", "fe80::18f1:e8fa:6023:2707"})
			})
		})
	})
}

func TestGetIPSet(t *testing.T) {
	convey.Convey("TestGetIPSet", t, func() {
		_ = ip.GetIPSet()

		convey.Convey("ErrorThenFake", func() {
			mockLookup := new(ip.MockAddrLookup)
			mockLookup.EXPECT().InterfaceAddrs().Return(nil, errors.New("nil")).Once()
			mockLookup.EXPECT().InterfaceAddrs().Return(fakeAddrs, nil).Once()
			withAddrLookup(t, mockLookup, func() {
				convey.So(ip.GetIPSet(), convey.ShouldBeNil)
				convey.So(ip.GetIPSet(), convey.ShouldResemble, []string{"192.168.13.19", "192.168.52.87"})
			})
		})
	})
}

func TestGetIPSetWithLinkLocalUnicast(t *testing.T) {
	convey.Convey("TestGetIPSetWithLinkLocalUnicast", t, func() {
		_ = ip.GetIPSetWithLinkLocalUnicast()

		convey.Convey("ErrorThenFake", func() {
			mockLookup := new(ip.MockAddrLookup)
			mockLookup.EXPECT().InterfaceAddrs().Return(nil, errors.New("nil")).Once()
			mockLookup.EXPECT().InterfaceAddrs().Return(fakeAddrs, nil).Once()
			withAddrLookup(t, mockLookup, func() {
				convey.So(ip.GetIPSetWithLinkLocalUnicast(), convey.ShouldBeNil)
				convey.So(ip.GetIPSetWithLinkLocalUnicast(), convey.ShouldResemble, []string{"fe80::1", "fe80::aede:48ff:fe00:1122", "fe80::18f1:e8fa:6023:2707", "192.168.13.19", "192.168.52.87", "169.254.19.0"})
			})
		})
	})
}

func TestGetIPv4Set(t *testing.T) {
	convey.Convey("TestGetIPv4Set", t, func() {
		_ = ip.GetIPv4Set()

		convey.Convey("ErrorThenFake", func() {
			mockLookup := new(ip.MockAddrLookup)
			mockLookup.EXPECT().InterfaceAddrs().Return(nil, errors.New("nil")).Once()
			mockLookup.EXPECT().InterfaceAddrs().Return(fakeAddrs, nil).Once()
			withAddrLookup(t, mockLookup, func() {
				convey.So(ip.GetIPv4Set(), convey.ShouldBeNil)
				convey.So(ip.GetIPv4Set(), convey.ShouldResemble, []string{"192.168.13.19", "192.168.52.87"})
			})
		})
	})
}

func TestGetIPv6Set(t *testing.T) {
	convey.Convey("TestGetIPv6Set", t, func() {
		_ = ip.GetIPv6Set()

		convey.Convey("ErrorThenFake", func() {
			mockLookup := new(ip.MockAddrLookup)
			mockLookup.EXPECT().InterfaceAddrs().Return(nil, errors.New("nil")).Once()
			mockLookup.EXPECT().InterfaceAddrs().Return(fakeAddrs, nil).Once()
			withAddrLookup(t, mockLookup, func() {
				convey.So(ip.GetIPv6Set(), convey.ShouldBeNil)
				convey.So(ip.GetIPv6Set(), convey.ShouldBeNil)
			})
		})
	})
}

func TestLocalIP(t *testing.T) {
	convey.Convey("TestLocalIP", t, func() {
		// LocalIP caches its result in a package-level, process-lifetime
		// cache (cacheLocalIP) that cannot be reset from this external test
		// package. That makes the error-path non-deterministic across
		// -count>1 runs (the cache survives between runs), so we only exercise
		// the happy-path here. The cache-miss lookup itself (getLocalIPBytes)
		// is covered deterministically via TestLocalIPRealTime below, which
		// bypasses the cache.
		_ = ip.LocalIP()
	})
}

func TestLocalIPRealTime(t *testing.T) {
	convey.Convey("TestLocalIPRealTime", t, func() {
		_ = ip.LocalIPRealTime()

		convey.Convey("ErrorThenFake", func() {
			mockLookup := new(ip.MockAddrLookup)
			mockLookup.EXPECT().InterfaceAddrs().Return(nil, errors.New("nil")).Once()
			mockLookup.EXPECT().InterfaceAddrs().Return(fakeAddrs, nil).Once()
			withAddrLookup(t, mockLookup, func() {
				convey.So(ip.LocalIPRealTime(), convey.ShouldEqual, "")
				convey.So(ip.LocalIPRealTime(), convey.ShouldEqual, "192.168.13.19")
			})
		})
	})
}

func TestPrivateIP(t *testing.T) {
	convey.Convey("TestPrivateIP", t, func() {
		_ = ip.PrivateIP()

		convey.Convey("ErrorThenFake", func() {
			mockLookup := new(ip.MockAddrLookup)
			mockLookup.EXPECT().InterfaceAddrs().Return(nil, errors.New("nil")).Once()
			mockLookup.EXPECT().InterfaceAddrs().Return(fakeAddrs, nil).Once()
			withAddrLookup(t, mockLookup, func() {
				convey.So(ip.PrivateIP(), convey.ShouldEqual, "")
				convey.So(ip.PrivateIP(), convey.ShouldEqual, "192.168.13.19")
			})
		})
	})
}

func TestPrivateIPAll(t *testing.T) {
	convey.Convey("PrivateIPAll", t, func() {
		_ = ip.PrivateIPAll()

		convey.Convey("ErrorThenFake", func() {
			mockLookup := new(ip.MockAddrLookup)
			mockLookup.EXPECT().InterfaceAddrs().Return(nil, errors.New("nil")).Once()
			mockLookup.EXPECT().InterfaceAddrs().Return(fakeAddrs, nil).Once()
			withAddrLookup(t, mockLookup, func() {
				convey.So(ip.PrivateIPAll(), convey.ShouldBeNil)
				convey.So(ip.PrivateIPAll(), convey.ShouldResemble, []string{"192.168.13.19", "192.168.52.87"})
			})
		})
	})
}

func TestMacAddress(t *testing.T) {
	// localInterfacesJsonStr kept for reference (mock data).
	localInterfacesJsonStr := `[{"Index":1,"MTU":16384,"Name":"lo0","HardwareAddr":null,"Flags":21},{"Index":2,"MTU":1280,"Name":"gif0","HardwareAddr":null,"Flags":24},{"Index":3,"MTU":1280,"Name":"stf0","HardwareAddr":null,"Flags":0},{"Index":4,"MTU":1500,"Name":"en5","HardwareAddr":"rN5IABEi","Flags":19},{"Index":5,"MTU":1500,"Name":"ap1","HardwareAddr":"8hiYT/yD","Flags":18},{"Index":6,"MTU":1500,"Name":"en0","HardwareAddr":"8BiYT/yD","Flags":19},{"Index":7,"MTU":1500,"Name":"awdl0","HardwareAddr":"YvfUz2Xt","Flags":19},{"Index":8,"MTU":1500,"Name":"llw0","HardwareAddr":"YvfUz2Xt","Flags":19},{"Index":9,"MTU":1500,"Name":"en3","HardwareAddr":"giHPC2QF","Flags":19},{"Index":10,"MTU":1500,"Name":"en4","HardwareAddr":"giHPC2QE","Flags":19},{"Index":11,"MTU":1500,"Name":"en1","HardwareAddr":"giHPC2QB","Flags":19},{"Index":12,"MTU":1500,"Name":"en2","HardwareAddr":"giHPC2QA","Flags":19},{"Index":13,"MTU":1500,"Name":"bridge0","HardwareAddr":"giHPC2QB","Flags":19}]`
	var localInterface []net.Interface
	_ = json.Unmarshal([]byte(localInterfacesJsonStr), &localInterface)

	convey.Convey("TestMacAddress", t, func() {
		_ = ip.MacAddress()

		convey.Convey("ErrorThenFake", func() {
			mockLookup := new(ip.MockAddrLookup)
			mockLookup.EXPECT().Interfaces().Return(nil, errors.New("nil")).Once()
			mockLookup.EXPECT().Interfaces().Return(localInterface, nil).Once()
			withAddrLookup(t, mockLookup, func() {
				convey.So(ip.MacAddress(), convey.ShouldBeNil)
				convey.So(ip.MacAddress(), convey.ShouldResemble, []string{"ac:de:48:00:11:22", "f2:18:98:4f:fc:83", "f0:18:98:4f:fc:83", "62:f7:d4:cf:65:ed", "62:f7:d4:cf:65:ed", "82:21:cf:0b:64:05", "82:21:cf:0b:64:04", "82:21:cf:0b:64:01", "82:21:cf:0b:64:00", "82:21:cf:0b:64:01"})
			})
		})
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

func TestPublicIPByHTTPGet(t *testing.T) {
	// jsonRet := `{"city":"Beijing","country":"CN","hostname":"118.128.147.222.broad.bj.bj.dynamic.163data.com.cn","ip":"220.147.128.110","loc":"39.9075,116.3972","org":"AS4847 China Networks Inter-Exchange","region":"Beijing","timezone":"Asia/Shanghai"}`
	jsonRet := map[string]string{"city": "Beijing", "country": "CN", "hostname": "118.128.147.222.broad.bj.bj.dynamic.163data.com.cn", "ip": "220.147.128.110", "loc": "39.9075,116.3972", "org": "AS4847 China Networks Inter-Exchange", "region": "Beijing", "timezone": "Asia/Shanghai"}
	jsonResp, _ := json.Marshal(jsonRet)

	convey.Convey("TestPublicIPByHTTPGet", t, func() {
		var ipStr string
		var err error

		ts := mockServer(jsonResp, ip.HeaderContentTypeApplicationJSON, 0)
		defer ts.Close()
		ipStr, err = ip.PublicIPByHTTPGet(ts.URL, true)
		convey.So(ipStr, convey.ShouldEqual, "220.147.128.110")
		convey.So(err, convey.ShouldBeNil)

		jsonResp = []byte("49.55.188.188")
		ts = mockServer(jsonResp, ip.HeaderContentTypeTextPlain, 0)
		defer ts.Close()
		ipStr, err = ip.PublicIPByHTTPGet(ts.URL, true)
		convey.So(ipStr, convey.ShouldEqual, "49.55.188.188")
		convey.So(err, convey.ShouldBeNil)

		ts = mockServer(jsonResp, ip.HeaderContentTypeTextHTML, 0)
		defer ts.Close()
		ipStr, err = ip.PublicIPByHTTPGet(ts.URL, true)
		convey.So(ipStr, convey.ShouldEqual, "49.55.188.188")
		convey.So(err, convey.ShouldBeNil)

		ts = mockServer(jsonResp, ip.HeaderContentTypeTextHTML, 0)
		defer ts.Close()
		ipStr, err = ip.PublicIPByHTTPGet("", true)
		convey.So(ipStr, convey.ShouldEqual, "")
		convey.So(err, convey.ShouldBeNil)
	})

}

// sinkCall captures one PublicIPResultSink invocation.
type sinkCall struct {
	url         string
	contentType string
	ret         string
}

// withPublicIPResultSink installs fn as the package-level sink for the duration
// of fn and unconditionally restores the prior value on exit. The default value
// of PublicIPResultSink is nil, so tests using this helper always leave it nil.
func withPublicIPResultSink(fn func(url, contentType, ret string), body func()) {
	orig := ip.PublicIPResultSink
	ip.PublicIPResultSink = fn
	defer func() { ip.PublicIPResultSink = orig }()
	body()
}

// TestPublicIPByHTTPGetResultSink verifies the G2 fix: printResult=true output
// is routed through the optional PublicIPResultSink callback, never fmt.Printf.
func TestPublicIPByHTTPGetResultSink(t *testing.T) {
	jsonRet := map[string]string{"city": "Beijing", "ip": "220.147.128.110"}
	jsonResp, _ := json.Marshal(jsonRet)
	plainResp := []byte("49.55.188.188")

	convey.Convey("TestPublicIPByHTTPGetResultSink", t, func() {
		convey.Convey("sink receives each content type with printResult=true", func() {
			var calls []sinkCall
			rec := func(u, ct, r string) { calls = append(calls, sinkCall{u, ct, r}) }

			ts := mockServer(jsonResp, ip.HeaderContentTypeApplicationJSON, 0)
			defer ts.Close()
			ipStr, err := ip.PublicIPByHTTPGet("", true)
			convey.So(ipStr, convey.ShouldEqual, "")
			convey.So(err, convey.ShouldBeNil)

			withPublicIPResultSink(rec, func() {
				// JSON branch: ret is the marshalled body map.
				ts2 := mockServer(jsonResp, ip.HeaderContentTypeApplicationJSON, 0)
				defer ts2.Close()
				ipStr, err = ip.PublicIPByHTTPGet(ts2.URL, true)
				convey.So(ipStr, convey.ShouldEqual, "220.147.128.110")
				convey.So(err, convey.ShouldBeNil)

				// text/plain branch: ret is the resolved IP string.
				ts3 := mockServer(plainResp, ip.HeaderContentTypeTextPlain, 0)
				defer ts3.Close()
				ipStr, err = ip.PublicIPByHTTPGet(ts3.URL, true)
				convey.So(ipStr, convey.ShouldEqual, "49.55.188.188")
				convey.So(err, convey.ShouldBeNil)

				// text/html branch: ret is the resolved IP string.
				ts4 := mockServer(plainResp, ip.HeaderContentTypeTextHTML, 0)
				defer ts4.Close()
				ipStr, err = ip.PublicIPByHTTPGet(ts4.URL, true)
				convey.So(ipStr, convey.ShouldEqual, "49.55.188.188")
				convey.So(err, convey.ShouldBeNil)
			})

			convey.So(len(calls), convey.ShouldEqual, 3)
			// JSON: ret is the marshalled map containing the ip key.
			convey.So(calls[0].contentType, convey.ShouldContainSubstring, ip.HeaderContentTypeApplicationJSON)
			convey.So(calls[0].ret, convey.ShouldContainSubstring, "220.147.128.110")
			convey.So(calls[0].ret, convey.ShouldContainSubstring, "Beijing")
			// text/plain and text/html: ret is the bare IP.
			convey.So(calls[1].contentType, convey.ShouldContainSubstring, ip.HeaderContentTypeTextPlain)
			convey.So(calls[1].ret, convey.ShouldEqual, "49.55.188.188")
			convey.So(calls[2].contentType, convey.ShouldContainSubstring, ip.HeaderContentTypeTextHTML)
			convey.So(calls[2].ret, convey.ShouldEqual, "49.55.188.188")
		})

		convey.Convey("printResult=false never invokes the sink", func() {
			var calls []sinkCall
			rec := func(u, ct, r string) { calls = append(calls, sinkCall{u, ct, r}) }

			ts := mockServer(plainResp, ip.HeaderContentTypeTextPlain, 0)
			defer ts.Close()
			withPublicIPResultSink(rec, func() {
				ipStr, err := ip.PublicIPByHTTPGet(ts.URL, false)
				convey.So(ipStr, convey.ShouldEqual, "49.55.188.188")
				convey.So(err, convey.ShouldBeNil)
			})
			convey.So(calls, convey.ShouldBeEmpty)
		})

		convey.Convey("nil sink with printResult=true silently skips (default, no stdout)", func() {
			// No sink installed (default nil). Must not panic and must still
			// resolve the IP — this is the backward-compatible default.
			ts := mockServer(plainResp, ip.HeaderContentTypeTextPlain, 0)
			defer ts.Close()
			withPublicIPResultSink(nil, func() {
				ipStr, err := ip.PublicIPByHTTPGet(ts.URL, true)
				convey.So(ipStr, convey.ShouldEqual, "49.55.188.188")
				convey.So(err, convey.ShouldBeNil)
			})
		})
	})
}

func TestPublicIP(t *testing.T) {
	// jsonRet := `{"city":"Beijing","country":"CN","hostname":"118.128.147.222.broad.bj.bj.dynamic.163data.com.cn","ip":"220.147.128.110","loc":"39.9075,116.3972","org":"AS4847 China Networks Inter-Exchange","region":"Beijing","timezone":"Asia/Shanghai"}`
	jsonRet := map[string]string{"city": "Beijing", "country": "CN", "hostname": "118.128.147.222.broad.bj.bj.dynamic.163data.com.cn", "ip": "220.147.128.110", "loc": "39.9075,116.3972", "org": "AS4847 China Networks Inter-Exchange", "region": "Beijing", "timezone": "Asia/Shanghai"}
	jsonResp, _ := json.Marshal(jsonRet)

	convey.Convey("TestPublicIP", t, func() {
		var ipStr string

		ts := mockServer(jsonResp, ip.HeaderContentTypeApplicationJSON, 0)
		defer ts.Close()
		ipStr = ip.PublicIP(0, ts.URL)
		convey.So(ipStr, convey.ShouldEqual, "220.147.128.110")

		ts = mockServer(jsonResp, ip.HeaderContentTypeApplicationJSON, 0)
		defer ts.Close()
		ipStr = ip.PublicIP(time.Millisecond*500, ts.URL)
		convey.So(ipStr, convey.ShouldEqual, "220.147.128.110")

		jsonResp = []byte("49.55.188.188")
		ts = mockServer(jsonResp, ip.HeaderContentTypeTextPlain, 0)
		defer ts.Close()
		ipStr = ip.PublicIP(0, ts.URL)
		convey.So(ipStr, convey.ShouldEqual, "49.55.188.188")

		ts = mockServer(jsonResp, ip.HeaderContentTypeTextPlain, 0)
		defer ts.Close()
		ipStr = ip.PublicIP(time.Millisecond*500, ts.URL)
		convey.So(ipStr, convey.ShouldEqual, "49.55.188.188")

		ts = mockServer(jsonResp, ip.HeaderContentTypeTextPlain, 0)
		defer ts.Close()
		ipStr = ip.PublicIP(0, ts.URL)
		convey.So(ipStr, convey.ShouldEqual, "49.55.188.188")

		ts = mockServer(jsonResp, ip.HeaderContentTypeTextPlain, time.Millisecond*200)
		defer ts.Close()
		ipStr = ip.PublicIP(time.Millisecond*100, ts.URL)
		convey.So(ipStr, convey.ShouldEqual, "")

		ts = mockServer(jsonResp, ip.HeaderContentTypeTextPlain, 0)
		defer ts.Close()
		ipStr = ip.PublicIP(time.Millisecond*500, ts.URL)
		convey.So(ipStr, convey.ShouldEqual, "49.55.188.188")

		ts = mockServer(jsonResp, ip.HeaderContentTypeTextHTML, 0)
		defer ts.Close()
		ipStr = ip.PublicIP(time.Second*6, ts.URL)
		convey.So(ipStr, convey.ShouldEqual, "49.55.188.188")

		ts = mockServer(jsonResp, ip.HeaderContentTypeTextHTML, 0)
		defer ts.Close()
		ipStr = ip.PublicIP(time.Second * 6)
		convey.So(ipStr, convey.ShouldEqual, "")
	})

}

func generateHttpRequestData(xForwardedFor, xRealIP, remoteAddr string) *http.Request {
	headers := http.Header{}
	headers.Set("X-Forwarded-For", xForwardedFor)
	headers.Set("X-Real-Ip", xRealIP)
	return &http.Request{
		Header:     headers,
		RemoteAddr: remoteAddr,
	}
}

func TestClientIP(t *testing.T) {
	convey.Convey("TestClientIP", t, func() {
		convey.So(ip.ClientIP(nil), convey.ShouldEqual, "")
		convey.So(ip.ClientIP(generateHttpRequestData("  20.20.20.20, 30.30.30.30", " 10.10.10.10  ", "  40.40.40.40:42123 ")), convey.ShouldEqual, "20.20.20.20")
		convey.So(ip.ClientIP(generateHttpRequestData("  ", " 10.10.10.10  ", "  40.40.40.40:42123 ")), convey.ShouldEqual, "10.10.10.10")
		convey.So(ip.ClientIP(generateHttpRequestData("", " 10.10.10.10  ", "  40.40.40.40:42123 ")), convey.ShouldEqual, "10.10.10.10")
		convey.So(ip.ClientIP(generateHttpRequestData("30.30.30.30  ", " 10.10.10.10  ", "  40.40.40.40:42123 ")), convey.ShouldEqual, "30.30.30.30")
		convey.So(ip.ClientIP(generateHttpRequestData("", "", "  40.40.40.40:42123 ")), convey.ShouldEqual, "40.40.40.40")
		convey.So(ip.ClientIP(generateHttpRequestData("", "", "50.50.50.50:42123")), convey.ShouldEqual, "50.50.50.50")
		convey.So(ip.ClientIP(generateHttpRequestData("", "", "50.50.50.50")), convey.ShouldEqual, "")
	})

}

func TestClientPublicIP(t *testing.T) {
	convey.Convey("TestClientPublicIP", t, func() {
		convey.So(ip.ClientPublicIP(nil), convey.ShouldEqual, "")
		convey.So(ip.ClientPublicIP(generateHttpRequestData("10.3.5.45, 21.45.9.1", " 10.10.10.10  ", "101.1.0.4:1010")), convey.ShouldEqual, "21.45.9.1")
		convey.So(ip.ClientPublicIP(generateHttpRequestData("101.3.5.45, 21.45.9.1", " 10.10.10.10  ", "101.1.0.4:1010")), convey.ShouldEqual, "101.3.5.45")
		convey.So(ip.ClientPublicIP(generateHttpRequestData("", " 10.10.10.10  ", "101.1.0.4:1010")), convey.ShouldEqual, "101.1.0.4")
		convey.So(ip.ClientPublicIP(generateHttpRequestData("21.45.9.1", "", "101.1.0.4:1010")), convey.ShouldEqual, "21.45.9.1")
		convey.So(ip.ClientPublicIP(generateHttpRequestData("21.45.9.1, ", "", "101.1.0.4:1010")), convey.ShouldEqual, "21.45.9.1")
		convey.So(ip.ClientPublicIP(generateHttpRequestData("192.168.5.45, 210.45.9.1, 89.5.6.1", "", "101.1.0.4:1010")), convey.ShouldEqual, "210.45.9.1")
		convey.So(ip.ClientPublicIP(generateHttpRequestData("192.168.5.45, 172.24.9.1, 89.5.6.1", "", "101.1.0.4:1010")), convey.ShouldEqual, "89.5.6.1")
		convey.So(ip.ClientPublicIP(generateHttpRequestData("192.168.5.45, 172.24.9.1", "", "101.1.0.4:1010")), convey.ShouldEqual, "101.1.0.4")
		convey.So(ip.ClientPublicIP(generateHttpRequestData("192.168.5.45, 172.24.9.1", "", "101.1.0.4:5670")), convey.ShouldEqual, "101.1.0.4")
		convey.So(ip.ClientPublicIP(generateHttpRequestData(" 172.17.40.152, 192.168.5.45", " 10.10.10.10  ", "  40.40.40.40:42123")), convey.ShouldEqual, "40.40.40.40")
		convey.So(ip.ClientPublicIP(generateHttpRequestData(" 172.17.40.152, 192.168.5.45", " 50.50.50.50  ", "  40.40.40.40:42123")), convey.ShouldEqual, "50.50.50.50")
		convey.So(ip.ClientPublicIP(generateHttpRequestData("", "", "  127.0.0.1:42123 ")), convey.ShouldEqual, "")
	})
}

func BenchmarkGetLocalIP(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = ip.LocalIP()
	}
}

func BenchmarkGetLocalIPRealTime(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = ip.LocalIPRealTime()
	}
}
