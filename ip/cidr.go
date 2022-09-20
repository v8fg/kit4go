package ip

import (
	"encoding/hex"
	"net"
	"strconv"
	"strings"
)

// ParseCIDR parses cidr as a CIDR notation IP address and prefix length,
// like "192.0.2.0/24" or "2001:db8::/32", as defined in
// RFC 4632 and RFC 4291.
//
// It returns the IP flag, address and the network implied by the IP and
// prefix length.
// For example, ParseCIDR("192.0.2.1/24") returns the IP version flag ipv4, address
// 192.0.2.1 and the network 192.0.2.0/24.
func ParseCIDR(cidr string) (ipVersionFlag Flag, ipAddr string, ipNet *net.IPNet) {
	ipVersionFlag = FlagVInValid
	_ip, ipNet, err := net.ParseCIDR(cidr)

	if err != nil {
		return
	}

	ipAddr = _ip.String()
	if len(_ip.To4()) == net.IPv4len {
		ipVersionFlag = FlagV4
	} else if len(_ip.To16()) == net.IPv6len {
		ipVersionFlag = FlagV6
	}
	return
}

// MaskByte gets the cidr`s mask.
func MaskByte(cidr string) []byte {
	_, _, ipNet := ParseCIDR(cidr)
	if ipNet == nil {
		return []byte{}
	}
	return ipNet.Mask
}

// MaskString gets the cidr`s mask string format.
func MaskString(cidr string) string {
	_, _, ipNet := ParseCIDR(cidr)
	if ipNet == nil {
		return ""
	}
	return ipNet.Mask.String()
}

// CIDRToIPMask gets the cidr`s ip/mask string format.
//
// If cidr ipv6 format, but represent the ipv4, mask string will be ipv6 format.
func CIDRToIPMask(cidr string) string {
	flag, ip, ipNet := ParseCIDR(cidr)
	if len(ip) == 0 || ipNet == nil {
		return ""
	}

	if flag == FlagV4 && len(ipNet.Mask) == net.IPv6len {
		return ip + "/" + ipNet.Mask[12:].String()
	}

	return ip + "/" + ipNet.Mask.String()
}

// MaskIPToCIDR parses the ipMask with format ip/mask as CIDR string format.
func MaskIPToCIDR(ipMask string) string {
	im := strings.Split(ipMask, "/")
	if len(im) != 2 {
		return ""
	}

	bits := len(im[1])
	if bits != 8 && bits != 32 {
		return ""
	}

	var ones uint8
	decodeS, _ := hex.DecodeString(im[1])

	for i := 0; i < len(decodeS); i++ {
		ones += hammingWeight(decodeS[i])
	}

	return im[0] + "/" + strconv.FormatUint(uint64(ones), 10)
}

func hammingWeight(num uint8) (ones uint8) {
	for ; num > 0; num &= num - 1 {
		ones++
	}
	return
}
