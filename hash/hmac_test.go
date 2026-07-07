package hash

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// RFC 4231-style vectors and the classic Wikipedia HMAC-SHA256 example.
func TestHMAC_KnownVectors(t *testing.T) {
	cases := []struct {
		name   string
		key    string
		data   string
		sha256 string
	}{
		{"wikipedia-fox", "key", "The quick brown fox jumps over the lazy dog",
			"f7bc83f430538424b13298e6aa6fb143ef4d59a14946175997479dbc2d1a3cd8"},
		{"empty-empty", "", "",
			"b613679a0814d9ec772f95d778c35fc5ff1697c493715653c6c712144292c5ad"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.sha256, HMACSHA256Hex([]byte(c.key), []byte(c.data)))
		})
	}
}

func TestHMAC_AlgorithmsAndLengths(t *testing.T) {
	key, data := []byte("secret"), []byte("payload")
	require.Equal(t, 20, len(HMACSHA1(key, data)))
	require.Equal(t, 32, len(HMACSHA256(key, data)))
	require.Equal(t, 64, len(HMACSHA512(key, data)))

	// Matches a hand-rolled stdlib HMAC.
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	require.True(t, bytes.Equal(HMACSHA256(key, data), mac.Sum(nil)))
}

func TestHMAC_Base64(t *testing.T) {
	key, data := []byte("k"), []byte("d")
	// base64 of the raw HMAC must decode back to the raw HMAC.
	raw := HMACSHA256(key, data)
	dec, err := base64.StdEncoding.DecodeString(HMACSHA256Base64(key, data))
	require.NoError(t, err)
	require.True(t, bytes.Equal(raw, dec))
}

func TestHMAC_DifferentKeysDiffer(t *testing.T) {
	d := []byte("data")
	require.False(t, bytes.Equal(HMACSHA256([]byte("k1"), d), HMACSHA256([]byte("k2"), d)))
}

// Equal must be constant-time and agree with bytes.Equal for matching inputs.
func TestEqual(t *testing.T) {
	a := HMACSHA256([]byte("k"), []byte("d"))
	b := HMACSHA256([]byte("k"), []byte("d"))
	c := HMACSHA256([]byte("k"), []byte("x"))
	require.True(t, Equal(a, b))
	require.False(t, Equal(a, c))
	require.True(t, Equal(nil, nil))
	require.False(t, Equal(a, nil))
}

func TestHMAC_Concurrent(t *testing.T) {
	const n = 32
	want := HMACSHA256Hex([]byte("k"), []byte("d"))
	var wg sync.WaitGroup
	errs := make(chan string, n)
	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			for range 100 {
				if HMACSHA256Hex([]byte("k"), []byte("d")) != want {
					errs <- "mismatch"
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	require.Empty(t, errs)
}
