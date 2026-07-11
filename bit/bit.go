// Package bit contains some bit ops, some bithacks ref: https:graphics.stanford.edu/~seander/bithacks.html.
package bit

import "errors"

const intSize = 32 << (^uint(0) >> 63) // 32 or 64

// ErrNegativeBit is returned when a bit operation is given a negative bit index.
// Callers may branch on it with errors.Is.
var ErrNegativeBit = errors.New("bit: negative bit index")

// Number marks the integer number or underlying integer number.
type Number interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64
}

// HasOppositeSigns detects if two integers have opposite signs.
// If x and y have opposite signs, returns true.
func HasOppositeSigns[T Number](x, y T) bool {
	return (x ^ y) < 0
}

// Min returns the minimum value of x and y, only support the type int.
//
// diff = x - y
// x > y, y + (diff &  0) => y
// x < y, y + (diff & -1) => x
func Min[T ~int](x, y T) T {
	x = x - y
	y += x & (x >> (intSize - 1))
	return y

}

// Max returns the maximum value of x and y, only support the type int.
//
// diff = x - y
// x > y, x - (diff &  0) => x
// x < y, x - (diff & -1) => y
func Max[T ~int](x, y T) T {
	y = x - y
	x -= y & (y >> (intSize - 1))
	return x
}

// Abs returns the absolute value of n, only support the type int.
//
// The number n value shall in [math.MinInt+1, math.Max], if equals math.MinInt will be overflow.
func Abs[T ~int](n T) T {
	return (n ^ (n >> (intSize - 1))) - (n >> (intSize - 1))
}

// CountOneBit counts the one bit count in the num.
func CountOneBit[T Number](num T) (ones int) {
	for ; num != 0; num &= num - 1 {
		ones++
	}
	return
}

// IsPowerOfTwo checks the num is the power of two.
func IsPowerOfTwo[T Number](num T) bool {
	return num > 0 && num&(num-1) == 0
}

// RightOneBitNum returns the number represented by the rightmost one in the binary representation,
// return number greater than or equals to the num, power of two.
//
// Like: 0100 -> 0100, 1100 -> 0100
func RightOneBitNum[T Number](num T) T {
	return num & (^num + 1)
}

// LeftOneBitNum returns the number represented by the leftmost one in the binary representation,
// return number greater than or equals to the num, power of two.
//
// Like: 0100 -> 0100, 1100 -> 1000
func LeftOneBitNum[T Number](num T) T {
	if (num & (num - 1)) == 0 {
		return num
	}

	// num |= num >> 1
	// num |= num >> 2
	// num |= num >> 4
	// num |= num >> 8
	// num |= num >> 16
	// num |= num >> 32
	for i := range 6 {
		num |= num >> (1 << i)
	}
	return (num + 1) >> 1
}

// NextHighestPowerOfTwo computes the next highest power of 2 of 64-bit number, no less than the num.
//
// Like: 0100 -> 0100, 0101 -> 1000
func NextHighestPowerOfTwo[T Number](num T) T {
	if (num & (num - 1)) == 0 {
		return num
	}

	num--
	// num |= num >> 1
	// num |= num >> 2
	// num |= num >> 4
	// num |= num >> 8
	// num |= num >> 16
	// num |= num >> 32
	for i := range 6 {
		num |= num >> (1 << i)
	}
	num++
	return num
}

// PreHighestPowerOfTwo computes the pre highest power of 2 of 64-bit number, no less than the num.
//
// Like: 0100 -> 0100, 0101 -> 0100
func PreHighestPowerOfTwo[T Number](num T) T {
	return LeftOneBitNum(num)
}

// Swap swaps the first two numbers in the slice.
func Swap[T Number](nums []T) {
	if len(nums) < 2 {
		return // no-op: need at least two elements to swap
	}
	nums[0] ^= nums[1]
	nums[1] ^= nums[0]
	nums[0] ^= nums[1]
}

// Sum returns the sum of x, y.
func Sum[T Number](x, y T) T {
	for y != 0 {
		carry := (x & y) << 1
		x ^= y
		y = carry
	}
	return x
}

// MaxBits returns the maximum bits can represent the number, if the number is power of two, plus one.
//
// Like: 0->0, 1->1, 2->2, 3->2, 4->3, 5->3, 8->4
func MaxBits[T Number](num T) (bits int) {
	for ; num >= 1; num >>= 1 {
		bits++
	}
	return bits
}

// Bit returns the specified bit of a binary number.
// Returns ErrNegativeBit if y is less than 0.
func Bit[T Number](x T, y int) (T, error) {
	if y < 0 {
		var zero T
		return zero, ErrNegativeBit
	}
	return (x >> y) & 1, nil
}

// ReverseBit reverses the specified bit of a binary number, 0 to 1 and 1 to 0.
// Returns ErrNegativeBit if y is less than 0.
func ReverseBit[T Number](x T, y int) (T, error) {
	if y < 0 {
		var zero T
		return zero, ErrNegativeBit
	}
	return x ^ (1 << y), nil
}

// SetBit sets the specified bit of a binary number to 1.
// Returns ErrNegativeBit if y is less than 0.
func SetBit[T Number](x T, y int) (T, error) {
	if y < 0 {
		var zero T
		return zero, ErrNegativeBit
	}
	return x | (1 << y), nil
}

// UnsetBit sets the specified bit of a binary number to 0.
// Returns ErrNegativeBit if y is less than 0.
func UnsetBit[T Number](x T, y int) (T, error) {
	if y < 0 {
		var zero T
		return zero, ErrNegativeBit
	}
	return x & ^(1 << y), nil
}
