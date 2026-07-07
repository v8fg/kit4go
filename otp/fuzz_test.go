package otp_test

import (
	"encoding/base32"
	"testing"
	"time"

	"github.com/v8fg/kit4go/otp"
)

// fixedFuzzTime is a frozen timestamp used by the TOTP fuzz target so that
// code generation and verification share the exact same 30s step. The fuzzer
// must NOT depend on wall-clock skew, so we never call time.Now() inside
// f.Fuzz: generating and verifying at the same instant eliminates the ±1
// window and makes every assertion deterministic.
var fixedFuzzTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// seedSecrets are known-valid base32 (NoPadding) secrets used as f.Add corpus
// seeds. They are produced from raw bytes via base32 so the round-trip is
// guaranteed valid for upstream's base32 decoder.
var (
	seedSecret1 = base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte("hello"))
	seedSecret2 = base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(make([]byte, 20))
	seedSecret3 = "JBSWY3DPEHPK3PXP" // canonical RFC test vector secret
)

// FuzzHOTPCode exercises HOTPCode + VerifyHOTP over a fuzzed base32 secret and
// counter. For every (secret, counter) pair the following invariants must hold:
//
//  1. HOTPCode returns no error and a 6-digit code (default Digits=6).
//  2. VerifyHOTP accepts that code for the same counter (HOTP is fully
//     deterministic in secret+counter, so this never flakes).
//
// The raw fuzzer bytes are base32-encoded into the secret, guaranteeing
// upstream's base32 decoder always succeeds and we always hit the happy path.
// Empty/whitespace secrets are intentionally included: they decode to an empty
// key and HMAC still digests them into a real (if insecure) code — that is a
// documented upstream behavior, not an error, and the fuzz target asserts it
// stays non-erroring rather than masking it.
func FuzzHOTPCode(f *testing.F) {
	for _, seed := range []string{seedSecret1, seedSecret2, seedSecret3} {
		for _, counter := range []uint64{0, 1, 2, 42} {
			f.Add([]byte(seed), counter)
		}
	}

	f.Fuzz(func(t *testing.T, rawSecret []byte, counter uint64) {
		secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(rawSecret)

		code, err := otp.HOTPCode(secret, counter)
		if err != nil {
			// A base32-encoded secret must never reach the upstream decoder
			// error path. If it does, something regressed.
			t.Fatalf("HOTPCode(%q, %d): unexpected error: %v", secret, counter, err)
		}
		if len(code) != 6 {
			t.Fatalf("HOTPCode(%q, %d): code len = %d, want 6", secret, counter, len(code))
		}

		if !otp.VerifyHOTP(code, counter, secret) {
			t.Fatalf("VerifyHOTP rejected freshly-generated code %q for secret=%q counter=%d", code, secret, counter)
		}
	})
}

// FuzzTOTPCode exercises TOTP generation + verification over a fuzzed base32
// secret. TOTP is time-bound, so to keep assertions deterministic we drive
// both sides with a single frozen timestamp (fixedFuzzTime) via the *Custom
// variants. This pins code generation and verification to the same 30s step
// and removes any ±1 window flakiness — the invariant is:
//
//	TOTPCodeCustom(secret, fixedFuzzTime) -> code, no error, 6 digits
//	VerifyTOTPCustom(code, secret, fixedFuzzTime) == true
//
// The default TOTPCode/VerifyTOTP entry points are covered by the unit tests
// (TestTOTPCode, TestVerifyTOTP); the fuzz target deliberately uses the Custom
// pair to guarantee repeatability under the fuzzer.
func FuzzTOTPCode(f *testing.F) {
	for _, seed := range []string{seedSecret1, seedSecret2, seedSecret3} {
		f.Add([]byte(seed))
	}

	f.Fuzz(func(t *testing.T, rawSecret []byte) {
		secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(rawSecret)

		code, err := otp.TOTPCodeCustom(secret, fixedFuzzTime, nil)
		if err != nil {
			t.Fatalf("TOTPCodeCustom(%q, %v): unexpected error: %v", secret, fixedFuzzTime, err)
		}
		if len(code) != 6 {
			t.Fatalf("TOTPCodeCustom(%q, %v): code len = %d, want 6", secret, fixedFuzzTime, len(code))
		}

		if !otp.VerifyTOTPCustom(code, secret, fixedFuzzTime, nil) {
			t.Fatalf("VerifyTOTPCustom rejected freshly-generated code %q for secret=%q at %v", code, secret, fixedFuzzTime)
		}
	})
}
