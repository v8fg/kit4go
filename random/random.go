package random

import (
	"math"
	"math/rand"
	"strings"
	"unsafe"
)

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"                // 52 characters
const letterDigitBytes = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ" // 62 characters
const (
	letterIdxBits = 6                             // 6 bits to represent a letter index
	letterIdxMask = 1<<letterIdxBits - 1          // All 1-bits, as many as letterIdxBits
	letterIdxMax  = letterIdxMask / letterIdxBits // # of letter indices fitting in 63 bits
)

// maxBits returns the maximum number of bits can represent the number.
func maxBits(n int) int {
	if n <= 0 {
		return 0
	}
	ret := int(math.Ceil(math.Log2(float64(n))))
	// the power of 2
	if n&(n-1) == 0 {
		ret++
	}
	return ret
}

// RandStringWithLetter only returns a random string(uppercase or lowercase) of length n, with no numbers.
//
//	more details ref: https://stackoverflow.com/questions/22892120/how-to-generate-a-random-string-of-a-fixed-length-in-go/31832326#31832326
func RandStringWithLetter(n int) string {
	return randStringWithLetterDigits(n, false)
}

// RandStringWithLetterDigits only returns a random string(uppercase or lowercase) of length n.
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
	// A localRand.Int63() generates 63 random bits, enough for letterIdxMax characters!
	for i, cache, remain := n-1, localRand.Int63(), letterIdxMax; i >= 0; {
		if remain == 0 {
			cache, remain = localRand.Int63(), letterIdxMax
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
func RandStringInCharset(n int, charset []rune) string {
	idxBits := maxBits(len(charset))
	idxMask := int64(1<<idxBits - 1)
	idxMax := idxMask / int64(idxBits)

	b := strings.Builder{}
	b.Grow(n)

	for i, cache, remain := n-1, localRand.Int63(), idxMax; i >= 0; {
		// round repeat
		if remain == 0 || cache == 0 {
			cache, remain = localRand.Int63(), idxMax
		}
		if idx := int(cache & idxMask); idx < len(charset) {
			b.WriteRune(charset[idx])
			i--
		}
		cache >>= idxBits
		remain--
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
		for i := maxBits(kind); kind > 0; kind &= kind - 1 {
			i--
			posIndex = append(posIndex, i)
		}
	}

	var ik int
	for i := 0; i < n; i++ {
		ik = RandIn(posIndex)
		count, base := characters[ik][0], characters[ik][1]
		result[i] = uint8(base + rand.Intn(count))
	}
	return result
}

// RandIn returns one random value from the given slice.
// If empty slice will panic.
func RandIn[T any](slice []T) T {
	n := len(slice)
	if n == 0 {
		panic("slice nil")
	}

	idxBits := maxBits(n)
	idxMask := int64(1<<idxBits - 1)
	idx := int(localRand.Int63()&idxMask) % n
	return slice[idx]
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
	for i := 0; i < size; i++ {
		j := localRand.Intn(i + 1)
		m[i] = m[j]
		m[j] = i
	}

	ret := make([]T, n)
	for i := 0; i < n; i++ {
		ret[i] = slice[m[i]]
	}
	return ret
}

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
	for i := 0; i < n; i++ {
		dfIdx[i] = byte(uid % LenLetterDigitBytes)
		dfIdx[i] = (dfIdx[i] + byte(i)*dfIdx[0]) % LenLetterDigitBytes
		uid = uid / LenLetterDigitBytes
	}

	// Confusion
	for i := 0; i < n; i++ {
		idx := (byte(i) * prime) % byte(n)
		code[i] = letterDigitBytes[dfIdx[idx]]
	}
	return *(*string)(unsafe.Pointer(&code))
}
