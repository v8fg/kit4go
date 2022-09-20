package ip

import (
	"bytes"
	"net"
	"strings"
)

// InRange checks whether the ip string is between the start and end ip, the ip shall valid.
func InRange(start, end, ip string) bool {
	ib := ToIP(ip)
	if len(ib) == 0 {
		return false
	}

	if bytes.Compare(ib, ToIP(start)) >= 0 && bytes.Compare(ib, ToIP(end)) <= 0 {
		return true
	}
	return false
}

// InRangeCIDRIP checks whether the ip net.IP is in the cidr ip range, the cidr and ip shall valid.
//
// Slowly than the InRangeIPNet.
func InRangeCIDRIP(cidr string, ip net.IP) bool {
	if len(ip) == 0 || len(cidr) == 0 {
		return false
	}
	valid, _, ipNet := ParseCIDR(cidr)
	return valid.Valid() && ipNet.Contains(ip)
}

// InRangeCIDRStr checks whether the ip string is in the cidr ip range, the cidr and ip shall valid.
//
// Slowly than the InRangeIP and InRange.
func InRangeCIDRStr(cidr, ip string) bool {
	if len(ip) == 0 || len(cidr) == 0 {
		return false
	}
	valid, _, ipNet := ParseCIDR(cidr)
	return valid.Valid() && ipNet.Contains(ToIP(ip))
}

// InRangeIP checks whether the ip net.IP is between the start and end ip, the ip shall valid.
//
// Recommend
func InRangeIP(start, end, ip net.IP) bool {
	if len(ip) == 0 {
		return false
	}

	if bytes.Compare(ip, start) >= 0 && bytes.Compare(ip, end) <= 0 {
		return true
	}
	return false
}

// InRangeIPNet checks whether the ip net.IP is in the subNet net.IPNet range, the subNet and ip shall valid.
//
// Recommend
func InRangeIPNet(subNet *net.IPNet, ip net.IP) bool {
	if len(ip) == 0 || subNet == nil {
		return false
	}
	return subNet.Contains(ip)
}

// InCIDRsOrIPs check whether the ip net.IP is within the given mixed CIDR or IP list.
//
// You shall make sure that the cidrOrIPs are properly formatted.
//
// This method is not recommended if you have high performance requirements.
func InCIDRsOrIPs(cidrOrIPs []string, ip net.IP) (int, bool) {
	for index, cidrOrIP := range cidrOrIPs {
		if strings.Contains(cidrOrIP, "/") {
			valid, _, ipNet := ParseCIDR(cidrOrIP)
			if valid.Valid() && ipNet.Contains(ip) {
				return index, true
			}
		} else {
			if sp := ToIP(cidrOrIP); sp != nil {
				if bytes.Compare(sp, ip) == 0 {
					return index, true
				}
			}
		}
	}
	return -1, false
}

// FastInRangeMixedIPsOrCIDRs in order to carry out IP range filtering more quickly and efficiently,
// IP and CIDR are specially processed.
//
// You shall ensure that the network number and mask are in a minimal format, the input ip is also need minimal.
//
// CIDR IP(IPNet) adds one byte to the low address or high byte (big-endian mode) to identify the network number bits.
//
// Require for your params:
// 	The ip net.IP only support the following ip formats:
//		ipv4:  4bytes
//		ipv6: 16bytes, it must only represent the ipv6
//
// 	The mixedIPsOrCIDRs only support the following formats:
//  	ipv4:      4bytes
//  	ipv4 CIDR: 5bytes, with extra bytes to represents network number bits.
//  	ipv6:      16bytes
//  	ipv6 CIDR: 17bytes, with extra bytes to represents network number bits.
//
//	Warning: all the above ipv6 or ipv6 CIDR, must only represent the ipv6, not for the ipv4 or ipv4 CIDR.
//
// The deals for the mapping mixedIPOrCIDR and ip:
//  diff size 0:
//      ipv4 to ipv4, bytes.Compare
//      ipv6 to ipv6, bytes.Compare
//  diff size others:
//      ipv4 CIDR to ipv4: net ip match
//      ipv6 CIDR to ipv6: net ip match
//      ipv6 CIDR to ipv4: false
//      ipv4 CIDR to ipv6: false
//
// If you use protobuf, [][]byte == repeated bytes.
func FastInRangeMixedIPsOrCIDRs(mixedIPsOrCIDRs [][]byte, ip net.IP) (int, bool) {
	ipLen := len(ip)
	if ipLen != net.IPv4len && ipLen != net.IPv6len {
		return -1, false
	}

	for index, mixedIPOrCIDR := range mixedIPsOrCIDRs {
		lm := len(mixedIPOrCIDR)
		diffSize := lm - ipLen

		if diffSize == 0 {
			if bytes.Compare(mixedIPOrCIDR, ip) == 0 {
				return index, true
			}
			continue
		} else if diffSize == 1 {
			bits := mixedIPOrCIDR[0]
			if int(bits) > 8*ipLen {
				continue
			}

			m := net.CIDRMask(int(bits), 8*ipLen)
			nn := mixedIPOrCIDR[1:] // case 0, you shall ensure that the network number and mask are in a minimal format.
			// nn := net.IP(mixedIPOrCIDR[1:]).Mask(m) // case 1

			// nn := make(net.IP, ipLen)  // case 2
			// for i := 0; i < ipLen; i++ {
			// 	nn[i] = ip[i] & m[i]
			// }

			// if ipLen != len(nn) {
			// 	continue
			// }

			ok := true
			for i := 0; i < ipLen; i++ {
				if nn[i]&m[i] != ip[i]&m[i] {
					ok = false
					break
				}
			}
			if ok {
				return index, true
			}
		}
	}
	return -1, false
}

// CIDRContains checks the ip whether in the given network with prefix length.
//
//  1. checks the ip in the given   IP: shall set ones = len(ipNet)
//  2. checks the ip in the given CIDR: shall set ones = the real mask value
//
//  Default the ip shall simply format, to improve performance, formats like:
// 		ipv4: good
// 		ipv6: good
//  	ipv6: but represents the ipv4, bad
//  To avoid unexpected result, if you can't assure the ip format, always enable formatIPToV4 = true, will work well.
func CIDRContains(ones int, ipNet, ip net.IP, formatIPToV4 bool) bool {
	maskBytesLen := len(ipNet)
	if ones > 8*maskBytesLen || ones < 0 {
		return false
	}

	ipLen := len(ip)
	if ipLen != net.IPv4len && ipLen != net.IPv6len {
		return false
	}
	ipNetLen := len(ipNet)
	if ipNetLen != net.IPv4len && ipNetLen != net.IPv6len {
		return false
	}

	if formatIPToV4 {
		if p4 := ip.To4(); len(p4) == net.IPv4len {
			ipLen = net.IPv4len
			ip = p4
		}
	}

	if p4 := ipNet.To4(); len(p4) == net.IPv4len {
		ipNetLen = net.IPv4len
		ipNet = p4
	}

	// quick compare IP format without mask or len(mask) = len(ipNet)
	if ones == 8*maskBytesLen && ipLen == ipNetLen {
		return bytes.Compare(ipNet, ip) == 0
	}

	m := net.CIDRMask(ones, 8*maskBytesLen) // mask aligns with the ipNet
	if maskBytesLen == net.IPv6len && ipNetLen == net.IPv4len {
		m = m[12:] // again aligns with ip, for the following deals
	}

	// As we can`t assure the ipNet is the wanted network number, so convert it to the corresponding network number.
	nn := ipNet.Mask(m)
	if ipLen != len(nn) {
		return false
	}

	for i := 0; i < ipLen; i++ {
		if nn[i]&m[i] != ip[i]&m[i] {
			return false
		}
	}
	return true
}

// InRangeMixedIPsOrCIDRs in order to carry out IP range filtering more quickly and efficiently,
// IP and CIDR are specially processed.
//
// CIDR IP(IPNet) adds one byte to the low address or high byte (big-endian mode) to identify the network number bits.
//
// The ip net.IP only support the following ip formats:
//	ipv4: 4bytes
//	ipv6: 16bytes, no matter if it represents the ipv4
//
// The mixedIPsOrCIDRs only support the following formats:
//  ipv4:      4bytes or 16bytes(ipv6 format storage)
//  ipv4 CIDR: 5bytes or 17bytes(ipv6 format storage), with extra bytes to represents network number bits.
//  ipv6:      16bytes
//  ipv6 CIDR: 17bytes, with extra bytes to represents network number bits.
//
// Notes:
//  If you have converted ipv4 to ipv6, namely all mixedIPsOrCIDRs storage base ipv6 format,
//  shall make sure the incoming ip net.IP is also the ipv6 format.
//
// Use protobuf, [][]byte == repeated bytes.
func InRangeMixedIPsOrCIDRs(mixedIPsOrCIDRs [][]byte, ip net.IP) (int, bool) {
	ipLen := len(ip)
	if ipLen != net.IPv4len && ipLen != net.IPv6len {
		return -1, false
	}

	// converts the input ip to real formats and length
	if p4 := ip.To4(); len(p4) == net.IPv4len {
		ip = p4
		ipLen = net.IPv4len
	}

	for index, mixedIPOrCIDR := range mixedIPsOrCIDRs {
		lm := len(mixedIPOrCIDR)

		// contains mask byte, if ones = bits = max ipv4/ipv6 bits, bytes.Compare
		if lm == net.IPv4len+1 || lm == net.IPv6len+1 {
			if CIDRContains(int(mixedIPOrCIDR[0]), mixedIPOrCIDR[1:], ip, false) {
				return index, true
			}
		} else {
			if CIDRContains(8*lm, mixedIPOrCIDR[:], ip, false) {
				return index, true
			}
		}

	}
	return -1, false
}
