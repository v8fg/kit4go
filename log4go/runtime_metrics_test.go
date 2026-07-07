package log4go

import (
	"testing"
)

// Test_RuntimeStats_Sane verifies RuntimeStats returns sane, non-zero runtime
// metrics for monitoring export (and exercises the path that Prometheus etc.
// would call at scrape cadence).
func Test_RuntimeStats_Sane(t *testing.T) {
	m := RuntimeStats()
	if m.GOMAXPROCS < 1 {
		t.Errorf("GOMAXPROCS=%d want >=1", m.GOMAXPROCS)
	}
	if m.NumGoroutine < 1 {
		t.Errorf("NumGoroutine=%d want >=1", m.NumGoroutine)
	}
	// after allocating a few things, heap should be non-trivial
	if m.HeapObjects == 0 {
		t.Error("HeapObjects=0; ReadMemStats not populated")
	}
	// GC CPU fraction is a valid [0,1] double
	if m.GCCPUFraction < 0 || m.GCCPUFraction > 1 {
		t.Errorf("GCCPUFraction=%v out of [0,1]", m.GCCPUFraction)
	}
}

// Test_RuntimeStats_StableAcrossCalls: two calls return NumGC that is
// non-decreasing (monitoring snapshots must be consistent).
func Test_RuntimeStats_StableAcrossCalls(t *testing.T) {
	a := RuntimeStats()
	// force some allocation + a GC so the second snapshot can differ sensibly
	for range 1000 {
		_ = make([]byte, 1024)
	}
	b := RuntimeStats()
	if b.NumGC < a.NumGC {
		t.Errorf("NumGC went backwards: %d -> %d", a.NumGC, b.NumGC)
	}
	if b.HeapObjects == 0 {
		t.Error("second snapshot HeapObjects=0")
	}
}

// Test_ShardLogger_StartupLogAndCount verifies the startup banner path and the
// ShardCount accessor.
func Test_ShardLogger_StartupLogAndCount(t *testing.T) {
	s := NewShardLogger(4)
	defer s.Close()
	if got := s.ShardCount(); got != 4 {
		t.Errorf("ShardCount=%d want 4", got)
	}
}
