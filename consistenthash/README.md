# consistenthash

Rendezvous hashing (HRW — Highest Random Weight) for generic, typed nodes. Pure
standard library (`hash/fnv` + `sync`).

## Why

Consistent hashing maps keys to nodes such that adding or removing one node
relocates only ~1/N of keys — essential when the fleet scales and you cannot
afford a mass redistribution. Rendezvous hashing achieves this with **no virtual
nodes and no ring**: the responsible node for key `k` is simply
`argmax over nodes of hash(id(node) || k)`. The rule is one line, the
distribution is uniform, and membership changes are minimal.

Lookups are O(N). That is the right tradeoff for the tens-to-low-hundreds of
nodes typical of shard routing; for very large sets use Maglev or a ring.

## API

```go
m := consistenthash.New(
    func(s string) string { return s },     // id extractor
    consistenthash.WithNodes("shard-1", "shard-2", "shard-3"),
)

node, ok := m.Get("auction-42")        // primary node for the key
replicas := m.GetN("auction-42", 3)    // top-3 (primary + fallbacks)
m.Add("shard-4")                       // scale out: few keys move
m.Remove("shard-2")                    // drain: only its keys move
m.Len()
```

| Method | Behavior |
|---|---|
| `New(id, opts...) *Map[T]` | Build a map; `id` extracts a node's stable string identity |
| `WithNodes(...)` / `WithHash(h)` | Seed nodes / override the hash (default FNV-1a 64) |
| `Add(...)` / `Remove(node)` | Mutate membership (duplicates by id ignored) |
| `Get(key) (T, bool)` | The responsible node (highest HRW score); `false` if empty |
| `GetN(key, n) []T` | Top-n distinct nodes by score — replication / fallback list |
| `Len() int` | Node count |

All methods are safe for concurrent use.

## Properties (verified by tests)

- **Deterministic**: same key + same membership always maps to the same node.
- **Stable on add**: adding one node to four moves ~1/5 of keys (asserted
  within bounds over 5000 keys).
- **Stable on remove**: removing a node moves only the keys it owned
  (non-removed-node keys stay put).
- **Balanced**: 40000 keys across four nodes land within ±15% of N/4 each.
- **Replication**: `GetN` returns distinct nodes, ordered by score, with
  `GetN[0] == Get`.

## Ad-tech uses

- Route an **auction / user hash** to a bidder shard; scaling the fleet moves
  the fewest possible auctions.
- Sticky upstream selection (a user's traffic stays on one SSP path).
- Partition a keyspace across workers with minimal redistribution on deploy.

## Testing

100% statement coverage, `-race` clean, including a concurrent add/read churn
run. Uses a statistical balance check, so run counts are generous to avoid
flakes.

```bash
go test -race -cover ./consistenthash/...
```
