# xlo: slice helpers

Small generic slice utilities. Two flavours per helper: the plain one and a
`Lo`/`Lop` variant used internally for benchmark comparison. Pure standard
library.

## Usage

- `Uniq[T comparable](collection []T) []T` dedupe, order-preserving.
- `LoUniq[T comparable](collection []T) []T` same, alternate implementation.
- `LoMap[T, R any](collection []T, iteratee func(item T, index int) R) []R` transform.
- `LopMap[T, R any](collection []T, iteratee func(item T, index int) R) []R` parallel map.

## Example

```go
import "github.com/v8fg/kit4go/xlo"

xlo.Uniq([]int{1, 2, 2, 3})                       // [1 2 3]
xlo.LoMap([]int{1, 2, 3}, func(v, _ int) int { return v * 2 })  // [2 4 6]
```
