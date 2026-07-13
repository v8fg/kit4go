# bimap

Generic bidirectional map — O(1) lookup by key OR by value, enforcing a
one-to-one relationship. Pure standard library.

## Quick start

```go
import "github.com/v8fg/kit4go/bimap"

bm := bimap.New[int, string]()
bm.Insert(200, "OK")
bm.Insert(404, "Not Found")

bm.Get(404)         // ("Not Found", true)
bm.GetKey("OK")     // (200, true)
bm.HasKey(500)      // false
bm.Delete(404)      // removes both sides
```

## API

| Method | Description |
|--------|-------------|
| `New[K,V]()` | Empty BiMap |
| `FromMap(map)` | Build from regular map (error on dup values) |
| `Insert(k, v)` | Add pair (error on dup key/value) |
| `MustInsert(k, v)` | Add pair (panic on dup) |
| `Get(k) (V, bool)` | Forward lookup |
| `GetKey(v) (K, bool)` | Reverse lookup |
| `HasKey(k) / HasValue(v)` | Existence check |
| `Delete(k) / DeleteValue(v)` | Remove pair |
| `Len() / Keys() / Values()` | Inspection |
| `Clear()` | Remove all |

Not safe for concurrent use (documented).
