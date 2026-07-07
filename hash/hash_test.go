package hash

import (
	"bytes"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

// Known vectors from published test suites (NIST FIPS, RFC 1321).
func TestCryptoDigests_KnownVectors(t *testing.T) {
	cases := []struct {
		name string
		in   string
		md5  string
		sha1 string
		s256 string
		s512 string
	}{
		{"empty", "",
			"d41d8cd98f00b204e9800998ecf8427e",
			"da39a3ee5e6b4b0d3255bfef95601890afd80709",
			"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			"cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e"},
		{"abc", "abc",
			"900150983cd24fb0d6963f7d28e17f72",
			"a9993e364706816aba3e25717850c26c9cd0d89d",
			"ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
			"ddaf35a193617abacc417349ae20413112e6fa4e89a97ea20a9eeee64b55d39a2192992a274fc1a836ba3c23a3feebbd454d4423643ce80e2a9ac94fa54ca49f"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.md5, MD5Hex(c.in))
			require.Equal(t, c.sha1, SHA1Hex(c.in))
			require.Equal(t, c.s256, SHA256Hex(c.in))
			require.Equal(t, c.s512, SHA512Hex(c.in))
		})
	}
}

// Raw-byte digests must match the stdlib hasher exactly and have fixed lengths.
func TestCryptoDigests_RawBytesAndLengths(t *testing.T) {
	data := []byte("auction_id=42")
	require.Equal(t, 16, len(MD5(data)))
	require.Equal(t, 20, len(SHA1(data)))
	require.Equal(t, 28, len(SHA224(data)))
	require.Equal(t, 32, len(SHA256(data)))
	require.Equal(t, 48, len(SHA384(data)))
	require.Equal(t, 64, len(SHA512(data)))

	s256 := sha256.Sum256(data)
	require.True(t, bytes.Equal(SHA256(data), s256[:]))
	m := md5.Sum(data)
	require.True(t, bytes.Equal(MD5(data), m[:]))
	s1 := sha1.Sum(data)
	require.True(t, bytes.Equal(SHA1(data), s1[:]))
	var s512 [64]byte = sha512.Sum512(data)
	require.True(t, bytes.Equal(SHA512(data), s512[:]))
}

// MD5Hex/SHA256Hex must equal hex.EncodeToString of the raw bytes.
func TestHexHelpers(t *testing.T) {
	s := "bid/imp/win"
	require.Equal(t, hex.EncodeToString(MD5([]byte(s))), MD5Hex(s))
	require.Equal(t, hex.EncodeToString(SHA256([]byte(s))), SHA256Hex(s))
}

// A fresh hasher per call means concurrent callers never corrupt each other.
func TestCryptoDigests_Concurrent(t *testing.T) {
	const goroutines = 64
	const iters = 200
	want := SHA256Hex("payload")
	var wg sync.WaitGroup
	var mismatches atomic.Uint64
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			for range iters {
				if got := SHA256Hex("payload"); got != want {
					mismatches.Add(1)
				}
			}
		}()
	}
	wg.Wait()
	require.Equal(t, uint64(0), mismatches.Load(),
		"concurrent SHA256 calls produced inconsistent results")
}

func TestNilAndEmptyInput(t *testing.T) {
	// nil and empty slice must hash identically (they do in stdlib).
	require.Equal(t, MD5(nil), MD5([]byte{}))
	require.Equal(t, SHA256(nil), SHA256([]byte{}))
	// Empty-string hex digests (non-empty result, not "").
	require.Equal(t, "d41d8cd98f00b204e9800998ecf8427e", MD5Hex(""))
	require.Len(t, SHA256Hex(""), 64)
}
