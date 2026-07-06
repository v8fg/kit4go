// Package signing provides HMAC request signing for API authentication.
//
// Whereas the kit4go hash package performs content hashing (digesting a byte
// payload with a fixed algorithm), this package authenticates a request:
// it canonicalizes a set of request parameters, binds them to a signing
// timestamp, and HMAC-SHA256 signs the canonical string under a shared
// secret. Verify recomputes the signature and checks the timestamp is within
// a max age, defeating replay.
//
// Canonical string format: parameters sorted by key (excluding the signature
// and timestamp keys), joined as k1=v1&k2=v2&...&<TimestampKey>=<unix-seconds>,
// then HMAC-SHA256(secret, string), hex-encoded. The timestamp is carried in
// the parameter set under a fixed key (TimestampKey = "_ts") so the receiver
// can verify it.
//
// Pure standard library. Ad-tech SSP/DSP uses: authenticating bid requests,
// postbacks, MMP callbacks, and internal API calls with a shared secret.
package signing

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// SignatureKey is the parameter name carrying the computed signature in the
// parameter set. It is excluded from the canonical signing string.
const SignatureKey = "_sig"

// TimestampKey is the parameter name carrying the signing timestamp (Unix
// seconds) in the parameter set. It IS included in the canonical signing
// string, binding the signature to a point in time.
const TimestampKey = "_ts"

// DefaultMaxAge is the default window (5 minutes) within which a signature is
// considered fresh when no WithMaxAge option is supplied to Verify.
const DefaultMaxAge = 5 * time.Minute

// config holds the resolved options for a Sign or Verify call.
type config struct {
	timestamp time.Time        // Sign: the signing instant (default now).
	maxAge    time.Duration    // Verify: reject if _ts is older than now-maxAge.
	now       func() time.Time // Verify: clock seam for expiry checks.
}

// Option configures a Sign or Verify call.
type Option func(*config)

// WithTimestamp sets the signing instant for Sign (default time.Now). Use it in
// tests so signatures are deterministic. It has no effect on Verify, which
// reads the timestamp from the received params.
func WithTimestamp(ts time.Time) Option {
	return func(c *config) { c.timestamp = ts }
}

// WithMaxAge sets the freshness window for Verify (default DefaultMaxAge). A
// signature whose embedded _ts is older than now-maxAge (or in the future) is
// rejected. 0 disables the freshness check (verify the HMAC only).
func WithMaxAge(d time.Duration) Option {
	return func(c *config) { c.maxAge = d }
}

// WithNow injects the clock used by Verify for expiry checks, enabling
// deterministic expiry tests without sleeping. It has no effect on Sign.
func WithNow(fn func() time.Time) Option {
	return func(c *config) { c.now = fn }
}

// resolve merges opts onto a config populated with defaults.
func resolve(opts []Option) *config {
	c := &config{
		timestamp: time.Now(),
		maxAge:    DefaultMaxAge,
		now:       time.Now,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Sign computes the hex-encoded HMAC-SHA256 signature of params under secret.
//
// The canonical signing string is built from params sorted by key (excluding
// SignatureKey and TimestampKey), then a single trailing TimestampKey entry
// for the signing instant. Sign does NOT mutate params; callers that need the
// timestamp in the transmitted params should read it from the embedded value or
// set it themselves before signing.
//
// An empty secret yields a signature derived from the empty key, which is a
// valid (if weak) HMAC; callers are responsible for supplying a strong secret.
// It never returns an error in the current stdlib HMAC implementation but
// keeps the error in the signature for forward compatibility.
func Sign(params map[string]string, secret string, opts ...Option) (string, error) {
	c := resolve(opts)
	msg := canonical(params, c.timestamp.Unix())
	return compute(secret, msg), nil
}

// Verify recomputes the signature from params (which must include the
// TimestampKey entry produced at signing time) and reports whether it matches
// signature in constant time AND the embedded timestamp is within maxAge of
// now. Missing TimestampKey, a malformed timestamp, an out-of-window timestamp
// (too old or in the future), or any HMAC mismatch all return false.
func Verify(params map[string]string, secret, signature string, opts ...Option) bool {
	c := resolve(opts)

	raw, ok := params[TimestampKey]
	if !ok {
		return false
	}
	ts, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return false
	}
	// Freshness: reject stale or future-dated timestamps. A future timestamp
	// is treated as a replay/attack vector, not clock skew tolerance.
	if c.maxAge > 0 {
		now := c.now()
		signedAt := time.Unix(ts, 0)
		if now.Sub(signedAt) > c.maxAge || signedAt.After(now) {
			return false
		}
	}

	want := compute(secret, canonical(params, ts))
	return subtle.ConstantTimeCompare([]byte(want), []byte(signature)) == 1
}

// canonical builds the deterministic signing string: params sorted by key
// (excluding SignatureKey and TimestampKey) as k=v pairs joined by '&', then a
// trailing &_ts=<unixSeconds>. Keys and values are url.QueryEscape'd so the
// separators '&' and '=' are NOT ambiguous — a value containing "&b=2" cannot
// masquerade as a second parameter (parameter-injection resistance). The
// timestamp is appended last so the sorted params portion is unaffected by it.
func canonical(params map[string]string, ts int64) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		if k == SignatureKey || k == TimestampKey {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.Grow(len(keys) * 16) // heuristic; avoids some regrowths
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(url.QueryEscape(k))
		b.WriteByte('=')
		b.WriteString(url.QueryEscape(params[k]))
	}
	// Append the bound timestamp. Use a leading '&' only when there were other
	// params, so an empty param set still has a well-formed canonical string.
	if len(keys) > 0 {
		b.WriteByte('&')
	}
	b.WriteString(TimestampKey)
	b.WriteByte('=')
	b.WriteString(strconv.FormatInt(ts, 10))
	return b.String()
}

// compute returns the lowercase hex HMAC-SHA256 of msg keyed by secret.
func compute(secret, msg string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(msg)) // sha256/hash Write never errors
	return hex.EncodeToString(mac.Sum(nil))
}
