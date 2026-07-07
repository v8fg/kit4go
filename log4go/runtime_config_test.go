package log4go

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/goleak"
)

// Compile-time check: *Logger satisfies the RuntimeConfig contract (the hot,
// lock-free configuration surface).
var _ RuntimeConfig = (*Logger)(nil)

// TestRuntimeConfig_ConcurrentHotUpdateNoRace proves the hot-update surface is
// safe under extreme concurrency (the ad-tech millions-of-records/sec case):
// several goroutines rapidly mutate level/caller/func/base fields via RuntimeConfig
// while others log furiously, with no data race and no stall — every setting is
// read via atomic loads on the delivery path.
func TestRuntimeConfig_ConcurrentHotUpdateNoRace(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	defer Close()
	Close()

	if err := SetupLog(LogConfig{Level: "info", ConsoleWriter: ConsoleWriterOptions{Enable: true}}); err != nil {
		t.Fatal(err)
	}
	rc := Runtime()

	var stop atomic.Bool
	var wg sync.WaitGroup

	// Hot mutators: rapidly change runtime config from several goroutines.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for !stop.Load() {
				rc.SetLevel(DEBUG + (i % 5))
				rc.WithCaller(i%2 == 0)
				rc.WithFuncName(i%2 == 1)
				rc.SetBaseField("probe", i)
			}
		}(i)
	}

	// Loggers: hammer the delivery path while config mutates underneath.
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 5000; j++ {
				Info("concurrent hot-update %d/%d", i, j)
			}
		}(i)
	}

	time.Sleep(100 * time.Millisecond) // let mutators + loggers overlap
	stop.Store(true)
	wg.Wait()
}

// TestRuntimeConfig_SetSampling covers SetSampling's enable and disable paths
// (RuntimeConfig surface) applied in place on a logger.
func TestRuntimeConfig_SetSampling(t *testing.T) {
	root := newLoggerWithRecords(make(chan *Record, 4))
	defer root.Close()

	root.SetSampling(10, 5)
	s := root.sampler.Load()
	if s == nil || s.Initial != 10 || s.Thereafter != 5 {
		t.Fatalf("SetSampling(10,5) not applied: %+v", s)
	}
	root.SetSampling(0, 0) // disable
	if s := root.sampler.Load(); s != nil {
		t.Fatalf("SetSampling(0,0) should disable, got %+v", s)
	}
	// clamp branches: negative initial -> 0; thereafter <= 0 (with initial>0) -> 1
	root.SetSampling(-1, 5)
	if s := root.sampler.Load(); s.Initial != 0 || s.Thereafter != 5 {
		t.Errorf("SetSampling(-1,5) clamp: Initial=%d Thereafter=%d want 0,5", s.Initial, s.Thereafter)
	}
	root.SetSampling(3, 0)
	if s := root.sampler.Load(); s.Initial != 3 || s.Thereafter != 1 {
		t.Errorf("SetSampling(3,0) clamp: Initial=%d Thereafter=%d want 3,1", s.Initial, s.Thereafter)
	}
}

// TestRuntimeConfig_SetContextExtractor covers SetContextExtractor's set/clear
// paths (RuntimeConfig) — the runtime trace/context-capture toggle.
func TestRuntimeConfig_SetContextExtractor(t *testing.T) {
	root := newLoggerWithRecords(make(chan *Record, 4))
	defer root.Close()

	fn := func(context.Context) map[string]any { return map[string]any{"trace_id": "x"} }
	root.SetContextExtractor(fn)
	if root.ctxExtractor.Load() == nil {
		t.Fatal("extractor not installed")
	}
	root.SetContextExtractor(nil) // clear -> falls back to global stack
	if root.ctxExtractor.Load() != nil {
		t.Fatal("extractor not cleared")
	}
}

// TestClone_PreservesSamplerAndExtractor verifies clone() copies the current
// sampler and context extractor into the child (the non-nil atomic store arms).
func TestClone_PreservesSamplerAndExtractor(t *testing.T) {
	root := newLoggerWithRecords(make(chan *Record, 4))
	defer root.Close()
	root.SetSampling(7, 3)
	root.SetContextExtractor(func(context.Context) map[string]any { return nil })

	child := root.With("k", "v") // triggers clone
	if child.sampler.Load() == nil {
		t.Error("child lost sampler on clone")
	}
	if child.ctxExtractor.Load() == nil {
		t.Error("child lost context extractor on clone")
	}
	// child mutates its own copy without affecting the parent
	child.SetSampling(0, 0)
	if root.sampler.Load() == nil {
		t.Error("child SetSampling leaked to parent")
	}
}
