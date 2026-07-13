# tuple

Generic `Pair[A, B]` and `Triple[A, B, C]` for bundling 2 or 3 values of
potentially different types. Pure standard library.

When all fields are `comparable`, the tuple can be used as a map key.

## Quick start

```go
import "github.com/v8fg/kit4go/tuple"

p := tuple.NewPair("US", 331)
p.First   // "US"
p.Second  // 331

// As a compound map key
m := map[tuple.Pair[string, int]]string{}
m[tuple.NewPair("a", 1)] = "first"

// Triple
t := tuple.NewTriple("x", 42, true)
a, b, c := t.Values()
```

## API

| Type | Function/Method | Description |
|------|----------------|-------------|
| `Pair[A, B]` | `NewPair(a, b)` | Construct |
| | `.Values() (A, B)` | Destructure |
| `Triple[A, B, C]` | `NewTriple(a, b, c)` | Construct |
| | `.Values() (A, B, C)` | Destructure |
