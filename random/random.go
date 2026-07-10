package random

import (
	"errors"
	"math/rand/v2"
	"strings"
	"unsafe"
)

// ErrEmptySlice is returned by RandIn when the input slice is empty.
var ErrEmptySlice = errors.New("random: empty slice")

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"                // 52 characters
const letterDigitBytes = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ" // 62 characters
const (
	letterIdxBits = 6                             // 6 bits to represent a letter index
	letterIdxMask = 1<<letterIdxBits - 1          // All 1-bits, as many as letterIdxBits
	letterIdxMax  = letterIdxMask / letterIdxBits // # of letter indices fitting in 63 bits
)

// RandStringWithLetter only returns a random string(uppercase or lowercase) of length n, with no numbers.
//
//	more details ref: https://stackoverflow.com/questions/22892120/how-to-generate-a-random-string-of-a-fixed-length-in-go/31832326#31832326
//
// This is a convenience over the package's math/rand/v2 global source and is NOT
// cryptographically secure; for secrets/tokens use the crypto sources
// (CryptoInt/CryptoRead/CryptoReadString).
func RandStringWithLetter(n int) string {
	return randStringWithLetterDigits(n, false)
}

// RandStringWithLetterDigits only returns a random string(uppercase or lowercase) of length n.
//
// This is a convenience over the package's math/rand/v2 global source and is NOT
// cryptographically secure; for secrets/tokens use the crypto sources
// (CryptoInt/CryptoRead/CryptoReadString).
func RandStringWithLetterDigits(n int) string {
	return randStringWithLetterDigits(n, true)
}

func randStringWithLetterDigits(n int, containDigits bool) string {
	if n <= 0 {
		return ""
	}

	charset := letterBytes
	if containDigits {
		charset = letterDigitBytes
	}

	b := make([]byte, n)
	// A rand.Int64() generates 63 random bits, enough for letterIdxMax characters!
	for i, cache, remain := n-1, rand.Int64(), letterIdxMax; i >= 0; {
		if remain == 0 {
			cache, remain = rand.Int64(), letterIdxMax
		}
		if idx := int(cache & letterIdxMask); idx < len(charset) {
			b[i] = charset[idx]
			i--
		}
		cache >>= letterIdxBits
		remain--
	}
	return *(*string)(unsafe.Pointer(&b))
}

// RandStringInCharset returns a random string of length n with the given charset.
// One character in the charset maybe use 1byte, 2bytes, 3bytes or 4bytes.
//
// An empty charset returns "". The index selection uses rand.IntN, which is
// rejection-free uniform in math/rand/v2 (the previous mask-then-modulo path
// over-sampled low indices when len(charset) was not a power of two).
func RandStringInCharset(n int, charset []rune) string {
	if len(charset) == 0 {
		return ""
	}

	b := strings.Builder{}
	b.Grow(n)

	for range n {
		b.WriteRune(charset[rand.IntN(len(charset))])
	}
	return b.String()
}

// RandStringWithKind returns a random string of length n with the given number kind and
// use it`s bits to indicate the inclusion characters kind.
//
//	kind=0000, returns contains the characters '0' - '9', 'A' - 'Z' and 'a' - 'z'
//	kind=0001, returns only contains the characters '0' - '9'
//	kind=0010, returns only contains the characters 'A' - 'Z'
//	kind=0100, returns only contains the characters 'a' - 'z'
//	kind=1000, the same as kind=0000
//	others will be combined the above kinds.
func RandStringWithKind(n int, kind int) []byte {
	characters, result := [][]int{{'9' - '0', '0'}, {'Z' - 'A', 'A'}, {'z' - 'a', 'a'}}, make([]byte, n)
	var posIndex []int
	if kind <= 0 || kind >= 8 {
		posIndex = []int{0, 1, 2}
	} else {
		// kind is a 3-bit mask: bit 0 → digits, bit 1 → uppercase, bit 2 →
		// lowercase. Include each character group whose bit is set, in bit
		// order. (The previous maxBits + clear-lowest-bit loop mapped the
		// lowest set bit to the highest index, which scrambled the groups
		// whenever the set bits were not contiguous — e.g. kind=5 (bits 0+2,
		// digits+lowercase) silently produced lowercase+uppercase instead.)
		for k := range 3 {
			if kind&(1<<k) != 0 {
				posIndex = append(posIndex, k)
			}
		}
	}

	var ik int
	for i := range n {
		ik = MustRandIn(posIndex) // posIndex is non-empty here.
		count, base := characters[ik][0], characters[ik][1]
		// count is the inclusive span (e.g. '9'-'0' = 9). rand.IntN(count)
		// returns [0,count), which would never select the last character
		// ('9', 'Z', 'z'); use count+1 so the full range [base, base+count]
		// is uniformly covered.
		result[i] = uint8(base + rand.IntN(count+1))
	}
	return result
}

// RandIn returns one random value from the given slice.
// An empty slice returns the zero value of T together with ErrEmptySlice.
func RandIn[T any](slice []T) (T, error) {
	var zero T
	n := len(slice)
	if n == 0 {
		return zero, ErrEmptySlice
	}

	// rand.IntN is rejection-free uniform in math/rand/v2; the previous
	// mask-then-modulo (`Int64()&idxMask % n`) over-sampled low indices
	// whenever n was not a power of two (e.g. n=3 -> idx0 ~50%).
	return slice[rand.IntN(n)], nil
}

// MustRandIn is like RandIn but panics on an empty slice. Use it when the
// caller has already guaranteed the slice is non-empty and wants a
// panic-on-programmer-error contract.
func MustRandIn[T any](slice []T) T {
	v, err := RandIn(slice)
	if err != nil {
		panic(err)
	}
	return v
}

// RandNIn returns a specified number of elements random value from the input slice.
func RandNIn[T any](n int, slice []T) []T {
	size := len(slice)
	if n <= 0 {
		return []T{}
	} else if n >= size {
		n = size
	}

	m := make([]int, size)
	for i := range size {
		j := rand.IntN(i + 1)
		m[i] = m[j]
		m[j] = i
	}

	ret := make([]T, n)
	for i := range n {
		ret[i] = slice[m[i]]
	}
	return ret
}

// DefaultSALT is the default salt value for the random code generator, and
// LenLetterDigitBytes is the size of the letter+digit alphabet.
const (
	DefaultSALT         = 89482311 // SALT
	LenLetterDigitBytes = 62
)

// number and nearest prime mapping
var codeLengthNearestPrimeMapping = []uint8{
	2, 2, 3, 2, 3, 3, 5, 5,
	7, 7, 7, 7, 11, 11, 13, 13,
}

// RandUniCodeByUID returns the random string with the length n.
//
//	Warn: the max n shall less than 10.
//	n=1,  max codes count 62
//	n=2,  max codes count 62^2=3844
//	n=3,  max codes count 62^3=238,328
//	n=4,  max codes count 62^4=14,776,336
//	n=5,  max codes count 62^5=916,132,832
//	n=6,  max codes count 62^6=56,800,235,584
//	n=7,  max codes count 62^7=3,521,614,606,208
//	n=8,  max codes count 62^8=218,340,105,584,896
//	n=9,  max codes count 62^9=1.353708655E16
//	n=10, max codes count 62^10=8.392993659E17
func RandUniCodeByUID(uid uint64, n int) string {
	return RandUniCodeByUIDWithSalt(uid, n, DefaultSALT)
}

// RandUniCodeByUIDWithSalt returns the random string with the length n and your salt.
//
//	Warn: the max n shall less than 10.
//	n=1,  max codes count 62
//	n=2,  max codes count 62^2=3844
//	n=3,  max codes count 62^3=238,328
//	n=4,  max codes count 62^4=14,776,336
//	n=5,  max codes count 62^5=916,132,832
//	n=6,  max codes count 62^6=56,800,235,584
//	n=7,  max codes count 62^7=3,521,614,606,208
//	n=8,  max codes count 62^8=218,340,105,584,896
//	n=9,  max codes count 62^9=1.353708655E16
//	n=10, max codes count 62^10=8.392993659E17
func RandUniCodeByUIDWithSalt(uid uint64, n int, slat uint64) string {
	if n <= 0 {
		return ""
	} else if n >= 10 {
		n = 10
	}

	// uid*3 = uid<<1 + uid, Co-prime with character set(letterDigitBytes) length 62
	uid = uid<<1 + uid + slat
	prime := codeLengthNearestPrimeMapping[n]

	code := make([]byte, n)
	dfIdx := make([]byte, n)

	// Diffusion
	for i := range n {
		dfIdx[i] = byte(uid % LenLetterDigitBytes)
		dfIdx[i] = (dfIdx[i] + byte(i)*dfIdx[0]) % LenLetterDigitBytes
		uid = uid / LenLetterDigitBytes
	}

	// Confusion
	for i := range n {
		idx := (byte(i) * prime) % byte(n)
		code[i] = letterDigitBytes[dfIdx[idx]]
	}
	return *(*string)(unsafe.Pointer(&code))
}

// digitBytes is the numeric charset for NumericCode.
const digitBytes = "0123456789"

// NumericCode returns a random n-digit numeric string (leading zeros allowed),
// e.g. a 6-digit SMS/email verification code. n <= 0 returns "".
//
// This is a convenience over the package's math/rand/v2 global source and is NOT
// cryptographically secure; for 2FA use package otp (TOTP/HOTP) or crypto
// sources (CryptoInt/CryptoRead).
func NumericCode(n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, n)
	for i := range b {
		b[i] = digitBytes[IntBetween(0, len(digitBytes))]
	}
	return string(b)
}
