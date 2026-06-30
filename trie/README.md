# trie

A generic, concurrency-safe prefix tree for string-keyed lookups: exact match,
longest-prefix match, and prefix-scan. Pure standard library.

## API

```go
tr := trie.New[string]()
tr.Insert("ssp/rubicon/bid", "endpoint-A")
tr.Insert("ssp/rubicon", "endpoint-B")

v, key, _ := tr.LongestPrefix("ssp/rubicon/bid/123")  // "endpoint-A", "ssp/rubicon/bid"
v, _, _   := tr.LongestPrefix("ssp/rubicon/x")          // "endpoint-B"
_, _, ok  := tr.LongestPrefix("other")                   // false
```

| Symbol | Behavior |
|---|---|
| `New[V]()` | Empty trie |
| `Insert(key, val)` | Set key=val |
| `Get(key) (V, bool)` | Exact match |
| `Has(key) bool` | Exact match check |
| `LongestPrefix(query) (V, key, bool)` | Longest configured prefix of query |
| `Delete(key) bool` | Remove + prune empty nodes |
| `KeysWithPrefix(prefix) []string` | All keys under prefix |
| `Len() int` | Key count |

Keys are "/"-segmented (like URL paths). Thread-safe.

## Ad-tech uses

- **Domain/URL routing** — match the longest configured domain path for a request.
- **SSP endpoint classification** — hierarchical SSP/publisher/creative routing.
- **Keyword blocklists** — prefix-scan for blocked terms.

## Testing

98% coverage, `-race` clean. Covers insert/get/has, longest-prefix (nested,
non-matching, empty), delete (with node pruning), keys-with-prefix, overwrite,
len, and a concurrent insert+read stress run.

```bash
go test -race -cover ./trie/...
```
