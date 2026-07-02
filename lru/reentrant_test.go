package lru

import (
	"testing"
	"time"
)

// Regression: a re-entrant call from onEvicted (even a read) must not deadlock.
// Before the fix, onEvicted ran under the write lock, so any callback that
// touched the cache hung it permanently.
func TestCache_ReentrantOnEvicted_NoDeadlock(t *testing.T) {
	var c *Cache[int, int]
	c = New[int, int](
		WithMaxSize[int, int](1),
		WithOnEvicted[int, int](func(k, v int) {
			_, _ = c.Get(k) // re-entrant read
		}),
	)
	c.Set(1, 1) // fills the size-1 cache

	done := make(chan struct{})
	go func() {
		c.Set(2, 2) // evicts 1 -> onEvicted(1) -> re-entrant Get
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("re-entrant onEvicted deadlocked the cache")
	}
}
