package otp

import (
	"testing"
	"time"
)

const benchSecret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ" // base32, 20 bytes decoded

// BenchmarkTOTPCode measures time-based OTP generation (HMAC-SHA1 over the
// time-step counter).
func BenchmarkTOTPCode(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = TOTPCode(benchSecret)
	}
}

// BenchmarkTOTPCodeCustom measures TOTP at a fixed time (no time.Now in the path).
func BenchmarkTOTPCodeCustom(b *testing.B) {
	t := time.Unix(1_700_000_000, 0)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = TOTPCodeCustom(benchSecret, t, nil)
	}
}

// BenchmarkHOTPCode measures counter-based OTP generation.
func BenchmarkHOTPCode(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = HOTPCode(benchSecret, 42)
	}
}

// BenchmarkVerifyTOTP measures a successful TOTP verification (recomputes the
// code and compares).
func BenchmarkVerifyTOTP(b *testing.B) {
	code, err := TOTPCode(benchSecret)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !VerifyTOTP(code, benchSecret) {
			b.Fatal("VerifyTOTP failed")
		}
	}
}
