//go:build integration

package kafka

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

// TestIntegration_AcksSweep measures the acks durability-vs-throughput tradeoff
// under replication: sweeps acks (leader vs all) on a 3-broker RF=3 cluster
// (topic stress-acks, 12 partitions). acks only matters with RF>1 — on RF=1
// leader==all (no replication). Reduced volume (100K/50K) + the cluster runs with
// 512M broker heaps to stay stable on one host. Writes /tmp/stress_acks_<backend>.md.
//
// PRE-REQ: 3-broker cluster + topic `stress-acks` (RF=3, 12 partitions).
func TestIntegration_AcksSweep(t *testing.T) {
	brokers := os.Getenv("KAFKA_BROKERS")
	if brokers == "" {
		t.Skip("KAFKA_BROKERS unset; skipping acks sweep")
	}
	const topic = "stress-acks"
	const batch = 1024
	backend := backendName
	path := "/tmp/stress_acks_" + backend + ".md"
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer f.Close()
	w := newFlushWriter(f)
	fmt.Fprintf(w, "# acks sweep — backend=%s (3-broker RF=3, 12 partitions, loopback, broker heap 512M)\n\n", backend)
	fmt.Fprintf(w, "topic=%s, batch=%d, linger 10ms. Reduced volume (100K/50K) for cluster stability.\n\n", topic, batch)
	fmt.Fprintf(w, "| mode | acks | payload | volume | rec/s | MB/s | wall | err |\n|---|---|---|---|---|---|---|---|\n")
	w.flush()

	ctx := context.Background()
	hsize := func(b int) string {
		switch {
		case b >= 10240:
			return "10KB"
		case b >= 1024:
			return "1KB"
		}
		return fmt.Sprintf("%dB", b)
	}

	// async: acks {leader, all} × payload {100B, 1KB, 10KB}
	for _, acks := range []string{AcksLeader, AcksAll} {
		for _, size := range []int{100, 1024, 10240} {
			n := 100_000
			if size >= 10240 {
				n = 50_000
			}
			prod, err := NewProducer(WithBrokers(brokers), WithTopic(topic), WithAcks(acks))
			if err != nil {
				row := fmt.Sprintf("| async | %s | %s | %s | - | - | - | NewProducer: %v |", acks, hsize(size), humanVol(n), err)
				fmt.Fprintln(w, row)
				w.flush()
				t.Log(row)
				continue
			}
			payload := make([]byte, size)
			msgs := make([]Message, 0, batch)
			time.Sleep(200 * time.Millisecond)
			start := time.Now()
			var cellErr error
			for i := 0; i < n; i += batch {
				b := batch
				if i+b > n {
					b = n - i
				}
				msgs = msgs[:0]
				for j := 0; j < b; j++ {
					msgs = append(msgs, Message{Topic: topic, Value: payload}) // null key → distributed
				}
				if e := prod.SendBatch(ctx, msgs); e != nil {
					cellErr = fmt.Errorf("SendBatch@%d: %w", i, e)
					break
				}
			}
			if ce := prod.Close(); ce != nil && cellErr == nil {
				cellErr = fmt.Errorf("Close: %w", ce)
			}
			dur := time.Since(start)
			row := fmt.Sprintf("| async | %s | %s | %s | %.0f | %.1f | %v | %s |",
				acks, hsize(size), humanVol(n), float64(n)/dur.Seconds(), float64(n*size)/dur.Seconds()/1024/1024,
				dur.Round(time.Millisecond), errOrEmpty(cellErr))
			fmt.Fprintln(w, row)
			w.flush()
			t.Log(row)
		}
	}

	// sync SendBatch(1024) × 100B: acks {leader, all}
	for _, acks := range []string{AcksLeader, AcksAll} {
		sp, err := NewSyncProducer(WithBrokers(brokers), WithTopic(topic), WithAcks(acks))
		if err != nil {
			row := fmt.Sprintf("| sync | %s | 100B | 100K | - | - | - | NewSyncProducer: %v |", acks, err)
			fmt.Fprintln(w, row)
			w.flush()
			t.Log(row)
			continue
		}
		const n = 100_000
		payload := make([]byte, 100)
		msgs := make([]Message, 0, batch)
		time.Sleep(200 * time.Millisecond)
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
		_ = sp.Close()
		dur := time.Since(start)
		row := fmt.Sprintf("| sync | %s | 100B | %s | %.0f | %.1f | %v | %s |",
			acks, humanVol(n), float64(n)/dur.Seconds(), float64(n*100)/dur.Seconds()/1024/1024,
			dur.Round(time.Millisecond), errOrEmpty(cellErr))
		fmt.Fprintln(w, row)
		w.flush()
		t.Log(row)
	}
	fmt.Fprintf(w, "\n_Done %s (backend=%s, acks sweep)_\n", time.Now().Format(time.RFC3339), backend)
	w.flush()
}
