package signing

import (
	"testing"
	"time"
)

var benchParams = map[string]string{
	"partner": "acme",
	"path":    "/v1/bid",
	"count":   "42",
}

// BenchmarkSign measures request signing: canonical-string build (sorted,
// query-escaped) + HMAC-SHA256 + hex encode.
func BenchmarkSign(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_, err := Sign(benchParams, "topsecret", WithTimestamp(time.Unix(1_700_000_000, 0)))
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkVerify measures verification of a valid signature: timestamp
// freshness check + canonical/HMAC recompute + constant-time compare.
func BenchmarkVerify(b *testing.B) {
	params := make(map[string]string, len(benchParams)+1)
	for k, v := range benchParams {
		params[k] = v
	}
	ts := time.Unix(1_700_000_000, 0)
	params[TimestampKey] = "1700000000"
	sig, err := Sign(benchParams, "topsecret", WithTimestamp(ts))
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()

	for b.Loop() {
		if !Verify(params, "topsecret", sig, WithMaxAge(0), WithNow(func() time.Time { return ts })) {
			b.Fatal("Verify failed")
		}
	}
}

// BenchmarkCanonical measures the canonical-string construction in isolation.
func BenchmarkCanonical(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = canonical(benchParams, 1_700_000_000)
	}
}

// BenchmarkCompute measures the HMAC-SHA256 + hex encode in isolation.
func BenchmarkCompute(b *testing.B) {
	msg := canonical(benchParams, 1_700_000_000)
	b.ReportAllocs()

	for b.Loop() {
		_ = compute("topsecret", msg)
	}
}
