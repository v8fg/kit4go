package loadbalance

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

// op codes for FuzzLifecycle — ops[i] % 3 drives an Add/Remove/Next.
const (
	opAdd    = 0
	opRemove = 1
	opNext   = 2
)

// splitIDs decodes idsBytes as a sequence of newline-terminated id strings.
// Trailing data after the last newline is dropped (incomplete ids are ignored).
// This keeps ids a single []byte fuzz argument, which go-native fuzz requires.
func splitIDs(idsBytes []byte) []string {
	if len(idsBytes) == 0 {
		return nil
	}
	var ids []string
	start := 0
	for i, b := range idsBytes {
		if b == '\n' {
			ids = append(ids, string(idsBytes[start:i]))
			start = i + 1
		}
	}
	return ids
}

// decodeWeights unpacks a []byte of little-endian int32 into []int. The last
// partial 4-byte group is ignored. Each decoded value is then clamped to [1, 50]
// by the caller as needed.
func decodeWeights(wb []byte) []int {
	n := len(wb) / 4
	out := make([]int, 0, n)
	for i := 0; i+4 <= len(wb); i += 4 {
		out = append(out, int(int32(binary.LittleEndian.Uint32(wb[i:]))))
	}
	return out
}

// FuzzLifecycle drives an arbitrary mix of Add/Remove/Next across all four
// strategies. The core invariants under fuzz:
//
//  1. No panic — any weight (negative, zero, huge), any id (empty, duplicate,
//     non-ASCII), any op interleaving must not crash.
//  2. Ordering soundness — every Next() result is one of the currently-present
//     entries; a Remove(x) means x is never returned again until re-Added.
//  3. Empty balancer returns ok=false; non-empty returns ok=true.
//
// Seeds cover: empty balancer; negative/zero weights (normalized to 1); id
// collisions forcing the replace path; remove-miss and remove-present; repeated
// Next on a stable set; mixed add/remove churn.
func FuzzLifecycle(f *testing.F) {
	// Seeds: ops, ids (\n-separated), packed LE int32 weights, strategy.
	type seed struct {
		ops      []byte
		ids      string
		weight   []byte
		strategy int
	}
	mkWeight := func(ws ...uint32) []byte {
		b := make([]byte, 4*len(ws))
		for i, w := range ws {
			binary.LittleEndian.PutUint32(b[i*4:], w)
		}
		return b
	}
	seeds := []seed{
		// Empty — only Next on a balancer with no entries.
		{ops: []byte{opNext, opNext}, ids: "", strategy: int(StrategySmoothWeightedRR)},
		// Single add then a burst of Next across every strategy.
		{ops: []byte{opAdd, opNext, opNext, opNext}, ids: "a\n", weight: mkWeight(1), strategy: int(StrategySmoothWeightedRR)},
		{ops: []byte{opAdd, opNext, opNext, opNext}, ids: "a\n", weight: mkWeight(1), strategy: int(StrategyRoundRobin)},
		{ops: []byte{opAdd, opNext, opNext, opNext}, ids: "a\n", weight: mkWeight(1), strategy: int(StrategyRandom)},
		{ops: []byte{opAdd, opNext, opNext, opNext}, ids: "a\n", weight: mkWeight(1), strategy: int(StrategyWeightedRandom)},
		// Negative / zero weights must normalize to 1, never divide-by-zero or panic.
		{ops: []byte{opAdd, opAdd, opNext, opNext}, ids: "a\nb\n", weight: mkWeight(0, 0xFFFFFFFB), strategy: int(StrategySmoothWeightedRR)}, // 0, -5
		{ops: []byte{opAdd, opAdd, opNext}, ids: "a\nb\n", weight: mkWeight(0xFFFFFFFF, 0), strategy: int(StrategyWeightedRandom)},           // -1, 0
		// Id collision → replace path (weight reset, no dup, no totalWeight drift).
		{ops: []byte{opAdd, opAdd, opAdd, opNext, opNext}, ids: "a\na\na\n", weight: mkWeight(1, 2, 3), strategy: int(StrategySmoothWeightedRR)},
		// Remove present, then Next must never see it again.
		{ops: []byte{opAdd, opAdd, opRemove, opNext, opNext, opNext}, ids: "a\nb\nb\n", weight: mkWeight(1, 1, 1), strategy: int(StrategyRoundRobin)},
		// Remove a missing id is a no-op.
		{ops: []byte{opAdd, opRemove, opNext}, ids: "a\nzzz\n", weight: mkWeight(1), strategy: int(StrategySmoothWeightedRR)},
		// Churn: add three, remove one, re-add a duplicate id, heavy Next.
		{ops: []byte{opAdd, opAdd, opAdd, opRemove, opAdd, opNext, opNext, opNext, opNext, opNext},
			ids: "x\ny\nz\ny\nx\n", weight: mkWeight(2, 1, 1, 1, 4), strategy: int(StrategySmoothWeightedRR)},
		// Empty-string and non-ASCII ids.
		{ops: []byte{opAdd, opAdd, opNext, opNext}, ids: "\n日本語\n", weight: mkWeight(1, 1), strategy: int(StrategySmoothWeightedRR)},
	}
	for _, s := range seeds {
		f.Add(s.ops, []byte(s.ids), s.weight, s.strategy)
	}

	f.Fuzz(func(t *testing.T, ops []byte, idsBytes []byte, weightBytes []byte, strategy int) {
		if len(ops) == 0 || strategy < 0 || strategy > int(StrategyWeightedRandom) {
			return
		}
		// Cap the work so a runaway seed (huge ops slice) can't stall the corpus.
		if len(ops) > 1<<14 {
			return
		}

		b := New[string](strID, nil, WithStrategy[string](Strategy(strategy)))

		// idSet mirrors the balancer's membership so we can assert every Next()
		// result is a currently-present entry (ordering soundness after Remove).
		idSet := make(map[string]bool)
		ids := splitIDs(idsBytes)
		weights := decodeWeights(weightBytes)
		idIdx, wIdx := 0, 0

		for _, op := range ops {
			switch op % 3 {
			case opAdd:
				if idIdx >= len(ids) {
					return
				}
				id := ids[idIdx]
				idIdx++
				w := 1
				if wIdx < len(weights) {
					w = weights[wIdx]
					wIdx++
				}
				b.Add(Entry[string]{Value: id, Weight: w})
				idSet[id] = true
			case opRemove:
				if idIdx >= len(ids) {
					return
				}
				id := ids[idIdx]
				idIdx++
				b.Remove(id)
				delete(idSet, id)
			case opNext:
				v, ok := b.Next()
				require.Equal(t, len(idSet) > 0, ok, "Next ok=%v but idSet has %d entries", ok, len(idSet))
				if ok {
					require.True(t, idSet[v], "Next returned %q which is not in the present set %v", v, idSet)
				}
			}
		}
		// Final consistency: Len matches the mirror set size.
		require.Equal(t, len(idSet), b.Len(), "Len drifted from mirror set")
	})
}

// FuzzSWRRDistribution checks the canonical nginx SWRR invariant under fuzzed
// weights: over exactly totalWeight consecutive picks, each entry is selected
// precisely weight times (exact proportionality), and the picks are a
// permutation-by-multiplicity of the entry set (roundtrip consistency).
//
// Weights are clamped to [1, 50] and the entry count to [1, 8] so totalWeight
// stays bounded (≤ 400) and the cycle-completion loop cannot stall the corpus.
func FuzzSWRRDistribution(f *testing.F) {
	mkWeight := func(ws ...uint32) []byte {
		b := make([]byte, 4*len(ws))
		for i, w := range ws {
			binary.LittleEndian.PutUint32(b[i*4:], w)
		}
		return b
	}
	seeds := [][]byte{
		mkWeight(5, 1, 1),        // canonical nginx example → 5 a, 1 b, 1 c
		mkWeight(3, 1),           // 2-entry 3:1
		mkWeight(1, 1, 1, 1),     // uniform → one each
		mkWeight(1),              // single entry → always itself
		mkWeight(7, 3),           // 70/30
		mkWeight(2, 2, 2),        // equal mid weights
		mkWeight(50, 1),          // extreme ratio
		mkWeight(4, 3, 2, 1),     // descending
		mkWeight(1, 2, 3, 4),     // ascending
		mkWeight(10, 10, 10, 10), // large equal
	}
	for _, w := range seeds {
		f.Add(w)
	}

	f.Fuzz(func(t *testing.T, weightBytes []byte) {
		weights := decodeWeights(weightBytes)
		if len(weights) == 0 || len(weights) > 8 {
			return
		}
		// Clamp each weight into [1, 50]; addLocked would normalize <=0 to 1, but
		// clamping keeps the cycle length bounded and avoids huge totalWeight.
		entries := make([]Entry[string], 0, len(weights))
		totalWeight := 0
		validValues := make(map[string]int, len(weights))
		for i, w := range weights {
			if w < 1 {
				w = 1
			}
			if w > 50 {
				w = 50
			}
			id := string(rune('a' + i))
			entries = append(entries, Entry[string]{Value: id, Weight: w})
			validValues[id] = w
			totalWeight += w
		}
		if totalWeight == 0 || totalWeight > 400 {
			return
		}

		b := New(strID, entries) // default SWRR

		counts := make(map[string]int, len(entries))
		for i := 0; i < totalWeight; i++ {
			v, ok := b.Next()
			require.True(t, ok, "Next returned ok=false on a non-empty balancer")
			require.Contains(t, validValues, v, "Next returned %q not in entry set", v)
			counts[v]++
		}

		// Exact proportionality: one full cycle selects each entry exactly weight times.
		for id, w := range validValues {
			require.Equal(t, w, counts[id],
				"SWRR proportional invariant broken: %q selected %d times, want %d (weights=%v)",
				id, counts[id], w, weights)
		}

		// Ordering soundness: every selected id was a valid entry (roundtrip).
		require.Len(t, counts, len(entries), "distinct selected ids != entry count")
		// Determinism check: a fresh balancer over the same weights reproduces the
		// exact same pick counts (SWRR is deterministic modulo currentWeight).
		b2 := New(strID, entries)
		b2Counts := make(map[string]int, len(entries))
		for i := 0; i < totalWeight; i++ {
			v, _ := b2.Next()
			b2Counts[v]++
		}
		require.Equal(t, counts, b2Counts, "SWRR non-deterministic across balancer instances")
	})
}
