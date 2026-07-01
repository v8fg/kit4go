# bit: bit operations

Generic bit-twiddling helpers over integer types. Some are bithacks
(graphics.stanford.edu/~seander/bithacks.html). Pure standard library, zero
allocation on the hot path.

The `Number` constraint covers signed and unsigned integers.

## Usage

- `Min[T ~int](x, y T) T`, `Max[T ~int](x, y T) T` branchless min/max.
- `Abs[T ~int](n T) T` absolute value.
- `HasOppositeSigns[T Number](x, y T) bool` true if signs differ.
- `CountOneBit[T Number](num T) int` population count (Hamming weight).
- `IsPowerOfTwo[T Number](num T) bool`.
- `RightOneBitNum[T Number](num T) T` lowest set bit isolated (`num & -num`).
- `LeftOneBitNum[T Number](num T) T` highest set bit isolated.
- `NextHighestPowerOfTwo[T Number](num T) T` round up to next power of two.
- `PreHighestPowerOfTwo[T Number](num T) T` round down to previous power of two.
- `MaxBits[T Number](num T) int` bit width needed to represent `num`.
- `GetBit[T Number](x T, y int) T` value of bit `y`.
- `Swap[T Number](nums []T)` swap two elements by index.
- `Sum[T Number](x, y T) T`.

## Example

```go
import "github.com/v8fg/kit4go/bit"

bit.CountOneBit(0b1011)              // 3
bit.NextHighestPowerOfTwo(1000)      // 1024
bit.IsPowerOfTwo(64)                 // true
```
