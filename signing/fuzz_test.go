package signing

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

// FuzzSignRoundtrip asserts the core signing invariant: any params map and any
// secret that Sign accepts must Verify true when the receiver rebuilds the
// param set with the embedded timestamp. Freshness is disabled
// (WithMaxAge(0)) and the clock is pinned (WithNow), so the only variable
// exercised is the HMAC roundtrip — making the assertion fully deterministic.
//
// Seed corpus mirrors the existing unit-test shapes so -runs=0 (seed-only)
// already exercises: empty params, single entry, multi-entry, unicode values,
// and a value embedding the '&' separator (parameter-injection regression).
func FuzzSignRoundtrip(f *testing.F) {
	// Seeds: each (paramsBlob, secret) pair. paramsBlob is "k=v&k2=v2" form,
	// "" meaning an empty param set.
	seeds := []struct {
		params string
		secret string
	}{
		{"", "shared-secret"},
		{"k=v", "shared-secret"},
		{"auction_id=42&bidder=acme&price=1.25", "shared-secret"},
		{"k=v&a=b&c=d", "s"},
		{"\xc3\xa9=caf\xc3\xa9", "secret-with-unicode-金"},
		{"a=1%26b=2", "shared-secret"}, // value embeds '&' after URL-unescape
	}
	for _, s := range seeds {
		f.Add([]byte(s.params), []byte(s.secret))
	}

	// Fixed clock + maxAge=0: only the HMAC roundtrip is under test, never
	// expiry, so the verdict cannot depend on wall time.
	const ts int64 = 1_700_000_000
	now := func() time.Time { return time.Unix(ts, 0) }

	f.Fuzz(func(t *testing.T, paramsBlob, secret []byte) {
		params := parseParams(t, string(paramsBlob))

		sig, err := Sign(params, string(secret), WithTimestamp(time.Unix(ts, 0)))
		if err != nil {
			t.Fatalf("Sign returned unexpected error: %v", err)
		}

		// Rebuild the receiver's view: original params plus the embedded _ts.
		// Sign documents that it does NOT inject _ts into params, so the receiver
		// sees exactly what was signed plus the timestamp the carrier added.
		recv := make(map[string]string, len(params)+1)
		for k, v := range params {
			recv[k] = v
		}
		recv[TimestampKey] = strconv.FormatInt(ts, 10)

		if !Verify(recv, string(secret), sig, WithNow(now), WithMaxAge(0)) {
			t.Fatalf("Verify rejected a freshly signed param set: sig=%q params=%v", sig, recv)
		}
	})
}

// FuzzVerifyTampered asserts tamper resistance: flipping exactly one byte of a
// valid hex signature must make Verify return false. The signature is always
// produced from a freshly signed, in-window param set, so the baseline (untampered)
// signature would verify true; any single-byte mutation must therefore fail the
// constant-time HMAC compare. Determinism is guaranteed by a pinned timestamp
// and clock.
//
// The fuzz input only drives which byte index and replacement value are tried;
// the param set and secret are fixed so the baseline signature is stable within
// a given process and the assertion reduces to "tampered != valid".
func FuzzVerifyTampered(f *testing.F) {
	f.Add(uint8(0), uint8(1))  // flip first byte 0 -> 1
	f.Add(uint8(63), uint8(0)) // flip last byte to '0'
	f.Add(uint8(31), uint8(255))

	// Fixed, in-window signing/verification state.
	const ts int64 = 1_700_000_000
	now := func() time.Time { return time.Unix(ts, 0) }
	params := map[string]string{"auction_id": "42", "bidder": "acme", "price": "1.25"}
	const secret = "shared-secret"

	f.Fuzz(func(t *testing.T, idxByte, replByte uint8) {
		sig, err := Sign(params, secret, WithTimestamp(time.Unix(ts, 0)))
		if err != nil {
			t.Fatalf("Sign returned unexpected error: %v", err)
		}
		if len(sig) == 0 {
			t.Skip("empty signature; nothing to tamper")
		}

		// Sanity: the un-tampered signature must verify, otherwise the rest of
		// the assertion is meaningless.
		recv := map[string]string{
			"auction_id": "42",
			"bidder":     "acme",
			"price":      "1.25",
			TimestampKey: strconv.FormatInt(ts, 10),
		}
		if !Verify(recv, secret, sig, WithNow(now)) {
			t.Fatalf("baseline signature failed to verify before tampering: %q", sig)
		}

		idx := int(idxByte) % len(sig)
		want := rune(replByte)
		orig := rune(sig[idx])

		// If the fuzzer lands on a no-op flip (same byte), nudge it to a
		// different value so the test still asserts a real change.
		if want == orig {
			if orig == '0' {
				want = '1'
			} else {
				want = '0'
			}
		}

		tampered := sig[:idx] + string(want) + sig[idx+1:]
		if tampered == sig {
			t.Fatalf("tamper produced no change: idx=%d orig=%q want=%q", idx, orig, want)
		}

		if Verify(recv, secret, tampered, WithNow(now)) {
			t.Fatalf("Verify accepted a tampered signature: orig=%q tampered=%q idx=%d", sig, tampered, idx)
		}
	})
}

// parseParams decodes a "k=v&k2=v2" blob into a map. Malformed fragments (no
// '=', empty key) are skipped rather than failing the fuzz iteration, so the
// corpus converges on well-formed maps while still tolerating arbitrary bytes.
// Empty/wholly-malformed input yields an empty map, which Sign handles.
func parseParams(t *testing.T, blob string) map[string]string {
	t.Helper()
	out := make(map[string]string)
	if strings.TrimSpace(blob) == "" {
		return out
	}
	for _, pair := range strings.Split(blob, "&") {
		eq := strings.IndexByte(pair, '=')
		if eq < 0 {
			continue // not a k=v pair; skip
		}
		k := pair[:eq]
		v := pair[eq+1:]
		if k == "" {
			continue // empty key is not a valid parameter
		}
		// Drop anything that collides with the reserved keys so the fuzzer
		// cannot manufacture a timestamp/signature collision; Sign ignores
		// them anyway, this just keeps the receiver map honest.
		if k == SignatureKey || k == TimestampKey {
			continue
		}
		out[k] = v
	}
	return out
}
