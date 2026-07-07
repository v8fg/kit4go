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
	"container/heap"
	"sort"
	"sync"
)

// Tracker maintains the top-K items by frequency.
//
// Concurrency: safe for concurrent use. Every method (Touch, TouchN, Top, Count,
// Len, K, Reset) acquires an internal sync.Mutex, so concurrent callers are
// serialised — correct, but a single Tracker contends under high write rates.
// For more throughput, shard by key and merge the per-shard Top results.
type Tracker struct {
	mu      sync.Mutex
	k       int
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
// To keep memory bounded at O(K), only keys currently in the top-K set are
// retained in the count map. A key that has never entered the set, or that has
// been evicted, is not tracked — its accumulated count is discarded and Count()
// reports 0 for it. This trades exact per-key counts for non-top-K keys (which
// a leaderboard cannot surface anyway) for a hard O(K) memory bound.
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

	// Key not in the set. Compute the count it would have if admitted, counting
	// only what we still track (0 once a key has been evicted).
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
		heap.Pop(t.minHeap)
		// Drop the evicted key's count so counts stays bounded at ~K.
		delete(t.counts, minItem.key)
		t.counts[key] = newCount
		heap.Push(t.minHeap, &item{key: key, count: newCount})
	}
	// else: key not competitive — do not store it, keeping counts bounded.
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
	sort.Slice(entries, func(i, j int) bool { return entries[i].Count > entries[j].Count })
	return entries
}

// Count returns the current count for key, or 0 if the key is unseen or has
// fallen out of the top-K set (evicted keys are no longer tracked, so their
// count is reported as 0 once they leave the leaderboard).
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
