package ip

import (
	"math/big"
	"net"
	"strconv"
	"strings"
)

// Flag IP Version flag, mark the ip version or invalid.
type Flag int

// FLag mark the ipv4, ipv6, invalid or no specified.
const (
	FlagVInValid = 0 // invalid ip
	FlagV4       = 4
	FlagV6       = 6
)

func (f *Flag) String() string {
	if *f == FlagV4 {
		return "ipv4"
	}
	if *f == FlagV6 {
		return "ipv6"
	}
	if *f == FlagVInValid {
		return "invalid"
	}
	return ""
}

// Valid checks the flag is in v4, v6 or others.
//
//	v4: return true
//	v6: return true
func (f *Flag) Valid() bool {
	return *f == FlagV4 || *f == FlagV6
}

// BytesIPToIPv4Number first converts the ip net.IP to the ipv4, then returns the numeric format.
//
//	Invalid ip: not 4bytes or 16bytes, will output 0.
//
//	Rule: [4]byte for ipv4, [12:]byte for ipv6, then to number.
func BytesIPToIPv4Number(ip net.IP) *big.Int {
	l := len(ip)
	if l != net.IPv4len && l != net.IPv6len {
		return big.NewInt(0)
	}

	if l == net.IPv6len {
		ip = ip[12:]
	}
	return big.NewInt(0).SetBytes(ip)
}

// BytesIPToNumber first converts the ip net.IP to the ipv4, then returns the numeric format.
//
// Invalid ip: not 4bytes or 16bytes, will output 0.
//
// Rule: [4]byte for ipv4, [16]byte for ipv6, then to number.
func BytesIPToNumber(ip net.IP) *big.Int {
	l := len(ip)
	if l != net.IPv4len && l != net.IPv6len {
		return big.NewInt(0)
	}
	return big.NewInt(0).SetBytes(ip)
}

// BytesIPToStr converts the ip net.IP to the corresponding ip string with the given flag.
//
// Flag range and deals:
//
//	flag=4 to ipv4 string, invalid return ""
//	flag=6 to ipv6 string, invalid return ""
//	others to the real IP.String(), invalid may output "" or "?" + hexString(ip)
func BytesIPToStr(ip net.IP, flag Flag) string {
	return toIPString(ip, flag)
}

// BytesIPToStrIPv4 converts the ip net.IP to the corresponding ipv4 string.
//
// If the ip is not a valid ipv4, or ipv6 represents the corresponding ipv4, return "".
func BytesIPToStrIPv4(ip net.IP) string {
	return BytesIPToStr(ip, FlagV4)
}

// BytesIPToStrIPv6 converts the ip net.IP to the corresponding ipv6 string.
//
// If the ip is not a valid ipv6, or ipv6 represents the corresponding ipv4, return "".
func BytesIPToStrIPv6(ip net.IP) string {
	return BytesIPToStr(ip, FlagV6)
}

// IsV4 checks whether the ip string is in valid ipv4 format.
//
// Warning: if the ip is ipv6 format, but represents the ipv4 will treat as ipv4.
func IsV4(ip string) bool {
	return FlagV4 == VersionFlag(ip)
}

// IsV4ByBytesIP checks whether the ip net.IP is in valid ipv4 format.
//
// Warning: if the ip is ipv6 format, but represents the ipv4 will treat as ipv4.
func IsV4ByBytesIP(ip net.IP) bool {
	return FlagV4 == VersionFlagByBytes(ip)
}

// IsV6 checks whether the ip string is in valid ipv6 format.
//
// Warning: if ip is ipv6 format, but represents the ipv4 will treat as ipv4.
func IsV6(ip string) bool {
	return FlagV6 == VersionFlag(ip)
}

// IsV6ByBytesIP checks whether the ip net.IP is in valid ipv6 format.
//
// Warning: if the ip is ipv6 format, but represents the ipv4 will treat as ipv4.
func IsV6ByBytesIP(ip net.IP) bool {
	return FlagV6 == VersionFlagByBytes(ip)
}

// VersionFlag gets the ip version Flag from the ip string and returns a flag within the given range.
//
//	0=FlagVInValid, mark invalid ip
//	4=FlagV4, mark valid ipv4
//	6=FlagV6, mark valid ipv6
//
// Warning: if the ip is ipv6 format, but represents the ipv4 will treat as ipv4.
func VersionFlag(ip string) Flag {
	ib := net.ParseIP(ip)
	if len(ib.To4()) == net.IPv4len {
		return FlagV4
	} else if len(ib.To16()) == net.IPv6len {
		return FlagV6
	}
	return FlagVInValid
}

// VersionFlagByBytes gets the ip version Flag from the ip net.IP and returns a flag within the given range.
//
//	0=FlagVInValid, mark invalid ip
//	4=FlagV4, mark valid ipv4
//	6=FlagV6, mark valid ipv6
//
// Warning: if the ip is ipv6 format, but represents the ipv4 will treat as ipv4.
func VersionFlagByBytes(ip net.IP) Flag {
	if len(ip.To4()) == net.IPv4len {
		return FlagV4
	} else if len(ip.To16()) == net.IPv6len {
		return FlagV6
	}
	return FlagVInValid
}

// VersionFlagByContains gets the ip version Flag from the ip string, using the strings.Contains,
// which returns a flag in the given range.
//
//	0=FlagVInValid, mark invalid ip
//	4=FlagV4, mark valid ipv4
//	6=FlagV6, mark valid ipv6
//
// Warning: if the ip is ipv6 format, but represents the ipv4 will treat as ipv4.
func VersionFlagByContains(ip string) Flag {
	ib := net.ParseIP(ip)
	if ib != nil {
		if strings.Contains(ip, ".") {
			return FlagV4
		}
		if strings.Contains(ip, ":") {
			return FlagV6
		}
	}
	return FlagVInValid
}

// NumberIPv4ToStr converts the ip, numeric ipv4 format to an ipv4 string.
func NumberIPv4ToStr(ip uint32) string {
	ib := make(net.IP, net.IPv4len)
	ib[0] = byte(ip >> 24)
	ib[1] = byte(ip >> 16)
	ib[2] = byte(ip >> 8)
	ib[3] = byte(ip)
	return ib.String()
}

// NumberToIP converts the ip, numeric ip format to net.IP.
//
// If the numeric ip`s bytes size over the limit return nil.
//
// Limits: ip`s bytes size
//
//	flag=4, <=16, ipv4 maybe represents with ipv6 format.
//	flag=6, <=16
//	others, <=16
//
// Flag to deal:
//
//	flag=4 to [4]byte
//	flag=6 to [16]byte
//	others to the nearest bytes size format.
func NumberToIP(ip *big.Int, flag Flag) net.IP {
	ib := ip.Bytes()
	l := len(ib)

	if l > net.IPv6len {
		return nil
	}

	switch flag {
	case FlagV4:
		ib = copyByteFromRight(ib, net.IPv4len)
	case FlagV6:
		ib = copyByteFromRight(ib, net.IPv6len)
	default:
		if l <= net.IPv4len {
			ib = copyByteFromRight(ib, net.IPv4len)
		} else if l <= net.IPv6len {
			ib = copyByteFromRight(ib, net.IPv6len)
		}
	}
	return ib
}

// NumberToIPv4 converts the ip, numeric ip format to net.IP.
func NumberToIPv4(ip *big.Int) net.IP {
	return NumberToIP(ip, FlagV4)
}

// NumberToIPv6 converts the ip, numeric ip format ip to net.IP.
func NumberToIPv6(ip *big.Int) net.IP {
	return NumberToIP(ip, FlagV6)
}

// TextNumberToIPStr converts the numeric number format ip with the given base, to the corresponding string ip.
//
// The base constraints: base < 2 || base > 62
//
// Warning: the num shall with the same prefix as the base.
func TextNumberToIPStr(num string, base int) string {
	if base < 2 || base > big.MaxBase {
		return ""
	}

	if ib, ok := big.NewInt(0).SetString(num, base); ok {
		return NumberToIP(ib, 0).String()
	}

	return ""
}

// ToIP converts the ip string, to net.IP.
//
// If the ip is valid, the corresponding net.IP is returned:
//
//	ipv4: [16]byte, fill the prefix: v4InV6Prefix = []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff}
//	ipv6: [16]byte
//
// else return nil.
//
// Tips: as ipv4 and ipv6 both to [16]byte, the real ipv4 bytes shall bytes[12:].
func ToIP(ip string) net.IP {
	return net.ParseIP(ip)
}

// ToIPReal converts the ip string to the real net.IP, ipv4 to [4]byte and ipv6 to [16]byte
//
// If the ip is valid, the corresponding net.IP is returned, else return nil.
func ToIPReal(ip string) (ret net.IP) {
	ib := net.ParseIP(ip)

	ret = ib.To4()
	if len(ret) == net.IPv4len {
		return ret
	}

	ret = ib.To16()
	if len(ret) == net.IPv6len {
		return ret
	}
	return ret
}

// ToNumber converts the ip string to the numeric format.
//
// If the ip is valid, the corresponding number is returned, else return 0.
//
//	ipv4 to the corresponding ipv4 numeric format
//	ipv6 to the corresponding ipv6 numeric format
//	others to 0
//
// Tips: ipv6 may overflow uint64, so return big.Int.
func ToNumber(ip string) *big.Int {
	ib := ToIPReal(ip)
	if len(ib) == 0 {
		return big.NewInt(0)
	}
	return big.NewInt(0).SetBytes(ib)
}

// ToIPv4Number coverts the ip string to the corresponding ipv4 numeric format.
//
// If the ip is valid, the corresponding number is returned, else return 0.
//
//	ipv4 to the corresponding ipv4 numeric format
//	ipv6, if presents the ipv4 to the corresponding ipv4 numeric format
func ToIPv4Number(ip string) uint32 {
	ib := ToIPReal(ip)
	if len(ib) == 0 {
		return 0
	}

	if len(ib) == net.IPv4len {
		return bytesToUint32(ib)
	}
	return 0
}

// ToNumberIPv4 coverts the ip string to the numeric format.
//
// If the ip is valid, the corresponding number is returned, else return 0.
//
//	ipv4 to the corresponding ipv4 numeric format
//	ipv6 remain the last 4bytes, namely deal [12:], then convert to the corresponding numeric format
//	others to 0
//
// Warning: if the ip is valid ipv6 string, will miss the overflow parts.
func ToNumberIPv4(ip string) uint32 {
	ib := ToIPReal(ip)
	if len(ib) == 0 {
		return 0
	}

	if len(ib) == net.IPv4len {
		return bytesToUint32(ib)
	}
	return bytesToUint32(ib[12:])
}

// ToCIDRStr converts the fuzzyIPV4 to cidr format.
//
// The support fuzzy formats like, need least one separate character ".\d{1,3}|.*":
//
//	 	192.168.1.1         -> 192.168.1.1/32
//	 	192.168.1.*			-> 192.168.1.0/24
//	 	192.168.*.*			-> 192.168.0.0/16
//	 	192.*.*.*			-> 192.0.0.0/8
//			192.*.*				-> 192.0.0.0/8
//			192.*				-> 192.0.0.0/8
//			*.*					-> 0.0.0.0/0
//			*.*.*				-> 0.0.0.0/0
//			*.*.*.*				-> 0.0.0.0/0
//	 We set the first character '*' index as the ip number`s mask bits and replace all the "*" to "0".
func ToCIDRStr(fuzzyIPV4 string) string {
	if len(fuzzyIPV4) == 0 {
		return ""
	}

	fIps := strings.Split(fuzzyIPV4, ".")
	if len(fIps) <= 1 || len(fIps) > 4 {
		return ""
	}

	fuzzyIPV4 += strings.Repeat(".0", 4-len(fIps))

	ones := 32
	pos := -1 // no character *

	// check valid
	fIps = strings.Split(fuzzyIPV4, ".")
	for index, fIP := range fIps {
		if fIP != "*" {
			ones = (index + 1) * 8
			parseInt, err := strconv.ParseUint(fIP, 10, 32)
			if err != nil || parseInt > 255 {
				return ""
			}
		} else {
			pos = index
			break
		}
	}

	if pos >= 0 {
		fuzzyIPV4 = strings.ReplaceAll(fuzzyIPV4, "*", "0")
		ones = 8 * pos
	}

	return fuzzyIPV4 + "/" + strconv.FormatInt(int64(ones), 10)
}

// ToStrIP converts the ip string to the corresponding ip string with the given flag.
//
// Flag range and deals:
//
//	flag=4 to ipv4 string
//	flag=6 to ipv6 string
//	others to the real IP.String()
func ToStrIP(ip string, flag Flag) string {
	ib := net.ParseIP(ip)
	return toIPString(ib, flag)
}

// ToStrIPv4 converts the ip string to the corresponding ipv4 string.
//
// output:
//
//	empty string: invalid ipv4 or ipv6 input, the valid ipv6 not represents the ipv4.
//	 ipv4 string: ipv4 or ipv6 str represents the ipv4.
func ToStrIPv4(ip string) string {
	return ToStrIP(ip, FlagV4)
}

// ToStrIPv6 converts the ip string to the corresponding ipv6 string.
//
// output:
//
//	empty string: invalid ipv4 or ipv6 input.
//	 ipv6 string: ipv6, ipv4 or ipv6 string represents the ipv4.
func ToStrIPv6(ip string) string {
	return ToStrIP(ip, FlagV6)
}

// ToTextIPv4Number first converts the ip to ipv4 number, then returns the corresponding numeric text with the given base.
//
// The base constraints: base < 2 || base > 62
func ToTextIPv4Number(ip string, base int) string {
	if base < 2 || base > big.MaxBase {
		return ""
	}
	ipB := ToIP(ip) // nil or [16]byte
	if len(ipB) == 0 {
		return ""
	}

	return big.NewInt(0).SetBytes(ipB[12:]).Text(base)
}

// ToTextIPv4NumberDecimal first converts the ip string to ipv4 number, then returns the corresponding text with the base 10.
func ToTextIPv4NumberDecimal(ip string) string {
	return ToTextIPv4Number(ip, 10)
}

// ToTextNumber converts the ip to the corresponding numeric text with the given base, if invalid return "".
//
// The base constraints: base < 2 || base > 62
func ToTextNumber(ip string, base int) string {
	if base < 2 || base > big.MaxBase {
		return ""
	}
	ipB := ToIPReal(ip)
	if len(ipB) == 0 {
		return ""
	}

	return big.NewInt(0).SetBytes(ipB).Text(base)
}

// ToTextNumberDecimal converts the ip to the corresponding numeric text with base 10, if invalid return "".
func ToTextNumberDecimal(ip string) string {
	return ToTextNumber(ip, 10)
}
