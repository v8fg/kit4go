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

// TestIntegration_MultiNodeStress measures real multi-node QPS against a 3-broker
// KRaft cluster (RF=3, 12 partitions — leaders spread across all brokers). Records
// have null key → round-robin/byte-partitioned across the 12 partitions → all 3
// brokers. Compares to the single-node matrix in STRESS_MATRIX.md.
//
// PRE-REQUISITE: a 3-broker cluster on KAFKA_BROKERS, and the topic pre-created:
//
//	kafka-topics.sh --bootstrap-server localhost:9092 --create \
//	  --topic stress-mn --partitions 12 --replication-factor 3
//
// (the kafka package has no admin client; create it out-of-band.) Loopback → no real
// network RTT, but DOES exercise RF=3 replication overhead + cross-broker distribution.
func TestIntegration_MultiNodeStress(t *testing.T) {
	brokers := os.Getenv("KAFKA_BROKERS")
	if brokers == "" {
		t.Skip("KAFKA_BROKERS unset; skipping multi-node stress")
	}
	const topic = "stress-mn"
	const batch = 1024
	backend := backendName
	path := "/tmp/stress_mn_" + backend + ".md"
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer f.Close()
	w := newFlushWriter(f)
	fmt.Fprintf(w, "# multi-node stress — backend=%s (3-broker KRaft, RF=3, 12 partitions, loopback)\n\n", backend)
	fmt.Fprintf(w, "topic=%s, batch=%d, linger 10ms, null key (round-robin across 12 partitions → 3 brokers).\n\n", topic, batch)
	fmt.Fprintf(w, "| mode | payload | volume | rec/s | MB/s | bufMB | heapMB | wall | err |\n|---|---|---|---|---|---|---|---|---|\n")
	w.flush()

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
	volFor := func(size int) int {
		if size >= 10240 {
			return 200_000
		}
		return 1_000_000
	}

	// async: 100B, 1KB, 10KB
	for _, size := range []int{100, 1024, 10240} {
		n := volFor(size)
		prod, err := NewProducer(WithBrokers(brokers), WithTopic(topic))
		if err != nil {
			row := fmt.Sprintf("| async | %s | %s | - | - | - | - | - | NewProducer: %v |", hsize(size), humanVol(n), err)
			fmt.Fprintln(w, row)
			w.flush()
			t.Log(row)
			continue
		}
		payload := make([]byte, size)
		msgs := make([]Message, 0, batch)
		time.Sleep(200 * time.Millisecond)
		runtime.GC()
		var m0 runtime.MemStats
		runtime.ReadMemStats(&m0)
		start := time.Now()
		var cellErr error
		for i := 0; i < n; i += batch {
			b := batch
			if i+b > n {
				b = n - i
			}
			msgs = msgs[:0]
			for j := 0; j < b; j++ {
				msgs = append(msgs, Message{Topic: topic, Value: payload}) // null Key → distributed
			}
			if e := prod.SendBatch(ctx, msgs); e != nil {
				cellErr = fmt.Errorf("SendBatch@%d: %w", i, e)
				break
			}
		}
		pm := prod.Metrics()
		var m1 runtime.MemStats
		runtime.ReadMemStats(&m1)
		if ce := prod.Close(); ce != nil && cellErr == nil {
			cellErr = fmt.Errorf("Close: %w", ce)
		}
		dur := time.Since(start)
		row := fmt.Sprintf("| async | %s | %s | %.0f | %.1f | %.1f | %.1f | %v | %s |",
			hsize(size), humanVol(n), float64(n)/dur.Seconds(), float64(n*size)/dur.Seconds()/1024/1024,
			float64(pm.BufferedBytes)/1024/1024, heapDeltaMB(m0, m1), dur.Round(time.Millisecond), errOrEmpty(cellErr))
		fmt.Fprintln(w, row)
		w.flush()
		t.Log(row)
	}

	// sync SendBatch(1024) × 100B (acks path: sarama=leader, franz-go=all-ISR → replication)
	sp, err := NewSyncProducer(WithBrokers(brokers), WithTopic(topic))
	if err == nil {
		payload := make([]byte, 100)
		const n = 1_000_000
		msgs := make([]Message, 0, batch)
		time.Sleep(200 * time.Millisecond)
		runtime.GC()
		var m0 runtime.MemStats
		runtime.ReadMemStats(&m0)
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
			if e := sp.SendBatch(ctx, msgs); e != nil {
				cellErr = fmt.Errorf("SendBatch@%d: %w", i, e)
				break
			}
		}
		var m1 runtime.MemStats
		runtime.ReadMemStats(&m1)
		_ = sp.Close()
		dur := time.Since(start)
		row := fmt.Sprintf("| syncSendBatch | 100B | %s | %.0f | %.1f | 0.0 | %.1f | %v | %s |",
			humanVol(n), float64(n)/dur.Seconds(), float64(n*100)/dur.Seconds()/1024/1024,
			heapDeltaMB(m0, m1), dur.Round(time.Millisecond), errOrEmpty(cellErr))
		fmt.Fprintln(w, row)
		w.flush()
		t.Log(row)
	}
	fmt.Fprintf(w, "\n_Done %s (backend=%s, 3-node)_\n", time.Now().Format(time.RFC3339), backend)
	w.flush()
}
