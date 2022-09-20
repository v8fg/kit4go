package ip

import (
	"net"
	"time"
)

var cacheLocalIP *localIP

type localIP struct {
	IP         net.IP
	LatestTime time.Time
	TTl        time.Duration
}

func init() {
	cacheLocalIP = &localIP{
		IP:         nil,
		LatestTime: time.Now(),
		TTl:        time.Minute,
	}
}

func (cacheLocalIP *localIP) GetLocalIP() (localIP string) {
	if cacheLocalIP == nil || cacheLocalIP.IP == nil || time.Now().After(cacheLocalIP.LatestTime.Add(cacheLocalIP.TTl)) {
		localIP = cacheLocalIP.UpdateCacheLocalIP()
	}
	if cacheLocalIP.IP == nil {
		return ""
	}
	return cacheLocalIP.IP.String()
}

func (cacheLocalIP *localIP) UpdateCacheLocalIP() (localIP string) {
	if ip := getLocalIPBytes(); ip != nil {
		cacheLocalIP.IP = ip
		cacheLocalIP.LatestTime = time.Now()
		cacheLocalIP.TTl = time.Minute
		localIP = ip.String()
	}
	return localIP
}

func GetIPAll(flag Flag, ignoreLinkLocalUnicast bool) (ips []string) {
	if ifa, err := net.InterfaceAddrs(); err == nil {
		for _, adr := range ifa {
			inet, ok := adr.(*net.IPNet)
			if ok && !inet.IP.IsLoopback() {
				if ignoreLinkLocalUnicast && inet.IP.IsLinkLocalUnicast() {
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

// GetIPSet get all the local IP set, but ignore the Loopback and LinkLocalUnicast.
func GetIPSet() (ips []string) {
	return GetIPAll(FlagVInValid, true)
}

// GetIPSetWithLinkLocalUnicast get all the local IP set, only ignore the Loopback.
func GetIPSetWithLinkLocalUnicast() (ips []string) {
	return GetIPAll(FlagVInValid, false)
}

// GetIPv4Set get all the local IPv4 set, but ignore the Loopback and LinkLocalUnicast.
func GetIPv4Set() (ips []string) {
	return GetIPAll(FlagV4, true)
}

// GetIPv6Set get all the local IPv6 set, but ignore the Loopback and LinkLocalUnicast.
func GetIPv6Set() (ips []string) {
	return GetIPAll(FlagV6, true)
}

// GetLocalIPRealTime get the local ipv4 string realtime, with no cache.
func GetLocalIPRealTime() (ipv4 string) {
	if ip := getLocalIPBytes(); ip != nil {
		ipv4 = ip.String()
	}
	return
}

// GetLocalIP get the local ipv4 string with the local cache.
func GetLocalIP() (ipv4 string) {
	return cacheLocalIP.GetLocalIP()
}

// getLocalIPBytes get the local ipv4 format net.IP
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

// GetPrivateIP get the local ipv4 format private IP.
func GetPrivateIP() (ipv4 string) {
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

func GetMacAddress() (macAddress []string) {
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
