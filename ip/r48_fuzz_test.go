package ip_test

import (
	"encoding/hex"
	"net"
	"strconv"
	"testing"

	"github.com/v8fg/kit4go/ip"
)

// FuzzMaskIPToCIDRCanonicalRoundTrip locks the canonical-mask round-trip over
// the full prefix range: MaskIPToCIDR applied to the canonical mask for prefix
// n returns "/n". (Non-canonical masks are rejected — exercised by
// TestR47_MaskIPToCIDR_RejectsNonCanonical; the contiguity fix is also unit-
// tested there.) Guards the CIDR mask path against a regression. E10.
func FuzzMaskIPToCIDRCanonicalRoundTrip(f *testing.F) {
	f.Add(0)
	f.Add(8)
	f.Add(24)
	f.Add(32)
	f.Fuzz(func(t *testing.T, n int) {
		if n < 0 {
			n = -n
		}
		n %= 33                     // [0, 32]
		mask := net.CIDRMask(n, 32) // canonical 4-byte mask
		input := "0.0.0.0/" + hex.EncodeToString(mask)
		want := "0.0.0.0/" + strconv.Itoa(n)
		if got := ip.MaskIPToCIDR(input); got != want {
			t.Errorf("MaskIPToCIDR(%q) = %q, want %q (canonical mask round-trip)", input, got, want)
		}
	})
}
