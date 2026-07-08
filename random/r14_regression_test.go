package random_test

// R14 regression tests for the non-crypto helpers in the random package.
//
// These are documented math/rand/v2 helpers (NOT security paths), but the
// bugs they cover are real correctness defects:
//
//   - F1: RandStringWithKind off-by-one excluded the last char of each kind.
//   - F2: RandIn / RandStringInCharset modulo bias over-sampled low indices.
//   - F3: StringByRead ignored crypto/rand.Read errors (encoded zero bytes).
//   - F4: RandStringInCharset divided by zero on an empty charset.
//
// Each test draws a large sample and asserts the previously-broken behavior is
// fixed. Tolerances are deliberately generous (no CI flakes) but tight enough
// that the old code fails decisively (0 occurrences / ~50% vs ~33%).

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/v8fg/kit4go/random"
)

// TestR14_F1_RandStringWithKind_LastCharEmitted verifies the off-by-one fix:
// '9' (digit kind=1), 'Z' (upper kind=2) and 'z' (lower kind=4) must each
// appear a non-zero, roughly-expected count. The old code (rand.IntN(count))
// never emitted any of them (0 occurrences over millions of draws).
func TestR14_F1_RandStringWithKind_LastCharEmitted(t *testing.T) {
	cases := []struct {
		name    string
		kind    int
		last    byte // the previously-excluded last character
		wantMin int  // generous floor: with n*200k chars, expect ~200k/10 (digits) or ~200k/26 (letters)
	}{
		{"digit '9'", 0b0001, '9', 10_000},
		{"upper 'Z'", 0b0010, 'Z', 3_000},
		{"lower 'z'", 0b0100, 'z', 3_000},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			const n = 200_000
			out := random.RandStringWithKind(n, c.kind)
			require.Len(t, out, n, "expected %d chars", n)

			var count int
			for i := 0; i < len(out); i++ {
				if out[i] == c.last {
					count++
				}
			}
			// Old code: count == 0 (IntN(count) excludes the last char).
			require.Greater(t, count, 0,
				"last char %q never emitted (count=0 reproduces the off-by-one)", c.last)
			require.GreaterOrEqual(t, count, c.wantMin,
				"last char %q under-sampled: got %d, want >= %d", c.last, count, c.wantMin)
		})
	}
}

// TestR14_F2_RandIn_UniformOverNonPowerOfTwo verifies the modulo-bias fix in
// RandIn: over a 3-element set each element must be ~33%. The old code
// (`Int64()&idxMask % n`) produced idx0 ~50% because idxMask=3 maps 4 raw
// values onto 3 indices (0,1,2,0).
func TestR14_F2_RandIn_UniformOverNonPowerOfTwo(t *testing.T) {
	const samples = 300_000
	// A 3-element set: 3 is not a power of two, so the old modulo path biased idx0.
	set := []int{10, 20, 30}
	counts := map[int]int{10: 0, 20: 0, 30: 0}
	for range samples {
		v, err := random.RandIn(set)
		require.NoError(t, err)
		counts[v]++
	}

	const expected = samples / 3 // 100_000
	// Old code: counts[10] ~50% (150_000), counts[20]=counts[30] ~25% (75_000).
	// With a 15% tolerance around 100k (85_000..115_000), the old code fails
	// decisively while the fixed uniform code passes comfortably.
	const tol = expected * 15 / 100
	for _, v := range set {
		require.InDelta(t, expected, counts[v], tol,
			"RandIn biased over 3-element set: value %d count=%d", v, counts[v])
	}
}

// TestR14_F2_RandStringInCharset_UniformOverNonPowerOfTwo verifies the index
// selection in RandStringInCharset is uniform over a 3-rune charset.
func TestR14_F2_RandStringInCharset_UniformOverNonPowerOfTwo(t *testing.T) {
	const n = 300_000
	charset := []rune{'A', 'B', 'C'} // len 3, not a power of two
	out := random.RandStringInCharset(n, charset)
	require.Len(t, out, n)

	counts := map[rune]int{'A': 0, 'B': 0, 'C': 0}
	for _, r := range out {
		counts[r]++
	}
	const expected = n / 3 // 100_000
	const tol = expected * 15 / 100
	for _, r := range charset {
		require.InDelta(t, expected, counts[r], tol,
			"RandStringInCharset biased over 3-rune charset: %q count=%d", r, counts[r])
	}
}

// TestR14_F4_RandStringInCharset_EmptyCharsetNoPanic verifies the divide-by-
// zero guard: nil and empty charsets return "" instead of panicking.
func TestR14_F4_RandStringInCharset_EmptyCharsetNoPanic(t *testing.T) {
	require.NotPanics(t, func() {
		require.Equal(t, "", random.RandStringInCharset(3, nil))
	})
	require.NotPanics(t, func() {
		require.Equal(t, "", random.RandStringInCharset(3, []rune{}))
	})
	// n <= 0 still returns "" regardless of charset.
	require.Equal(t, "", random.RandStringInCharset(0, []rune("ABC")))
}

// TestR14_F3_StringByRead_ReadErrorReturnsEmpty verifies that StringByRead
// returns "" when the underlying crypto reader fails, rather than encoding
// zero/partial bytes into a predictable string. The old code ignored the error
// and always returned base64(zeros).
func TestR14_F3_StringByRead_ReadErrorReturnsEmpty(t *testing.T) {
	mockSource := new(random.MockCryptoSource)
	// Inject a failing Read. StringByRead routes through DefaultCryptoSource.
	mockSource.EXPECT().Read([]byte{1, 2, 3, 4}).Return(0, errors.New("simulated CSPRNG failure")).Once()
	withCryptoSource(t, mockSource, func() {
		// On failure: must be "" (old code returned base64 of zero bytes).
		require.Equal(t, "", random.StringByRead([]byte{1, 2, 3, 4}))
	})

	// And the happy path still returns a non-empty base64 string.
	mockSource2 := new(random.MockCryptoSource)
	mockSource2.EXPECT().Read([]byte{1, 2, 3, 4}).Return(4, nil).Once()
	withCryptoSource(t, mockSource2, func() {
		require.NotEqual(t, "", random.StringByRead([]byte{1, 2, 3, 4}))
	})
}

// TestR14_F3_StringByRead_EmptyBufferReturnsEmpty covers the len(b)==0 guard.
func TestR14_F3_StringByRead_EmptyBufferReturnsEmpty(t *testing.T) {
	// No mock needed: empty buffer short-circuits before Read.
	require.Equal(t, "", random.StringByRead(nil))
	require.Equal(t, "", random.StringByRead([]byte{}))
}
