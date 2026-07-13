# bitset

Compact set of non-negative integers using a bit array — 1/64 the memory of a
map-based set. O(1) Set/Test, O(n/64) Len/ToSlice. Supports Union/Intersect.

Pure standard library. Uses: flag bitmasks, small-ID membership, bloom-filter
building blocks, deduplication of bounded integer spaces.

## Quick start

```go
import "github.com/v8fg/kit4go/bitset"

bs := bitset.New(1024)
bs.Set(5)
bs.Set(100)
bs.Test(5)   // true
bs.Test(6)   // false
bs.Len()      // 2
bs.ToSlice()  // [5, 100]
```
