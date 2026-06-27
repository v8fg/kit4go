package log4go

import (
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

// TestReload_ConcurrentWithLoggingNoRace proves a Reload (SIGHUP-driven config
// swap) racing with ongoing traffic is safe: the singleton is swapped atomically,
// the old logger is drained+stopped, and concurrent log calls never see a torn
// state. The ultra-high-concurrency guarantee for "setting updates mid-traffic".
func TestReload_ConcurrentWithLoggingNoRace(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	defer Close()
	Close()

	if err := SetupLog(LogConfig{Level: "info", ConsoleWriter: ConsoleWriterOptions{Enable: true}}); err != nil {
		t.Fatal(err)
	}

	var stop atomic.Bool
	var wg sync.WaitGroup

	// Reloader: repeatedly swap the singleton while traffic flows.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for !stop.Load() {
			_ = Reload(LogConfig{Level: "info", ConsoleWriter: ConsoleWriterOptions{Enable: true}})
		}
	}()

	// Loggers: keep delivering while Reloads race underneath.
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 5000; j++ {
				Info("reload-concurrent %d/%d", i, j)
			}
		}(i)
	}

	time.Sleep(100 * time.Millisecond)
	stop.Store(true)
	wg.Wait()
}
