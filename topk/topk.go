// Package topk maintains the top-K most frequent items in a stream using a
// min-heap of size K — O(log K) per touch, O(K) memory. Pure standard library.
//
// Unlike hotkey (which detects heavy keys in a sliding time window), topk tracks
// cumulative frequency and returns the K items with the highest counts at any
// point. It is the standard "leaderboard" data structure for streaming analytics.
//
// Ad-tech uses: top SSPs / creatives / placements by request volume, real-time
// leaderboard maintenance, heavy-hitter reporting (complement to countmin's
// approximate per-key estimation).
package topk

import (
	"cmp"
	"container/heap"
	"slices"
	"sync"
)

// candidateFactor bounds how many non-admitted candidate counts are retained
// alongside the K in-heap entries. The counts map holds at most
// k + (candidateFactor-1)*k == candidateFactor*k entries, i.e. memory stays
// O(K) regardless of stream cardinality. Candidates let a late heavy-hitter
// arriving incrementally via Touch accumulate and eventually displace an
// incumbent; once the cap is reached the smallest-count candidate is dropped.
const candidateFactor = 4

// Tracker maintains the top-K items by frequency.
//
// Concurrency: safe for concurrent use. Every method (Touch, TouchN, Top, Count,
// Len, K, Reset) acquires an internal sync.Mutex, so concurrent callers are
// serialised — correct, but a single Tracker contends under high write rates.
// For more throughput, shard by key and merge the per-shard Top results.
//
// Memory bound: O(K). The count map holds the K in-heap keys plus a bounded set
// of at most (candidateFactor-1)*K candidate keys that are accumulating but not
// yet in the top-K set. A key that is neither in the heap nor a tracked
// candidate reports Count 0.
type Tracker struct {
	mu      sync.Mutex
	k       int
	cap     int // max entries in counts (== candidateFactor*k)
	counts  map[string]int64
	minHeap *itemHeap
}

type item struct {
	key   string
	count int64
	index int // heap index
}

type itemHeap []*item

func (h itemHeap) Len() int           { return len(h) }
func (h itemHeap) Less(i, j int) bool { return h[i].count < h[j].count }
func (h itemHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *itemHeap) Push(x any) {
	it := x.(*item)
	it.index = len(*h)
	*h = append(*h, it)
}

func (h *itemHeap) Pop() any {
	old := *h
	n := len(old)
	it := old[n-1]
	old[n-1] = nil
	it.index = -1
	*h = old[:n-1]
	return it
}

// New builds a tracker for the top k items. Panics if k <= 0.
func New(k int) *Tracker {
	if k <= 0 {
		panic("topk: k must be > 0")
	}
	return &Tracker{
		k:       k,
		cap:     candidateFactor * k,
		counts:  make(map[string]int64),
		minHeap: &itemHeap{},
	}
}

// Touch increments the count for key by 1 and updates the top-K set.
func (t *Tracker) Touch(key string) {
	t.TouchN(key, 1)
}

// TouchN increments the count for key by n and updates the top-K set.
//
// Memory stays bounded at O(K): the count map holds the K in-heap keys plus at
// most (candidateFactor-1)*K candidate keys that are accumulating but have not
// yet beaten the heap minimum. A non-admitted key retains its accumulated count
// across TouchN calls so a late heavy-hitter arriving incrementally (one event
// at a time) can build up and eventually displace an incumbent. Once the
// candidate cap is reached the smallest-count candidate is dropped, so a key
// that is neither in the heap nor among the tracked candidates reports Count 0.
//
// n <= 0 is a no-op.
func (t *Tracker) TouchN(key string, n int64) {
	if n <= 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	// Key already in the top-K set: bump its count and re-sift the heap.
	for _, it := range *t.minHeap {
		if it.key == key {
			t.counts[key] += n
			it.count = t.counts[key]
			heap.Fix(t.minHeap, it.index)
			return
		}
	}

	// Key not in the set. Accumulate its candidate count (retained across calls
	// so incremental Touch on a late heavy-hitter can build up).
	newCount := t.counts[key] + n

	if t.minHeap.Len() < t.k {
		// Room available: admit the key unconditionally.
		t.counts[key] = newCount
		heap.Push(t.minHeap, &item{key: key, count: newCount})
		return
	}

	// Set is full: admit only if this key beats the current minimum.
	minItem := (*t.minHeap)[0]
	if newCount > minItem.count {
		// Promote the candidate; demote the heap minimum into the candidate set.
		heap.Pop(t.minHeap)
		// minItem's count stays in counts — it is now a candidate again and can
		// re-accumulate if it is touched later.
		t.counts[key] = newCount
		heap.Push(t.minHeap, &item{key: key, count: newCount})
	} else {
		// Not competitive yet — record the accumulated count as a candidate so a
		// future Touch can build on it, then enforce the memory cap.
		t.counts[key] = newCount
	}
	t.evictCandidates()
}

// evictCandidates drops the smallest-count non-heap entry (or entries) until the
// counts map is back within cap. Called on every touch that adds/updates a
// candidate. The heap keys are never evicted. Cost is O(len(counts)) and only
// incurred when the cap is exceeded.
func (t *Tracker) evictCandidates() {
	for len(t.counts) > t.cap {
		// Build the set of in-heap keys so we never evict a leaderboard member.
		inHeap := make(map[string]struct{}, t.minHeap.Len())
		for _, it := range *t.minHeap {
			inHeap[it.key] = struct{}{}
		}
		// Find the smallest-count candidate (non-heap key). Ties broken by key
		// for determinism.
		var dropKey string
		var dropCount int64
		first := true
		for k, c := range t.counts {
			if _, ok := inHeap[k]; ok {
				continue
			}
			if first || c < dropCount || (c == dropCount && k < dropKey) {
				first = false
				dropKey = k
				dropCount = c
			}
		}
		if first {
			return // nothing to drop (only in-heap keys present)
		}
		delete(t.counts, dropKey)
	}
}

// Entry is one ranked item returned by Top.
type Entry struct {
	Key   string
	Count int64
}

// Top returns the current top-K items sorted by count descending.
func (t *Tracker) Top() []Entry {
	t.mu.Lock()
	defer t.mu.Unlock()
	entries := make([]Entry, 0, t.minHeap.Len())
	for _, it := range *t.minHeap {
		entries = append(entries, Entry{Key: it.key, Count: it.count})
	}
	slices.SortFunc(entries, func(a, b Entry) int { return cmp.Compare(b.Count, a.Count) })
	return entries
}

// Count returns the current count for key, or 0 if the key is unseen, has been
// evicted from the candidate set, or has fallen out of the top-K set without
// being tracked as a candidate.
func (t *Tracker) Count(key string) int64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.counts[key]
}

// Len returns the number of items currently in the top-K set.
func (t *Tracker) Len() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.minHeap.Len()
}

// Reset clears all counts and the top-K set.
func (t *Tracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.counts = make(map[string]int64)
	t.minHeap = &itemHeap{}
}

// K returns the configured K.
func (t *Tracker) K() int { return t.k }
