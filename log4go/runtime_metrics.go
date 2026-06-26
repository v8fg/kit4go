package log4go

import (
	"runtime"
)

// RuntimeMetrics is a point-in-time snapshot of the Go runtime + log4go process
// metrics, suitable for exposing to a monitoring system (Prometheus / statsd /
// OpenTelemetry). Gather it via RuntimeStats() on a monitoring cadence.
type RuntimeMetrics struct {
	// GOMAXPROCS is the effective parallelism log4go sees (honors the cgroup CPU
	// quota on Go 1.25+; on older Go it may be the host count — see maxprocs).
	GOMAXPROCS int
	// NumGoroutine is the live goroutine count. For log4go it is constant under
	// load (bootstrap + writer daemons, no per-record goroutine) — a rising count
	// signals a leak elsewhere.
	NumGoroutine int
	// HeapAlloc is bytes allocated and still in use.
	HeapAlloc uint64
	// HeapInuse is bytes in in-use spans (a rough live-set size).
	HeapInuse uint64
	// HeapSys is bytes obtained from the OS for the heap.
	HeapSys uint64
	// HeapObjects is the number of live heap objects.
	HeapObjects uint64
	// NumGC is the number of completed GC cycles.
	NumGC uint32
	// StackInuse is bytes in in-use stack spans.
	StackInuse uint64
	// LastGCCPU is the fraction of CPU time spent in GC since the program start
	// (MemStats.GCCPUFraction), a single-number GC-pressure signal.
	GCCPUFraction float64
}

// RuntimeStats returns a snapshot of runtime memory + goroutine metrics for
// monitoring export.
//
// Performance note: it calls runtime.ReadMemStats, which briefly stops the world
// (sub-millisecond). Call it at monitoring cadence (e.g. every 10–30 s from a
// Prometheus collector) — NEVER on the per-record log hot path. log4go never
// calls this internally.
func RuntimeStats() RuntimeMetrics {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return RuntimeMetrics{
		GOMAXPROCS:    runtime.GOMAXPROCS(0),
		NumGoroutine:  runtime.NumGoroutine(),
		HeapAlloc:     ms.HeapAlloc,
		HeapInuse:     ms.HeapInuse,
		HeapSys:       ms.HeapSys,
		HeapObjects:   ms.HeapObjects,
		NumGC:         ms.NumGC,
		StackInuse:    ms.StackInuse,
		GCCPUFraction: ms.GCCPUFraction,
	}
}
