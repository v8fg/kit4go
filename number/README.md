# number: rounding & binary conversion

Floating-point rounding with explicit modes, plus little/big-endian byte
conversions for integer and float types. Pure standard library.

## Usage

### rounding

- `Round[T Float](f T, precision uint) float64` round half away from zero.
- `RoundToEven[T Float](f T, precision uint) float64` banker's rounding.
- `RoundFloor[T Float](f T, precision uint) float64` toward -inf.
- `RoundCeil[T Float](f T, precision uint) float64` toward +inf.
- `RoundTrunc[T Int|Uint|Float](f T, precision int) T` toward zero.
- `RoundTruncStr[T Int|Uint|Float](f T, precision int) string`.
- `RestoreToRealNumberStr[T Int|Uint|Float](f T) string` undo a truncated form.

`SetRegForNumber(useRegForNumber7 bool)` toggles an internal formatting knob.

### binary

- `ToBytes[T BinaryType](data T) ([]byte, error)` big-endian.
- `ToBytesLittleEndian[T BinaryType](data T) ([]byte, error)`.
- `BytesToData[T BinaryType](data []byte, kindAnyData T) (T, error)` big-endian.
- `BytesToDataLittleEndian[T BinaryType](data []byte, kindAnyData T) (T, error)`.
- `BytesToUint(b []byte) uint`.

`BinaryType` covers fixed-width numeric types.

## Example

```go
import "github.com/v8fg/kit4go/number"

number.Round(3.14159, 2)            // 3.14
number.RoundToEven(2.5, 0)          // 2
bs, _ := number.ToBytes(int32(1))   // [0 0 0 1]
```
