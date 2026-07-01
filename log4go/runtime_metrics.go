package log4go

import (
	"runtime"
	"sync/atomic"
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
	// MarshalPanics counts field-marshal / error-string panics recovered on the
	// render path (a buggy MarshalJSON, a typed-nil receiver). Non-zero means a
	// logged value is silently becoming null instead of crashing the pipeline —
	// investigate the caller's value. See field.safeJSONMarshal.
	MarshalPanics uint64
	// DaemonPanics counts panics recovered inside a writer daemon goroutine
	// (file/kafka/net/webhook). Non-zero means a writer has DIED — its records
	// stop flowing. Restart the process (or the writer) to recover. See
	// daemon_panic.go.
	DaemonPanics uint64
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
		MarshalPanics: atomic.LoadUint64(&marshalPanics),
		DaemonPanics:  atomic.LoadUint64(&daemonPanics),
	}
}
