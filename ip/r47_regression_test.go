package ip_test

import (
	"math/big"
	"net"
	"testing"

	"github.com/v8fg/kit4go/ip"
)

// TestR47_MaskIPToCIDR_RejectsNonCanonical: a non-contiguous mask (a host bit
// set) is not a valid CIDR mask and must be rejected, not silently mapped to a
// fabricated prefix length — otherwise an allowlist built from the literal mask
// bytes over-permits.
func TestR47_MaskIPToCIDR_RejectsNonCanonical(t *testing.T) {
	for _, c := range []string{
		"10.0.0.0/ffffff01", // 255.255.255.1 — host bit set
		"10.0.0.0/80808080", // non-contiguous
		"10.0.0.0/01000000",
	} {
		if got := ip.MaskIPToCIDR(c); got != "" {
			t.Errorf("MaskIPToCIDR(%q) = %q, want \"\" (non-canonical mask rejected)", c, got)
		}
	}
	// Canonical masks still map correctly.
	if got := ip.MaskIPToCIDR("10.0.0.0/ffffff80"); got != "10.0.0.0/25" {
		t.Errorf("MaskIPToCIDR(canonical /25) = %q, want 10.0.0.0/25", got)
	}
}

// TestR47_IsPublic_RejectsUnspecifiedAndBroadcast: the unspecified address
// (0.0.0.0 / ::), the IPv4 limited broadcast (255.255.255.255), and multicast
// are not public — without this, ClientPublicIP would return 0.0.0.0 from a
// forged X-Forwarded-For.
func TestR47_IsPublic_RejectsUnspecifiedAndBroadcast(t *testing.T) {
	for _, addr := range []net.IP{net.IPv4zero, net.IPv4bcast, net.IPv6zero, net.ParseIP("224.0.0.1")} {
		if ip.IsPublic(addr) {
			t.Errorf("IsPublic(%v) = true, want false (non-routable)", addr)
		}
	}
	if !ip.IsPublic(net.ParseIP("8.8.8.8")) {
		t.Error("IsPublic(8.8.8.8) = false, want true")
	}
}

// TestR47_NumberToIP_RejectsNegativeAndOverWidth: a negative number must not
// silently become a nonsense address (big.Int.Bytes returns the absolute value,
// dropping the sign); an over-width IPv4 number must not silently truncate.
func TestR47_NumberToIP_RejectsNegativeAndOverWidth(t *testing.T) {
	if got := ip.NumberToIPv4(big.NewInt(-1)); got != nil {
		t.Errorf("NumberToIPv4(-1) = %v, want nil (negative rejected)", got)
	}
	if got := ip.NumberToIPv4(new(big.Int).SetBytes([]byte{1, 0, 0, 0, 0, 0})); got != nil {
		t.Errorf("NumberToIPv4(over-width) = %v, want nil (does not fit in IPv4)", got)
	}
	if got := ip.NumberToIPv4(big.NewInt(16909060)).String(); got != "1.2.3.4" {
		t.Errorf("NumberToIPv4(16909060) = %v, want 1.2.3.4", got)
	}
}
