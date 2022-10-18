package ip

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

var cacheLocalIP *localIP

type localIP struct {
	IP         net.IP
	LatestTime time.Time
	TTL        time.Duration
}

func init() {
	cacheLocalIP = &localIP{
		IP:         nil,
		LatestTime: time.Now(),
		TTL:        time.Minute,
	}
}

// TypeFlag for ip type
const (
	TypeFlagIsUnspecified = 1 << iota
	TypeFlagIPIsLoopback
	TypeFlagIsPrivate
	TypeFlagIsMulticast
	TypeFlagIsInterfaceLocalMulticast
	TypeFlagIsLinkLocalMulticast
	TypeFlagIsLinkLocalUnicast
	TypeFlagIsGlobalUnicast

	TypeFlagLoopbackANdLinkLocalUnicast = TypeFlagIPIsLoopback | TypeFlagIsLinkLocalUnicast
)

// HeaderContentType header contentType
const (
	HeaderContentTypeApplicationJSON = "application/json"
	HeaderContentTypeTextPlain       = "text/plain"
	HeaderContentTypeTextHTML        = "text/html"
)

// APIListForPublicIP api list for get the public ip.
var APIListForPublicIP = []string{
	"https://ipinfo.io/ip",
	"https://ipinfo.io", // application/json
	"https://ifconfig.me/ip",
	"https://ifconfig.me",
	"https://api.ipify.org",
	"https://api.ipify.org?format=json",
	"https://api64.ipify.org?format=json",
	"https://ident.me",
	"https://ipecho.net/plain",
	"https://ifconfig.co",
	// "https://icanhazip.com", // maybe ipv6 formatted, ignore now
}

func (cacheLocalIP *localIP) LocalIP() (localIP string) {
	if cacheLocalIP == nil || cacheLocalIP.IP == nil || time.Now().After(cacheLocalIP.LatestTime.Add(cacheLocalIP.TTL)) {
		localIP = cacheLocalIP.UpdateCacheLocalIP()
	}
	if cacheLocalIP.IP != nil {
		localIP = cacheLocalIP.IP.String()
	}
	return localIP
}

func (cacheLocalIP *localIP) UpdateCacheLocalIP() (localIP string) {
	if ip := getLocalIPBytes(); ip != nil {
		cacheLocalIP.IP = ip
		cacheLocalIP.LatestTime = time.Now()
		cacheLocalIP.TTL = time.Minute
		localIP = ip.String()
	}
	return localIP
}

// GetIPAll returns all the local IP list with the given params.
//
// Tips:
//  1. flag=4: return only the ipv4.
//  2. flag=6: return only the ipv6.
//  3. others: return the ipv4 or ipv6.
func GetIPAll(flag Flag, ignoreTypeFlag int) (ips []string) {
	if ifa, err := net.InterfaceAddrs(); err == nil {
		for _, adr := range ifa {
			inet, ok := adr.(*net.IPNet)
			if ok {
				if inet.IP.IsUnspecified() && ignoreTypeFlag&TypeFlagIsUnspecified == TypeFlagIsUnspecified ||
					inet.IP.IsLoopback() && ignoreTypeFlag&TypeFlagIPIsLoopback == TypeFlagIPIsLoopback ||
					inet.IP.IsPrivate() && ignoreTypeFlag&TypeFlagIsPrivate == TypeFlagIsPrivate ||
					inet.IP.IsMulticast() && ignoreTypeFlag&TypeFlagIsMulticast == TypeFlagIsMulticast ||
					inet.IP.IsInterfaceLocalMulticast() && ignoreTypeFlag&TypeFlagIsInterfaceLocalMulticast == TypeFlagIsInterfaceLocalMulticast ||
					inet.IP.IsInterfaceLocalMulticast() && ignoreTypeFlag&TypeFlagIsLinkLocalMulticast == TypeFlagIsLinkLocalMulticast ||
					inet.IP.IsLinkLocalUnicast() && ignoreTypeFlag&TypeFlagIsLinkLocalUnicast == TypeFlagIsLinkLocalUnicast ||
					inet.IP.IsGlobalUnicast() && ignoreTypeFlag&TypeFlagIsGlobalUnicast == TypeFlagIsGlobalUnicast {
					continue
				}

				switch flag {
				case FlagV4:
					if inet.IP.To4() != nil {
						ips = append(ips, inet.IP.String())
					}
				case FlagV6:
					if inet.IP.To4() == nil {
						ips = append(ips, inet.IP.String())
					}
				default:
					ips = append(ips, inet.IP.String())
				}
			}
		}
	}
	return ips
}

// GetIPSet returns all the local IP set, but ignore the Loopback and LinkLocalUnicast.
func GetIPSet() (ips []string) {
	return GetIPAll(FlagVAll, TypeFlagLoopbackANdLinkLocalUnicast)
}

// GetIPSetWithLinkLocalUnicast returns all the local IP set, only ignore the Loopback.
func GetIPSetWithLinkLocalUnicast() (ips []string) {
	return GetIPAll(FlagVAll, TypeFlagIPIsLoopback)
}

// GetIPv4Set returns all the local IPv4 set, but ignore the Loopback and LinkLocalUnicast.
func GetIPv4Set() (ips []string) {
	return GetIPAll(FlagV4, TypeFlagLoopbackANdLinkLocalUnicast)
}

// GetIPv6Set returns all the local IPv6 set, but ignore the Loopback and LinkLocalUnicast.
func GetIPv6Set() (ips []string) {
	return GetIPAll(FlagV6, TypeFlagLoopbackANdLinkLocalUnicast)
}

// LocalIPRealTime returns the local ipv4 string realtime, with no cache.
func LocalIPRealTime() (ipv4 string) {
	if ip := getLocalIPBytes(); ip != nil {
		ipv4 = ip.String()
	}
	return
}

// LocalIP returns the local ipv4 string with the local cache.
func LocalIP() (ipv4 string) {
	return cacheLocalIP.LocalIP()
}

// getLocalIPBytes returns the local ipv4 format net.IP
func getLocalIPBytes() (ipv4 net.IP) {
	if ifa, err := net.InterfaceAddrs(); err == nil {
		for _, adr := range ifa {
			inet, ok := adr.(*net.IPNet)
			if ok && !inet.IP.IsLoopback() && !inet.IP.IsLinkLocalUnicast() {
				if ipv4 = inet.IP.To4(); ipv4 != nil {
					break
				}
			}
		}
	}
	return
}

// PrivateIP returns the first local ipv4 format private IP.
func PrivateIP() (ipv4 string) {
	if ifa, err := net.InterfaceAddrs(); err == nil {
		for _, adr := range ifa {
			inet, ok := adr.(*net.IPNet)
			if ok && !inet.IP.IsLoopback() && inet.IP.IsPrivate() {
				if p4 := inet.IP.To4(); p4 != nil {
					ipv4 = p4.String()
					break
				}
			}
		}
	}
	return
}

// PrivateIPAll returns all the local ipv4 format private IP.
func PrivateIPAll() (ipv4s []string) {
	if ifa, err := net.InterfaceAddrs(); err == nil {
		for _, adr := range ifa {
			inet, ok := adr.(*net.IPNet)
			if ok && !inet.IP.IsLoopback() && inet.IP.IsPrivate() {
				if p4 := inet.IP.To4(); p4 != nil {
					ipv4s = append(ipv4s, p4.String())
				}
			}
		}
	}
	return
}

// MacAddress returns all the local mac address.
func MacAddress() (macAddress []string) {
	if netInterfaces, err := net.Interfaces(); err == nil {
		for _, netInterface := range netInterfaces {
			macAddr := netInterface.HardwareAddr.String()
			if len(macAddr) == 0 {
				continue
			}
			macAddress = append(macAddress, macAddr)
		}
	}
	return macAddress
}

// IsPrivate checks whether the ip is private or not.
func IsPrivate(ip net.IP) bool {
	return ip.IsPrivate()
}

// IsPublic checks whether the ip is public or not.
func IsPublic(ip net.IP) bool {
	if ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	return true
}

// PublicIPByHTTPGet returns the public ip by HTTP.Get().
func PublicIPByHTTPGet(url string, printResult bool) (ip string, err error) {
	var response *http.Response
	if len(url) == 0 {
		return
	}

	// #nosec
	if response, err = http.Get(url); err == nil {
		defer func(Body io.ReadCloser) {
			_ = Body.Close()
		}(response.Body)

		if body, err := io.ReadAll(response.Body); err == nil {
			contentType := response.Header.Get("Content-Type")
			if strings.Contains(contentType, HeaderContentTypeTextPlain) {
				ip = string(bytes.Trim(body, " "))
				if printResult {
					fmt.Printf("url:%v, contentType:%v, ret:%v\n", url, contentType, ip)
				}
			} else if strings.Contains(contentType, HeaderContentTypeApplicationJSON) {
				data := make(map[string]interface{})
				err := json.Unmarshal(body, &data)
				if err == nil {
					ip, _ = data["ip"].(string)
				}
				if printResult {
					fmt.Printf("url:%v, contentType:%v, ret:%v\n", url, contentType, data)
				}
			} else if strings.Contains(contentType, HeaderContentTypeTextHTML) {
				ip = string(bytes.Trim(body, " "))
				if printResult {
					fmt.Printf("url:%v, contentType:%v, ret:%v\n", url, contentType, ip)
				}
			}
		}
	}
	return ip, err
}

func getPublicIPByHTTPGet(ret chan string, url string) {
	if ip, err := PublicIPByHTTPGet(url, false); err == nil {
		ret <- ip
	}
}

func getPublicIPMultiChannel(timeout time.Duration, urls []string) (ip string) {
	if len(urls) == 0 {
		return ""
	}

	ips := make(chan string, len(urls))
	tm := time.NewTimer(timeout)
	defer tm.Stop()

	for _, url := range urls {
		go func(url string) {
			getPublicIPByHTTPGet(ips, url)
		}(url)
	}

Loop:
	for {
		select {
		case _ip := <-ips:
			if len(strings.TrimSpace(_ip)) != 0 {
				ip = strings.TrimSpace(_ip)
				break Loop
			}
		case <-tm.C:
			break Loop
		}
	}

	return
}

// PublicIP returns the public ip with your public ip API list.
//
// set the min and max timeout.
// apiListForPublic can ref: APIListForPublicIP
func PublicIP(timeout time.Duration, apiListForPublic ...string) (url string) {
	if timeout <= time.Millisecond*100 {
		timeout = time.Millisecond * 100
	} else if timeout >= time.Second*5 {
		timeout = time.Second * 5
	}

	// most retries
	for i := 0; i < 3 && len(url) == 0; i++ {
		time.Sleep(time.Millisecond * 100)
		url = getPublicIPMultiChannel(timeout, apiListForPublic)
	}
	return
}

// ClientIP implements one best effort algorithm to return the real client IP.
// It trys to parse the headers defined in Request.Header (defaulting to [X-Forwarded-For, X-Real-Ip]).
// If the headers are not syntactically valid, the remote IP (coming form Request.RemoteAddr) is returned.
//
// X-Forwarded-For, examples: <client>, <proxy1>, <proxy2>
func ClientIP(r *http.Request) string {
	if r == nil {
		return ""
	}

	xForwardedFor := r.Header.Get("X-Forwarded-For")
	clientIP := strings.TrimSpace(strings.Split(xForwardedFor, ",")[0])
	if ip := net.ParseIP(clientIP); ip != nil {
		return ip.String()
	}

	clientIP = strings.TrimSpace(r.Header.Get("X-Real-Ip"))
	if ip := net.ParseIP(clientIP); ip != nil {
		return ip.String()
	}

	if clientIP, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr)); err == nil {
		if ip := net.ParseIP(clientIP); ip != nil {
			return ip.String()
		}
	}
	return ""
}

// ClientPublicIP implements one best effort algorithm to return the real client public IP.
// It trys to parse the headers defined in Request.Header (defaulting to [X-Forwarded-For, X-Real-Ip]).
// If the headers are not syntactically valid, the remote IP (coming form Request.RemoteAddr) is returned.
//
// X-Forwarded-For, examples: <client>, <proxy1>, <proxy2>
func ClientPublicIP(r *http.Request) string {
	if r == nil {
		return ""
	}

	var clientIP string
	for _, clientIP = range strings.Split(r.Header.Get("X-Forwarded-For"), ",") {
		clientIP = strings.TrimSpace(clientIP)
		if ip := net.ParseIP(clientIP); ip != nil && IsPublic(ip) {
			return ip.String()
		}
	}

	clientIP = strings.TrimSpace(r.Header.Get("X-Real-Ip"))
	if ip := net.ParseIP(clientIP); ip != nil && IsPublic(ip) {
		return ip.String()
	}

	if clientIP, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr)); err == nil {
		if ip := net.ParseIP(clientIP); ip != nil && IsPublic(ip) {
			return ip.String()
		}
	}
	return ""
}
