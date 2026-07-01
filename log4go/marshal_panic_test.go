package log4go

import (
	"sync/atomic"
	"testing"
)

// L5 observability: a recovered field-marshal / error-string panic must be
// counted and exposed via RuntimeStats, not silently swallowed to null.

type l5PanicMarshaler struct{}

func (l5PanicMarshaler) MarshalJSON() ([]byte, error) { panic("marshalboom") }

type l5PanicErr struct{}

func (l5PanicErr) Error() string { panic("errboom") }

// snapshotMarshalPanics reads the package counter and returns the snapshot plus a
// restore func, so each test asserts a delta and is isolated from other tests in
// the package that also exercise the recover path. Same-package tests run
// sequentially, so the snapshot/restore pair is race-free.
func snapshotMarshalPanics(t *testing.T) (before uint64, restore func()) {
	t.Helper()
	b := atomic.LoadUint64(&marshalPanics)
	return b, func() { atomic.StoreUint64(&marshalPanics, b) }
}

func TestSafeJSONMarshal_PanicRecoveredAndCounted(t *testing.T) {
	before, restore := snapshotMarshalPanics(t)
	defer restore()

	b, ok := safeJSONMarshal(l5PanicMarshaler{})
	if ok {
		t.Fatal("expected ok=false after a marshalling panic")
	}
	if len(b) != 0 {
		t.Fatalf("expected no bytes after panic, got %q", b)
	}
	if got := atomic.LoadUint64(&marshalPanics); got != before+1 {
		t.Fatalf("marshalPanics: want %d, got %d", before+1, got)
	}
}

func TestSafeErrorString_PanicRecoveredAndCounted(t *testing.T) {
	before, restore := snapshotMarshalPanics(t)
	defer restore()

	s, ok := safeErrorString(l5PanicErr{})
	if ok {
		t.Fatal("expected ok=false after an error-string panic")
	}
	if s != "" {
		t.Fatalf("expected empty string after panic, got %q", s)
	}
	if got := atomic.LoadUint64(&marshalPanics); got != before+1 {
		t.Fatalf("marshalPanics: want %d, got %d", before+1, got)
	}
}

func TestRuntimeStats_ExposesMarshalPanics(t *testing.T) {
	before, restore := snapshotMarshalPanics(t)
	defer restore()

	_, _ = safeJSONMarshal(l5PanicMarshaler{})
	_, _ = safeErrorString(l5PanicErr{})

	m := RuntimeStats()
	if m.MarshalPanics != before+2 {
		t.Fatalf("RuntimeStats().MarshalPanics: want %d, got %d", before+2, m.MarshalPanics)
	}
}

// Non-panicking marshal must NOT touch the counter (no false positives).
func TestSafeJSONMarshal_NoPanicNoCount(t *testing.T) {
	before, restore := snapshotMarshalPanics(t)
	defer restore()

	if _, ok := safeJSONMarshal(42); !ok {
		t.Fatal("expected ok=true for a plain int")
	}
	if got := atomic.LoadUint64(&marshalPanics); got != before {
		t.Fatalf("marshalPanics should be unchanged (%d) for a clean marshal, got %d", before, got)
	}
}
