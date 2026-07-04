package log4go

import (
	"testing"
	"time"

	"github.com/v8fg/kit4go/kafka"
)

// newBatchWriter builds a KafKaWriter in batch mode backed by mp (no broker).
func newBatchWriter(t *testing.T, mp *mockKafkaProducer, batchSize int, flush time.Duration) *KafKaWriter {
	t.Helper()
	w := NewKafKaWriter(KafKaWriterOptions{
		ProducerTopic:      "t",
		BufferSize:         1024,
		BatchMode:          true,
		BatchSize:          batchSize,
		BatchFlushInterval: flush,
	})
	w.producerFactory = func() (kafka.Producer, error) { return mp, nil }
	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	return w
}

func writeN(t *testing.T, w *KafKaWriter, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if err := w.Write(&Record{level: INFO, msg: "x"}); err != nil {
			t.Fatal(err)
		}
	}
}

// waitSent polls until Metrics().Sent >= want or the deadline elapses.
func waitSent(t *testing.T, w *KafKaWriter, want uint64, why string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if w.Metrics().Sent >= want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("%s: Sent=%d want >= %d", why, w.Metrics().Sent, want)
}

// Batch mode end-to-end: 500 records / batchSize 100 → 5 count-triggered flushes
// via SendBatch (Send never called); all delivered; metrics tracked.
func Test_KafKaWriter_BatchMode_EndToEnd(t *testing.T) {
	mp := newMockKafkaProducer()
	w := newBatchWriter(t, mp, 100, time.Second) // flush interval high → only count fires
	const n = 500
	writeN(t, w, n)
	waitSent(t, w, n, "drain 500")
	w.Stop()

	m := w.Metrics()
	if m.Sent != n {
		t.Errorf("Sent=%d want %d", m.Sent, n)
	}
	if m.Batches < 5 { // 500/100 = 5 full-batch flushes
		t.Errorf("Batches=%d want >= 5", m.Batches)
	}
	if m.BatchMax == 0 || m.BatchMax > 100 {
		t.Errorf("BatchMax=%d want (0,100] (engaged, ≤ batchSize)", m.BatchMax)
	}
	if sends, batches := mp.callCounts(); sends != 0 || batches == 0 {
		t.Errorf("producer calls: Send=%d (want 0), SendBatch=%d (want >0)", sends, batches)
	}
	if got := len(mp.sent); got != n {
		t.Errorf("mock received=%d want %d", got, n)
	}
}

// Partial batch is flushed by the timer when BatchSize is not reached.
func Test_KafKaWriter_BatchMode_FlushOnTimer(t *testing.T) {
	mp := newMockKafkaProducer()
	w := newBatchWriter(t, mp, 10000, 20*time.Millisecond) // high size, short interval
	const n = 5
	writeN(t, w, n)
	waitSent(t, w, n, "timer flush")
	w.Stop()

	if got := len(mp.sent); got != n {
		t.Errorf("mock received=%d want %d (timer flush)", got, n)
	}
	if m := w.Metrics(); m.Batches == 0 {
		t.Error("Batches=0, expected timer-triggered flush")
	}
	if sends, _ := mp.callCounts(); sends != 0 {
		t.Errorf("Send calls=%d want 0 (batch mode)", sends)
	}
}

// A partial batch still buffered on shutdown is flushed by Stop() (no loss).
func Test_KafKaWriter_BatchMode_ShutdownFlush(t *testing.T) {
	mp := newMockKafkaProducer()
	w := newBatchWriter(t, mp, 10000, 10*time.Second) // neither count nor timer fires
	const n = 7
	writeN(t, w, n)
	w.Stop() // must flush the partial batch on the way out

	if got := len(mp.sent); got != n {
		t.Errorf("mock received=%d want %d (shutdown flush)", got, n)
	}
	// Sent==n is the hard no-data-loss check; Batches>=1 confirms the shutdown
	// flush fired (exact count is incidental — neither count nor the 10s timer
	// can fire in-test, so it is normally 1).
	if m := w.Metrics(); m.Batches < 1 || m.Sent != n {
		t.Errorf("Metrics: Batches=%d want >=1, Sent=%d want %d (shutdown flush)", m.Batches, m.Sent, n)
	}
}

// Regression: default (non-batch) mode still uses per-record Send; Batches=0.
func Test_KafKaWriter_PerRecordMode_NoBatch(t *testing.T) {
	mp := newMockKafkaProducer()
	w := NewKafKaWriter(KafKaWriterOptions{ProducerTopic: "t", BufferSize: 1024}) // BatchMode default false
	w.producerFactory = func() (kafka.Producer, error) { return mp, nil }
	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	const n = 50
	writeN(t, w, n)
	waitSent(t, w, n, "per-record drain")
	w.Stop()

	if sends, batches := mp.callCounts(); sends != n || batches != 0 {
		t.Errorf("calls: Send=%d want %d, SendBatch=%d want 0", sends, n, batches)
	}
	if m := w.Metrics(); m.Batches != 0 || m.BatchMax != 0 {
		t.Errorf("non-batch Metrics: Batches=%d BatchMax=%d want 0", m.Batches, m.BatchMax)
	}
}

// Test_KafKaWriter_BatchFasterWhenSendCostly proves the batch MECHANISM pays off
// when the producer call has per-call cost — the regime where "batch should be
// faster" holds. The slow mock charges a per-CALL latency ONCE per Send/
// SendBatch; per-record pays it N times, batch pays it N/BatchSize times.
// (The real async producer's Send is a ~free enqueue, so its throughput is ≈
// parity — see TestIntegration_KafKaWriter_Throughput; batch helps when Send is
// expensive: sync producer, overloaded broker backing up, heavy per-call work.)
func Test_KafKaWriter_BatchFasterWhenSendCostly(t *testing.T) {
	if testing.Short() {
		t.Skip("timing-sensitive: compares batch vs per-record throughput; unreliable on slow/noisy CI runners")
	}
	const n = 5000
	const delay = 50 * time.Microsecond
	const batchSize = 100

	measure := func(batch bool) (uint64, time.Duration) {
		mp := newMockKafkaProducer()
		mp.callDelay = delay
		opts := KafKaWriterOptions{ProducerTopic: "t", BufferSize: 1 << 14, OverflowPolicy: "block"}
		if batch {
			opts.BatchMode = true
			opts.BatchSize = batchSize
			opts.BatchFlushInterval = time.Second // only count-triggered flushes
		}
		w := NewKafKaWriter(opts)
		w.producerFactory = func() (kafka.Producer, error) { return mp, nil }
		if err := w.Start(); err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		writeN(t, w, n)
		waitSent(t, w, n, "slow-mock drain")
		dur := time.Since(start)
		w.Stop()
		return w.Metrics().Sent, dur
	}

	prSent, prDur := measure(false)
	bSent, bDur := measure(true)
	prRps := float64(prSent) / prDur.Seconds()
	bRps := float64(bSent) / bDur.Seconds()
	t.Logf("per-record (callDelay=%v): %d rec / %v = %.0f rec/s", delay, prSent, prDur, prRps)
	t.Logf("batch(%d, callDelay=%v):   %d rec / %v = %.0f rec/s", batchSize, delay, bSent, bDur, bRps)
	t.Logf("speedup batch/per-record = %.1fx", bRps/prRps)
	if prSent != n || bSent != n {
		t.Errorf("loss: per=%d batch=%d want %d", prSent, bSent, n)
	}
	if bRps < prRps*10 {
		t.Errorf("batch only %.1fx faster (want >10x) — not amortizing per-call cost", bRps/prRps)
	}
}

// ProducerLinger wiring: 0 → not forwarded (kafka default 10ms applies); >0 →
// forwarded; kafka.LingerOff → forwarded (+ MaxBufferedRecords=1 deadlock guard).
// Verified by applying the built options to a fresh kafka.Options (no broker).
func Test_KafKaWriter_ProducerLingerOption(t *testing.T) {
	apply := func(opts []kafka.Option) kafka.Options {
		var o kafka.Options
		for _, opt := range opts {
			opt(&o)
		}
		return o
	}
	w0 := NewKafKaWriter(KafKaWriterOptions{Brokers: []string{"x"}, ProducerTopic: "t"})
	if o := apply(w0.kafkaProducerOpts()); o.ProducerLinger != 0 {
		t.Errorf("default ProducerLinger=%v want 0 (kafka default applies later)", o.ProducerLinger)
	}
	w5 := NewKafKaWriter(KafKaWriterOptions{Brokers: []string{"x"}, ProducerTopic: "t", ProducerLinger: 5 * time.Millisecond})
	if o := apply(w5.kafkaProducerOpts()); o.ProducerLinger != 5*time.Millisecond {
		t.Errorf("ProducerLinger=%v want 5ms", o.ProducerLinger)
	}
	woff := NewKafKaWriter(KafKaWriterOptions{Brokers: []string{"x"}, ProducerTopic: "t", ProducerLinger: kafka.LingerOff})
	o := apply(woff.kafkaProducerOpts())
	if o.ProducerLinger != kafka.LingerOff {
		t.Errorf("ProducerLinger=%v want LingerOff(%v)", o.ProducerLinger, kafka.LingerOff)
	}
	// LingerOff must also cap MaxBufferedRecords=1 (else the default count-trigger
	// Flush.Messages deadlocks under OverflowBlock — see kafkaProducerOpts doc).
	if o.MaxBufferedRecords != 1 {
		t.Errorf("LingerOff MaxBufferedRecords=%d want 1 (deadlock guard)", o.MaxBufferedRecords)
	}
}

// real-time buffer depth (InFlight/BufferedBytes) + Backend; ProducerSnapshot()
// gives full depth (Timestamp/Linger/etc). Verifies the producer's counters
// flow through to both surfaces.
func Test_KafKaWriter_ProducerMetricsBridge(t *testing.T) {
	mp := newMockKafkaProducer()
	w := NewKafKaWriter(KafKaWriterOptions{ProducerTopic: "t", BufferSize: 1024})
	w.producerFactory = func() (kafka.Producer, error) { return mp, nil }
	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	defer w.Stop()

	const n = 50
	writeN(t, w, n)
	waitSent(t, w, n, "bridge drain")

	want := mp.Metrics() // what the producer currently reports
	m := w.Metrics()
	if m.Backend != mp.Backend() {
		t.Errorf("Backend=%q want %q (not bridged)", m.Backend, mp.Backend())
	}
	if m.InFlight != want.InFlight || m.InFlight == 0 {
		t.Errorf("InFlight=%d want %d (>0, bridged from producer)", m.InFlight, want.InFlight)
	}
	if m.BufferedBytes != want.BufferedBytes || m.BufferedBytes == 0 {
		t.Errorf("BufferedBytes=%d want %d (>0, bridged from producer)", m.BufferedBytes, want.BufferedBytes)
	}

	// ProducerSnapshot: full depth — UTC Timestamp + Backend + the same InFlight.
	ps := w.ProducerSnapshot()
	if ps.Backend != mp.Backend() {
		t.Errorf("ProducerSnapshot.Backend=%q want %q", ps.Backend, mp.Backend())
	}
	if ps.Timestamp.IsZero() {
		t.Error("ProducerSnapshot.Timestamp is zero")
	}
	if ps.Timestamp.Location() != time.UTC {
		t.Errorf("ProducerSnapshot.Timestamp location=%v want UTC", ps.Timestamp.Location())
	}
	if ps.InFlight != want.InFlight {
		t.Errorf("ProducerSnapshot.InFlight=%d want %d", ps.InFlight, want.InFlight)
	}
}
