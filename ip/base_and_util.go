package ip

import (
	"net"
)

const hexDigit = "0123456789abcdef"

// bytesToUint32 with BigEndian
func bytesToUint32(b []byte) (result uint32) {
	n := len(b)
	for i := 0; i < n; i++ {
		result = result << 8
		result += uint32(b[i])
	}
	return result
}

func copyByteFromRight(src []byte, size int) (dst []byte) {
	dst = make([]byte, size)
	sl := len(src)
	if sl >= size {
		dst = src[sl-size:]
	} else {
		for i := 0; i < sl; i++ {
			dst[i+size-sl] = src[i]
		}
	}
	return
}

// ubtoa encodes the string form of the integer v to dst[start:] and
// returns the number of bytes written to dst. The caller must ensure
// that dst has sufficient length.
func ubtoa(dst []byte, start int, v byte) int {
	if v < 10 {
		dst[start] = v + '0'
		return 1
	} else if v < 100 {
		dst[start+1] = v%10 + '0'
		dst[start] = v/10 + '0'
		return 2
	}

	dst[start+2] = v%10 + '0'
	dst[start+1] = (v/10)%10 + '0'
	dst[start] = v/100 + '0'
	return 3
}

func hexString(b []byte) string {
	s := make([]byte, len(b)*2)
	for i, tn := range b {
		s[i*2], s[i*2+1] = hexDigit[tn>>4], hexDigit[tn&0xf]
	}
	return string(s)
}

// Convert i to a hexadecimal string. Leading zeros are not printed.
func appendHex(dst []byte, i uint32) []byte {
	if i == 0 {
		return append(dst, '0')
	}
	for j := 7; j >= 0; j-- {
		v := i >> uint(j*4)
		if v > 0 {
			dst = append(dst, hexDigit[v&0xf])
		}
	}
	return dst
}

// toIPString converts the ip net.IP to the according ip string with the given Flag.
//
//	flag=4 to ipv4 string
//	flag=6 to ipv6 string
//	others to the real IP.String()
func toIPString(ip net.IP, flag Flag) string {
	p := ip

	if len(ip) == 0 {
		return ""
	}

	switch flag {
	case FlagV4:
		return _toIPv4String(p)
	case FlagV6:
		return _toIPv6String(p)
	}

	// if not FlagV4 or FlagV6 will use the same deal as the original.
	if _realIPStr := _toIPv4String(p); len(_realIPStr) != 0 {
		return _realIPStr
	}
	if len(p) != net.IPv6len {
		return "?" + hexString(ip)
	}
	return _toIPv6String(p)
}

// _toIPv4String ref the func IP.String() in the package net/ip.
//
// If IPv4, use dotted notation.
func _toIPv4String(p net.IP) string {
	if p4 := p.To4(); len(p4) == net.IPv4len {
		const maxIPv4StringLen = len("255.255.255.255")
		b := make([]byte, maxIPv4StringLen)

		n := ubtoa(b, 0, p4[0])
		b[n] = '.'
		n++

		n += ubtoa(b, n, p4[1])
		b[n] = '.'
		n++

		n += ubtoa(b, n, p4[2])
		b[n] = '.'
		n++

		n += ubtoa(b, n, p4[3])
		return string(b[:n])
	}
	return ""
}

// _toIPv6String ref the func IP.String() in the package net/ip.
func _toIPv6String(p net.IP) string {
	if len(p) != net.IPv6len {
		return ""
	}

	// Find the longest run of zeros.
	e0 := -1
	e1 := -1
	for i := 0; i < net.IPv6len; i += 2 {
		j := i
		for j < net.IPv6len && p[j] == 0 && p[j+1] == 0 {
			j += 2
		}
		if j > i && j-i > e1-e0 {
			e0 = i
			e1 = j
			i = j
		}
	}
	// The symbol "::" MUST NOT be used to shorten just one 16 bit 0 field.
	if e1-e0 <= 2 {
		e0 = -1
		e1 = -1
	}

	const maxLen = len("ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff")
	b := make([]byte, 0, maxLen)

	// Print with possible :: in place of run of zeros
	for i := 0; i < net.IPv6len; i += 2 {
		if i == e0 {
			b = append(b, ':', ':')
			i = e1
			if i >= net.IPv6len {
				break
			}
		} else if i > 0 {
			b = append(b, ':')
		}
		b = appendHex(b, (uint32(p[i])<<8)|uint32(p[i+1]))
	}
	return string(b)
}
