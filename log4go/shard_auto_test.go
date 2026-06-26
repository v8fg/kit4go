package log4go

import (
	"runtime"
	"testing"
)

// Test_AutoShardCount verifies the clamp(GOMAXPROCS/2, 2, 8) policy across the
// realistic GOMAXPROCS range (1..64), covering small VMs, typical cloud boxes,
// and large hosts/over-provisioned containers.
func Test_AutoShardCount(t *testing.T) {
	cases := []struct {
		procs int
		want  int
	}{
		{1, 2},   // tiny VM / single-core container -> floor 2
		{2, 2},   // 2-core -> 2
		{4, 2},   // 4-core (common cloud) -> 2
		{6, 3},   // -> 3
		{8, 4},   // 8-core -> 4
		{10, 5},  // 10-core (this M5) -> 5
		{16, 8},  // 16-core -> 8 (no cap; scales with cores)
		{32, 16}, // 32-core -> 16 (scales, not capped at 8)
		{64, 32}, // 64-core host -> 32 (big machine gets more consumers)
		{128, 64}, // 128-core -> 64
	}
	prev := runtime.GOMAXPROCS(0)
	defer runtime.GOMAXPROCS(prev)
	for _, c := range cases {
		runtime.GOMAXPROCS(c.procs)
		if got := AutoShardCount(); got != c.want {
			t.Errorf("GOMAXPROCS=%d AutoShardCount=%d want %d", c.procs, got, c.want)
		}
	}
}

// Test_NewShardLogger_Auto verifies n<=0 routes to AutoShardCount.
func Test_NewShardLogger_Auto(t *testing.T) {
	prev := runtime.GOMAXPROCS(0)
	defer runtime.GOMAXPROCS(prev)
	runtime.GOMAXPROCS(8) // -> AutoShardCount 4

	a := NewShardLogger(0)
	if len(a.loggers) != 4 {
		t.Errorf("NewShardLogger(0) shards=%d want 4 (auto)", len(a.loggers))
	}
	a.Close()

	b := NewShardLoggerAuto()
	if len(b.loggers) != 4 {
		t.Errorf("NewShardLoggerAuto shards=%d want 4", len(b.loggers))
	}
	b.Close()

	// explicit pin still honored
	c := NewShardLogger(3)
	if len(c.loggers) != 3 {
		t.Errorf("NewShardLogger(3) shards=%d want 3", len(c.loggers))
	}
	c.Close()
}
