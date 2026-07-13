# iterx

Functional combinators over Go 1.23+ range-over-func iterators
([`iter.Seq`](https://pkg.go.dev/iter) / `iter.Seq2`): Map, Filter, Take, Drop,
Collect, Reduce, Chain, Zip, Range, Seq2Keys, Seq2Values. Pure standard library.

Go 1.23 introduced range-over-func and the `iter` package, but the stdlib ships
no combinators — you can `for v := range seq`, but there is no `Map` / `Filter` /
`Take` / `Reduce`. This package fills that gap with lazy, composable pipelines.

## Quick start

```go
import (
	"slices"
	"github.com/v8fg/kit4go/iterx"
)

nums := slices.Values([]int{1, 2, 3, 4, 5, 6})

// Lazy pipeline: filter evens, double them, take first two, materialize.
out := iterx.Collect(
	iterx.Take(
		iterx.Map(
			iterx.Filter(nums, func(v int) bool { return v%2 == 0 }),
			func(v int) int { return v * 2 },
		),
		2,
	),
) // [4 8]

// Reduce to a scalar.
sum := iterx.Reduce(nums, 0, func(acc, v int) int { return acc + v }) // 21

// Generate a range.
iterx.Collect(iterx.Range(0, 10, 2)) // [0 2 4 6 8]

// Zip two sequences into pairs.
for p := range iterx.Zip(slices.Values([]int{1, 2}), slices.Values([]string{"a", "b"})) {
	fmt.Println(p.First, p.Second) // 1 a / 2 b
}
```

## API

| Function | Kind | Description |
|----------|------|-------------|
| `Map(seq, fn)` | lazy | Apply `fn` to each element |
| `Filter(seq, pred)` | lazy | Keep elements where `pred` is true |
| `Take(seq, n)` | lazy | First `n` elements (n ≤ 0 → empty) |
| `Drop(seq, n)` | lazy | All but the first `n` elements |
| `Chain(seqs...)` | lazy | Concatenate multiple seqs in order |
| `Zip(a, b)` | lazy | Pair elements; stops at the shorter |
| `Range(start, end, step)` | lazy | Integer sequence (step 0 → empty) |
| `Seq2Keys(seq2)` | lazy | Keys of an `iter.Seq2` |
| `Seq2Values(seq2)` | lazy | Values of an `iter.Seq2` |
| `Collect(seq)` | terminal | Materialize into `[]T` (nil if empty) |
| `Reduce(seq, init, fn)` | terminal | Left fold to a scalar |

## Notes

- **Laziness**: `Map` / `Filter` / `Take` / `Drop` / `Chain` / `Zip` / `Range`
  return an `iter.Seq` that pulls from the source only when iterated. `Take(n)`
  over an infinite source realizes exactly `n` elements.
- **Early termination**: when a consumer stops ranging (`break` / `return`), the
  `yield` callback returns `false` and the upstream stops immediately. `Zip` uses
  `iter.Pull` and defers `stop()` so the second iterator is always released.
- **`Collect` returns `nil`** for an empty iterator (not a non-nil empty slice),
  matching the `sliceutil` convention.
- `Zip` pairs via `tuple.Pair[A, B]` (fields `First`, `Second`) — compose, don't
  redefine.
- The combinators are **single-threaded** by design. An `iter.Seq` is not safe to
  iterate concurrently; this is inherent to the Go iterator contract.
