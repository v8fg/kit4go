//go:build integration

package kafka

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"testing"
	"time"
)

// TestIntegration_StressMatrix measures, per backend (sarama default, or franz-go
// via -tags franzgo):
//
//   - sync: per-record Send + SendBatch — max QPS + memory (100B; size-independent)
//   - async: payload {100B,1KB,10KB,100KB} × batchsize {128,256,512,1024,2048,4096,10000}
//     — max QPS + memory
//
// Memory = two metrics: bufMB (producer.BufferedBytes, exact in-flight buffer at peak)
// and heapMB (Go HeapAlloc delta vs a GC'd baseline). msgs slice is reused ([%0]) to
// cut alloc noise. A 200ms settle after NewProducer removes franz-go cold-start noise.
// Results stream to /tmp/stress_<backend>.md. Gated by KAFKA_BROKERS (RF=1 for 1 node).
func TestIntegration_StressMatrix(t *testing.T) {
	brokers := os.Getenv("KAFKA_BROKERS")
	if brokers == "" {
		t.Skip("KAFKA_BROKERS unset; skipping stress matrix")
	}
	backend := backendName
	path := "/tmp/stress_" + backend + ".md"
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer f.Close()
	w := newFlushWriter(f)
	fmt.Fprintf(w, "# kafka stress — backend=%s (apache/kafka 3.8.0 KRaft, single node)\n\n", backend)
	fmt.Fprintf(w, "memory: bufMB=producer.BufferedBytes (exact in-flight), heapMB=HeapAlloc Δ vs GC'd baseline. msgs reused. 200ms settle.\n\n")
	w.flush()

	nano := time.Now().UnixNano()
	ctx := context.Background()
	hsize := func(b int) string {
		switch {
		case b >= 102400:
			return "100KB"
		case b >= 10240:
			return "10KB"
		case b >= 1024:
			return "1KB"
		}
		return fmt.Sprintf("%dB", b)
	}
	mb := func(b uint64) float64 { return float64(b) / 1024 / 1024 }

	// ---------- SYNC: per-record + SendBatch (100B) ----------
	fmt.Fprintf(w, "## sync (100B; size-independent)\n\n| method | batch | volume | rec/s | bufMB | heapMB | wall | err |\n|---|---|---|---|---|---|---|---|\n")
	w.flush()
	syncCells := []struct {
		batch int // 0 = per-record
		n     int
	}{{0, 100_000}, {128, 1_000_000}, {1024, 1_000_000}, {10000, 1_000_000}}
	for _, c := range syncCells {
		size := 100
		topic := fmt.Sprintf("stress-sync-%s-%d-%d", backend, c.batch, nano)
		sp, err := NewSyncProducer(WithBrokers(brokers), WithTopic(topic))
		if err != nil {
			r := fmt.Sprintf("| %s | %d | %s | - | - | - | - | NewSyncProducer: %v |", syncMethod(c.batch), c.batch, humanVol(c.n), err)
			fmt.Fprintln(w, r)
			w.flush()
			t.Log(r)
			continue
		}
		payload := make([]byte, size)
		time.Sleep(200 * time.Millisecond) // settle (franz-go lazy connect)
		runtime.GC()
		var m0 runtime.MemStats
		runtime.ReadMemStats(&m0)
		start := time.Now()
		var cellErr error
		if c.batch == 0 {
			for i := 0; i < c.n; i++ {
				if _, _, e := sp.Send(ctx, Message{Topic: topic, Value: payload}); e != nil {
					cellErr = fmt.Errorf("Send@%d: %w", i, e)
					break
				}
			}
		} else {
			msgs := make([]Message, 0, c.batch)
			for i := 0; i < c.n; i += c.batch {
				b := c.batch
				if i+b > c.n {
					b = c.n - i
				}
				msgs = msgs[:0]
				for j := 0; j < b; j++ {
					msgs = append(msgs, Message{Topic: topic, Value: payload})
				}
				if e := sp.SendBatch(ctx, msgs); e != nil {
					cellErr = fmt.Errorf("SendBatch@%d: %w", i, e)
					break
				}
			}
		}
		buf := sp.Metrics().BufferedBytes
		var m1 runtime.MemStats
		runtime.ReadMemStats(&m1)
		_ = sp.Close()
		dur := time.Since(start)
		r := fmt.Sprintf("| %s | %d | %s | %.0f | %.1f | %.1f | %v | %s |",
			syncMethod(c.batch), c.batch, humanVol(c.n), float64(c.n)/dur.Seconds(), mb(buf), heapDeltaMB(m0, m1),
			dur.Round(time.Millisecond), errOrEmpty(cellErr))
		fmt.Fprintln(w, r)
		w.flush()
		t.Log(r)
	}

	// ---------- ASYNC: payload × batchsize (QPS + memory) ----------
	payloads := []int{100, 1024, 10240, 102400}
	batches := []int{128, 256, 512, 1024, 2048, 4096, 10000}
	volFor := func(size int) int {
		switch {
		case size <= 1024:
			return 1_000_000
		case size <= 10240:
			return 200_000
		}
		return 20_000
	}
	fmt.Fprintf(w, "\n## async (payload × batchsize; QPS + memory)\n\n| payload | batch | volume | rec/s | MB/s | bufMB | heapMB | wall | err |\n|---|---|---|---|---|---|---|---|---|\n")
	w.flush()
	for _, size := range payloads {
		n := volFor(size)
		payload := make([]byte, size) // shared
		for _, batch := range batches {
			topic := fmt.Sprintf("stress-async-%s-%d-%d-%d", backend, size, batch, nano)
			prod, err := NewProducer(WithBrokers(brokers), WithTopic(topic))
			if err != nil {
				r := fmt.Sprintf("| %s | %d | %s | - | - | - | - | - | NewProducer: %v |", hsize(size), batch, humanVol(n), err)
				fmt.Fprintln(w, r)
				w.flush()
				t.Log(r)
				continue
			}
			time.Sleep(200 * time.Millisecond) // settle
			runtime.GC()
			var m0 runtime.MemStats
			runtime.ReadMemStats(&m0)
			msgs := make([]Message, 0, batch)
			start := time.Now()
			var cellErr error
			for i := 0; i < n; i += batch {
				b := batch
				if i+b > n {
					b = n - i
				}
				msgs = msgs[:0]
				for j := 0; j < b; j++ {
					msgs = append(msgs, Message{Topic: topic, Value: payload})
				}
				if e := prod.SendBatch(ctx, msgs); e != nil {
					cellErr = fmt.Errorf("SendBatch@%d: %w", i, e)
					break
				}
			}
			pm := prod.Metrics() // peak in-flight buffer (before Close drains)
			var m1 runtime.MemStats
			runtime.ReadMemStats(&m1)
			if ce := prod.Close(); ce != nil && cellErr == nil {
				cellErr = fmt.Errorf("Close: %w", ce)
			}
			dur := time.Since(start)
			r := fmt.Sprintf("| %s | %d | %s | %.0f | %.1f | %.1f | %.1f | %v | %s |",
				hsize(size), batch, humanVol(n), float64(n)/dur.Seconds(), float64(n*size)/dur.Seconds()/1024/1024,
				mb(pm.BufferedBytes), heapDeltaMB(m0, m1), dur.Round(time.Millisecond), errOrEmpty(cellErr))
			fmt.Fprintln(w, r)
			w.flush()
			t.Log(r)
		}
	}
	fmt.Fprintf(w, "\n_Done %s (backend=%s)_\n", time.Now().Format(time.RFC3339), backend)
	w.flush()
}

type flushWriter struct{ f *os.File }

func newFlushWriter(f *os.File) *flushWriter { return &flushWriter{f: f} }
func (w *flushWriter) Write(p []byte) (int, error) {
	n, err := w.f.Write(p)
	_ = w.f.Sync()
	return n, err
}
func (w *flushWriter) flush() { _ = w.f.Sync() }

func humanVol(v int) string {
	switch {
	case v >= 1_000_000:
		return fmt.Sprintf("%dM", v/1_000_000)
	case v >= 1_000:
		return fmt.Sprintf("%dK", v/1_000)
	}
	return fmt.Sprintf("%d", v)
}

func errOrEmpty(e error) string {
	if e == nil {
		return ""
	}
	return e.Error()
}

// heapDeltaMB returns the HeapAlloc delta clamped at 0 — if the heap shrank during
// a cell (GC reclaimed more than allocated) the raw uint64 subtract underflows to
// ~1.8e19 MB, so guard it. Shared by the single-node + multi-node stress tests.
func heapDeltaMB(m0, m1 runtime.MemStats) float64 {
	if m1.HeapAlloc <= m0.HeapAlloc {
		return 0
	}
	return float64(m1.HeapAlloc-m0.HeapAlloc) / 1024 / 1024
}

func syncMethod(batch int) string {
	if batch == 0 {
		return "per-record"
	}
	return "SendBatch"
}
