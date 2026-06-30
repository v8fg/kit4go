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
func (t *Tracker) TouchN(key string, n int64) {
	if n <= 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.counts[key] += n

	// Check if key is already in the heap.
	found := false
	for _, it := range *t.minHeap {
		if it.key == key {
			it.count = t.counts[key]
			heap.Fix(t.minHeap, it.index)
			found = true
			break
		}
	}
	if found {
		return
	}

	// Key not in heap. Add if there's room, or replace the min if we exceed k.
	if t.minHeap.Len() < t.k {
		heap.Push(t.minHeap, &item{key: key, count: t.counts[key]})
	} else {
		// Replace the min element if this key's count exceeds it.
		minItem := (*t.minHeap)[0]
		if t.counts[key] > minItem.count {
			heap.Pop(t.minHeap)
			heap.Push(t.minHeap, &item{key: key, count: t.counts[key]})
		}
	}
}

// Top returns the current top-K items sorted by count descending.
type Entry struct {
	Key   string
	Count int64
}

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

// Count returns the current count for key (0 if unseen).
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
