package log4go

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/IBM/sarama"
	"github.com/IBM/sarama/mocks"
)

// Test_KafKaWriter_EndToEndMockProducer drives the full Write -> channel ->
// daemon -> producer.Input() path against a sarama mock AsyncProducer (no real
// broker). It also asserts the sent counter and the real-time onEvent hook.
func Test_KafKaWriter_EndToEndMockProducer(t *testing.T) {
	cfg := sarama.NewConfig()
	cfg.Producer.Return.Successes = true
	mp := mocks.NewAsyncProducer(t, cfg)

	const n = 200
	for i := 0; i < n; i++ {
		mp.ExpectInputAndSucceed()
	}

	var sentEvents int64
	w := NewKafKaWriter(KafKaWriterOptions{ProducerTopic: "t", BufferSize: 1024})
	w.SetOnEvent(func(name string, delta int64) {
		if name == "sent" {
			atomic.AddInt64(&sentEvents, delta)
		}
	})
	// inject the mock producer (no real broker connection).
	w.producerFactory = func() (kafka.Producer, error) {
		return mp, nil
	}

	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond) // let daemon + mock consumer schedule
	for i := 0; i < n; i++ {
		if err := w.Write(&Record{level: INFO, msg: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	// wait for the async daemon to drain
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if w.Metrics().Sent >= n {
			break
		}
		time.Sleep(time.Millisecond)
	}
	w.Stop() // closes producer -> mock asserts all ExpectInput satisfied

	m := w.Metrics()
	if m.Sent != n {
		t.Errorf("Metrics.Sent=%d want %d", m.Sent, n)
	}
	if got := atomic.LoadInt64(&sentEvents); got != n {
		t.Errorf("onEvent sent=%d want %d", got, n)
	}
	if m.Errored != 0 {
		t.Errorf("Metrics.Errored=%d want 0", m.Errored)
	}
}

// Test_KafKaWriter_MockProducerErrors verifies error accounting via the mock.
func Test_KafKaWriter_MockProducerErrors(t *testing.T) {
	cfg := sarama.NewConfig()
	cfg.Producer.Return.Successes = true
	mp := mocks.NewAsyncProducer(t, cfg)

	const n = 50
	for i := 0; i < n; i++ {
		mp.ExpectInputAndFail(errors.New("request timeout"))
	}

	w := NewKafKaWriter(KafKaWriterOptions{ProducerTopic: "t", BufferSize: 1024})
	w.producerFactory = func() (kafka.Producer, error) {
		return mp, nil
	}
	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	// let the daemon + mock consumer goroutines schedule
	time.Sleep(50 * time.Millisecond)
	for i := 0; i < n; i++ {
		_ = w.Write(&Record{level: INFO, msg: "x"})
	}
	// wait for the async daemon to drain (mock consumer is timing-sensitive)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if w.Metrics().Sent >= n {
			break
		}
		time.Sleep(time.Millisecond)
	}
	w.Stop()

	m := w.Metrics()
	if m.Sent < n {
		t.Errorf("Sent=%d want %d", m.Sent, n)
	}
	if m.Errored < n {
		t.Errorf("Errored=%d want %d", m.Errored, n)
	}
}
