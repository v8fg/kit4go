package debounce

import (
	"sync"
	"time"
)

// fakeClock is a deterministic clock seam for throttle window tests (E5).
// It replaces time.Sleep-based window-assertions with explicit time
// advancement, removing wall-clock flakiness under CPU contention.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.UnixMilli(0)}
}

func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

// advance moves the clock forward by d; callers then synchronously observe the
// throttle's window decision via the now seam (no real Sleep needed).
func (f *fakeClock) advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}
