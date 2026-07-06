package signing

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const testSecret = "shared-secret"

// fixedNow is a deterministic clock for expiry tests.
func fixedNow() time.Time { return time.Unix(1_700_000_000, 0) }

// signAt signs params as of instant ts and returns the hex signature plus the
// params map with the embedded _ts entry (the form a receiver would see).
func signAt(t *testing.T, params map[string]string, ts int64) (string, map[string]string) {
	t.Helper()
	at := time.Unix(ts, 0)
	sig, err := Sign(params, testSecret, WithTimestamp(at))
	require.NoError(t, err)
	recv := make(map[string]string, len(params)+1)
	for k, v := range params {
		recv[k] = v
	}
	recv[TimestampKey] = strconv.FormatInt(ts, 10)
	return sig, recv
}

func TestSign_RoundtripWithVerify(t *testing.T) {
	params := map[string]string{"auction_id": "42", "bidder": "acme", "price": "1.25"}
	sig, recv := signAt(t, params, fixedNow().Unix())

	require.True(t, Verify(recv, testSecret, sig, WithNow(fixedNow)),
		"verify must accept a freshly signed, untampered param set")
}

func TestSign_DeterministicWithTimestamp(t *testing.T) {
	params := map[string]string{"k": "v", "a": "b"}
	ts := time.Unix(1_234_567_890, 0)

	sig1, err := Sign(params, testSecret, WithTimestamp(ts))
	require.NoError(t, err)
	sig2, err := Sign(params, testSecret, WithTimestamp(ts))
	require.NoError(t, err)
	require.Equal(t, sig1, sig2, "same inputs must yield the same signature")

	// A different timestamp must yield a different signature.
	sig3, err := Sign(params, testSecret, WithTimestamp(ts.Add(time.Second)))
	require.NoError(t, err)
	require.NotEqual(t, sig1, sig3)
}

func TestSign_MatchesHandRolledHMAC(t *testing.T) {
	// Independently reconstruct the canonical string and HMAC to confirm the
	// implementation builds exactly the documented format.
	params := map[string]string{"b": "2", "a": "1"}
	ts := int64(1_700_000_000)
	sig, err := Sign(params, testSecret, WithTimestamp(time.Unix(ts, 0)))
	require.NoError(t, err)

	// Canonical: sorted params (a, b), then _ts last.
	want := "a=1&b=2&_ts=" + strconv.FormatInt(ts, 10)
	mac := hmac.New(sha256.New, []byte(testSecret))
	mac.Write([]byte(want))
	require.Equal(t, hex.EncodeToString(mac.Sum(nil)), sig)
}

func TestVerify_ExpiredTimestamp_ReturnsFalse(t *testing.T) {
	params := map[string]string{"k": "v"}
	now := fixedNow()
	// Signed 10 minutes ago; default max age is 5m -> expired.
	sig, recv := signAt(t, params, now.Add(-10*time.Minute).Unix())

	require.False(t, Verify(recv, testSecret, sig, WithNow(func() time.Time { return now })),
		"expired timestamp must be rejected")
}

func TestVerify_CustomMaxAge_AllowsOlderSignature(t *testing.T) {
	params := map[string]string{"k": "v"}
	now := fixedNow()
	// Signed 7 minutes ago: rejected by default 5m, accepted by explicit 15m.
	sig, recv := signAt(t, params, now.Add(-7*time.Minute).Unix())

	require.False(t, Verify(recv, testSecret, sig, WithNow(func() time.Time { return now })))
	require.True(t, Verify(recv, testSecret, sig,
		WithNow(func() time.Time { return now }), WithMaxAge(15*time.Minute)))
}

func TestVerify_MaxAgeZero_DisablesFreshnessCheck(t *testing.T) {
	params := map[string]string{"k": "v"}
	now := fixedNow()
	// Very old, but WithMaxAge(0) means "verify HMAC only".
	sig, recv := signAt(t, params, now.Add(365*24*time.Hour).Unix())

	require.True(t, Verify(recv, testSecret, sig,
		WithNow(func() time.Time { return now }), WithMaxAge(0)))
}

func TestVerify_FutureTimestamp_ReturnsFalse(t *testing.T) {
	params := map[string]string{"k": "v"}
	now := fixedNow()
	// A timestamp in the future is treated as suspect, not as skew tolerance.
	sig, recv := signAt(t, params, now.Add(2*time.Minute).Unix())

	require.False(t, Verify(recv, testSecret, sig, WithNow(func() time.Time { return now })))
}

func TestVerify_TamperedParam_ReturnsFalse(t *testing.T) {
	params := map[string]string{"auction_id": "42", "price": "1.25"}
	sig, recv := signAt(t, params, fixedNow().Unix())

	recv["price"] = "9.99" // tamper after signing
	require.False(t, Verify(recv, testSecret, sig, WithNow(fixedNow)))
}

func TestVerify_TamperedTimestamp_ReturnsFalse(t *testing.T) {
	params := map[string]string{"k": "v"}
	sig, recv := signAt(t, params, fixedNow().Unix())

	recv[TimestampKey] = strconv.FormatInt(fixedNow().Add(-time.Hour).Unix(), 10)
	require.False(t, Verify(recv, testSecret, sig, WithNow(fixedNow)))
}

func TestVerify_WrongSecret_ReturnsFalse(t *testing.T) {
	params := map[string]string{"k": "v"}
	sig, recv := signAt(t, params, fixedNow().Unix())

	require.False(t, Verify(recv, "wrong-secret", sig, WithNow(fixedNow)))
}

func TestVerify_MissingTimestamp_ReturnsFalse(t *testing.T) {
	params := map[string]string{"k": "v"}
	sig, recv := signAt(t, params, fixedNow().Unix())
	delete(recv, TimestampKey)

	require.False(t, Verify(recv, testSecret, sig, WithNow(fixedNow)))
}

func TestVerify_MalformedTimestamp_ReturnsFalse(t *testing.T) {
	params := map[string]string{"k": "v"}
	sig, recv := signAt(t, params, fixedNow().Unix())
	recv[TimestampKey] = "not-a-number"

	require.False(t, Verify(recv, testSecret, sig, WithNow(fixedNow)))
}

func TestVerify_TamperedSignature_ReturnsFalse(t *testing.T) {
	params := map[string]string{"k": "v"}
	sig, recv := signAt(t, params, fixedNow().Unix())

	// Flip a hex character (keep it valid hex so the compare differs on value).
	tampered := sig
	if tampered[0] == '0' {
		tampered = "1" + tampered[1:]
	} else {
		tampered = "0" + tampered[1:]
	}
	require.False(t, Verify(recv, testSecret, tampered, WithNow(fixedNow)))
}

func TestSignSorted_EqualsUnsortedParams(t *testing.T) {
	// The canonical form sorts keys, so two maps with the same key/value pairs
	// but different insertion order must produce identical signatures.
	ordered := map[string]string{"a": "1", "b": "2", "c": "3"}
	reversed := map[string]string{"c": "3", "b": "2", "a": "1"}
	ts := time.Unix(99, 0)

	sigA, err := Sign(ordered, testSecret, WithTimestamp(ts))
	require.NoError(t, err)
	sigB, err := Sign(reversed, testSecret, WithTimestamp(ts))
	require.NoError(t, err)
	require.Equal(t, sigA, sigB, "signature must be independent of map iteration order")
}

func TestSign_EmptyParams(t *testing.T) {
	// Empty params still yields a valid signature over just the timestamp.
	ts := time.Unix(5, 0)
	sig, err := Sign(map[string]string{}, testSecret, WithTimestamp(ts))
	require.NoError(t, err)
	require.Len(t, sig, 64) // hex of 32-byte SHA-256

	// And it round-trips through Verify with the embedded _ts.
	recv := map[string]string{TimestampKey: "5"}
	require.True(t, Verify(recv, testSecret, sig, WithNow(func() time.Time { return ts }), WithMaxAge(0)))
}

func TestSign_DoesNotMutateParams(t *testing.T) {
	params := map[string]string{"a": "1"}
	_, err := Sign(params, testSecret, WithTimestamp(time.Unix(1, 0)))
	require.NoError(t, err)
	require.NotContains(t, params, TimestampKey, "Sign must not write the timestamp into params")
	require.NotContains(t, params, SignatureKey)
}

func TestSign_DifferentSecretsDiffer(t *testing.T) {
	params := map[string]string{"k": "v"}
	ts := time.Unix(1, 0)
	a, err := Sign(params, "s1", WithTimestamp(ts))
	require.NoError(t, err)
	b, err := Sign(params, "s2", WithTimestamp(ts))
	require.NoError(t, err)
	require.NotEqual(t, a, b)
}

func TestSign_IgnoresPreExistingSigAndTs(t *testing.T) {
	// If params already carry a stale _sig or _ts, Sign ignores them when
	// building the canonical string (uses the injected timestamp).
	params := map[string]string{
		"k":          "v",
		SignatureKey: "stale-signature",
		TimestampKey: "111",
	}
	ts := time.Unix(2_000_000_000, 0)
	sig, err := Sign(params, testSecret, WithTimestamp(ts))
	require.NoError(t, err)

	// Expected canonical: only k=v plus the injected _ts.
	want := "k=v&_ts=" + strconv.FormatInt(ts.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(testSecret))
	mac.Write([]byte(want))
	require.Equal(t, hex.EncodeToString(mac.Sum(nil)), sig)
}

// Verify must be safe under concurrent access (it only reads params). Run under
// -race to confirm no shared-state issues.
func TestVerify_Concurrent(t *testing.T) {
	params := map[string]string{"k": "v", "auction_id": "7"}
	sig, recv := signAt(t, params, fixedNow().Unix())

	const n = 32
	var wg sync.WaitGroup
	bad := make(chan bool, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				if !Verify(recv, testSecret, sig, WithNow(fixedNow)) {
					bad <- true
					return
				}
			}
		}()
	}
	wg.Wait()
	close(bad)
	require.Empty(t, bad, "all concurrent verifies must pass")
}

func TestConstantTimeCompare_Internal(t *testing.T) {
	// Sanity-check the compare path used by Verify: equal strings pass, any
	// difference fails, and length differences fail rather than panic.
	params := map[string]string{"k": "v"}
	sig, recv := signAt(t, params, fixedNow().Unix())
	require.True(t, Verify(recv, testSecret, sig, WithNow(fixedNow)))
	// Truncated signature (different length) must not match and must not panic.
	require.False(t, Verify(recv, testSecret, sig[:10], WithNow(fixedNow)))
}

// TestSign_NoParameterInjection is the P1 regression: after QueryEscape, a
// single key whose value embeds the separators must NOT collide with two
// distinct keys (parameter-injection resistance).
func TestSign_NoParameterInjection(t *testing.T) {
	one, _ := Sign(map[string]string{"a": "1&b=2"}, testSecret, WithTimestamp(fixedNow()))
	two, _ := Sign(map[string]string{"a": "1", "b": "2"}, testSecret, WithTimestamp(fixedNow()))
	require.NotEqual(t, one, two, "parameter-injection collision: {a:1&b=2} vs {a:1,b:2}")
}
