package number

import "testing"

// TestParseIntLossless covers every branch of parseIntLossless (the helper
// RoundTrunc uses to parse the truncated integer string without float precision
// loss): empty input, the signed (leading '-') path for both success and
// overflow, and the unsigned path for success, overflow, and malformed input.
func TestParseIntLossless(t *testing.T) {
	// Empty input.
	if v, ok := parseIntLossless[int64](""); ok || v != 0 {
		t.Fatalf("empty: got (%d,%v) want (0,false)", v, ok)
	}
	// Signed success.
	if v, ok := parseIntLossless[int64]("-12340"); !ok || v != -12340 {
		t.Fatalf("signed: got (%d,%v) want (-12340,true)", v, ok)
	}
	// Signed overflow -> ParseInt error.
	if _, ok := parseIntLossless[int64]("-99999999999999999999"); ok {
		t.Fatal("signed overflow: want (0,false)")
	}
	// Unsigned success.
	if v, ok := parseIntLossless[uint64]("456"); !ok || v != 456 {
		t.Fatalf("unsigned: got (%d,%v) want (456,true)", v, ok)
	}
	// Unsigned overflow -> ParseUint error.
	if _, ok := parseIntLossless[uint64]("99999999999999999999"); ok {
		t.Fatal("unsigned overflow: want (0,false)")
	}
	// Malformed (non-numeric) -> ParseUint error on the unsigned branch.
	if _, ok := parseIntLossless[int64]("abc"); ok {
		t.Fatal("malformed: want (0,false)")
	}
}
