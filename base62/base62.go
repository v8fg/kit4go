// Package base62 encodes and decodes unsigned integers as base-62 strings
// (0-9, A-Z, a-z) — the standard "short code" encoding for URL shorteners,
// tracking/click URLs, and shareable IDs.
//
// Encode maps an auto-increment integer ID to a compact, URL-safe slug; Decode
// reverses it. That round-trip is the one reusable primitive every short-link
// service needs (the storage of long↔short mappings is application-level, not
// here). Pure standard library, zero dependencies.
package base62

import "errors"

// Alphabet is the default base-62 alphabet: digits, then upper-, then lower-case.
const Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

var (
	// ErrInvalid is returned by Decode for an empty string or a character outside
	// the alphabet.
	ErrInvalid = errors.New("base62: invalid character")
	// ErrAlphabet is returned when a custom alphabet is not exactly 62 unique
	// ASCII bytes.
	ErrAlphabet = errors.New("base62: alphabet must be 62 unique bytes")
)

// defaultDecodeTable maps every byte to its index in Alphabet (-1 = not in the
// alphabet). It is built once at package init from the constant Alphabet, so
// Decode does not rebuild it per call. The table is read-only after init.
var defaultDecodeTable [256]int8

func init() {
	// Alphabet is a package constant known to be 62 unique bytes, so this never
	// fails; buildDecodeTable mirrors the validation in DecodeWithAlphabet to
	// keep a single source of truth for byte->index mapping.
	defaultDecodeTable = buildDecodeTable(Alphabet)
}

// Encode returns the base-62 representation of id using the default Alphabet.
func Encode(id uint64) string { return EncodeWithAlphabet(id, Alphabet) }

// EncodeWithAlphabet encodes id using the supplied 62-byte alphabet. It assumes
// the alphabet is valid (62 unique bytes); use a validated alphabet (the
// package Alphabet, or one you checked with DecodeWithAlphabet).
func EncodeWithAlphabet(id uint64, alphabet string) string {
	if id == 0 {
		return string(alphabet[0])
	}
	// uint64 max is 18,446,744,073,709,551,615 -> at most 11 base-62 chars.
	var buf [12]byte
	n := len(buf)
	for id > 0 {
		n--
		buf[n] = alphabet[id%62]
		id /= 62
	}
	return string(buf[n:])
}

// Decode parses a base-62 string (default Alphabet) back to its integer. It
// uses the package-init defaultDecodeTable, so it does not rebuild the lookup
// table per call.
func Decode(s string) (uint64, error) { return decodeWithTable(s, &defaultDecodeTable) }

// DecodeWithAlphabet decodes s using the supplied alphabet, which must be 62
// unique bytes (else ErrAlphabet). An empty string or any byte outside the
// alphabet yields ErrInvalid. The byte->index table is built per call, so this
// path supports any valid custom alphabet.
func DecodeWithAlphabet(s, alphabet string) (uint64, error) {
	if len(alphabet) != 62 {
		return 0, ErrAlphabet
	}
	idx, ok := buildDecodeTableErr(alphabet)
	if !ok {
		return 0, ErrAlphabet // duplicate byte in alphabet
	}
	return decodeWithTable(s, &idx)
}

// buildDecodeTable returns the byte->index mapping for a 62-byte alphabet,
// assuming the alphabet has no duplicates. It is used for known-good alphabets
// (the package Alphabet at init).
func buildDecodeTable(alphabet string) [256]int8 {
	var idx [256]int8
	for i := range idx {
		idx[i] = -1
	}
	for i := 0; i < 62; i++ {
		idx[alphabet[i]] = int8(i)
	}
	return idx
}

// buildDecodeTableErr is buildDecodeTable with duplicate detection; the ok
// flag is false if the alphabet repeats a byte.
func buildDecodeTableErr(alphabet string) ([256]int8, bool) {
	var idx [256]int8
	for i := range idx {
		idx[i] = -1
	}
	for i := 0; i < 62; i++ {
		c := alphabet[i]
		if idx[c] != -1 {
			return idx, false // duplicate byte in alphabet
		}
		idx[c] = int8(i)
	}
	return idx, true
}

// decodeWithTable walks s using a prebuilt byte->index table (-1 = invalid),
// returning ErrInvalid on an empty string or any out-of-alphabet byte.
func decodeWithTable(s string, idx *[256]int8) (uint64, error) {
	if len(s) == 0 {
		return 0, ErrInvalid
	}
	var val uint64
	for i := 0; i < len(s); i++ {
		d := idx[s[i]]
		if d < 0 {
			return 0, ErrInvalid
		}
		val = val*62 + uint64(d)
	}
	return val, nil
}
