# optional

Generic `Option[T]` — a value that may or may not be present, without the
ambiguity of nil pointers. Pure standard library.

## Quick start

```go
import "github.com/v8fg/kit4go/optional"

name := optional.Some("Alice")
age := optional.None[int]()

name.IsSome()        // true
name.Unwrap()        // "Alice"
age.UnwrapOr(0)      // 0 (fallback)

// From a pointer (nil → None)
opt := optional.FromPtr(findUser(id))
// Back to a pointer (None → nil)
p := opt.ToPtr()

// Transform
doubled := optional.Map(opt, func(v int) int { return v * 2 })
```

## API

| Function/Method | Description |
|-----------------|-------------|
| `Some(v)` | Wrap a value |
| `None[T]()` | Empty |
| `FromPtr(*T)` | nil → None, non-nil → Some(*p) |
| `IsSome() / IsNone()` | Presence check |
| `Get() (T, bool)` | Value + presence |
| `Unwrap()` | Value (panic if None) |
| `UnwrapOr(fallback)` | Value or fallback |
| `UnwrapOrElse(fn)` | Value or fn() |
| `UnwrapOrZero()` | Value or zero |
| `ToPtr() *T` | Inverse of FromPtr |
| `Map(opt, fn)` | Transform if Some |
| `MapOr(opt, fb, fn)` | Transform or fallback |
| `AndThen(opt, fn)` | Flat-map |
| `Equal(a, b, eq)` | Compare (needs eq fn) |

Zero-value is None.
