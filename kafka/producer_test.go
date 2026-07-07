//go:build !franzgo

package kafka

import (
	"context"
	"sync"
	"testing"

	"github.com/IBM/sarama"
	"github.com/IBM/sarama/mocks"
)

// mockAsyncCfg builds a sarama config suitable for the mock async producer
// (Return.Successes must match the producer's so the success drain works).
func mockAsyncCfg() *sarama.Config {
	cfg := sarama.NewConfig()
	cfg.Producer.Return.Successes = true
	cfg.Producer.Return.Errors = true
	return cfg
}

func TestProducer_Async_SendSuccess(t *testing.T) {
	mp := mocks.NewAsyncProducer(t, mockAsyncCfg())
	const n = 5
	for range n {
		mp.ExpectInputAndSucceed()
	}
	p, err := newSaramaProducer(Options{Brokers: []string{"x"}, Topic: "t"}.withDefaults(),
		func([]string, *sarama.Config) (sarama.AsyncProducer, error) { return mp, nil })
	if err != nil {
		t.Fatal(err)
	}
	for i := range n {
		if err := p.Send(context.Background(), Message{Value: []byte("hi")}); err != nil {
			t.Fatalf("Send[%d]: %v", i, err)
		}
	}
	waitUntil(t, func() bool { return p.Metrics().Enqueued == n }, "enqueue")
	waitUntil(t, func() bool { return p.Metrics().Success == n }, "success drain")
	if got := p.Metrics().Failed; got != 0 {
		t.Errorf("Failed=%d want 0", got)
	}
	if err := p.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestProducer_Async_SendError(t *testing.T) {
	mp := mocks.NewAsyncProducer(t, mockAsyncCfg())
	mp.ExpectInputAndFail(errBoom) // broker error -> Errors() drain
	p, err := newSaramaProducer(Options{Brokers: []string{"x"}, Topic: "t"}.withDefaults(),
		func([]string, *sarama.Config) (sarama.AsyncProducer, error) { return mp, nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Send(context.Background(), Message{Value: []byte("x")}); err != nil {
		t.Fatalf("Send: %v", err) // async: enqueue succeeds; failure is async
	}
	waitUntil(t, func() bool { return p.Metrics().Failed == 1 }, "error drain")
	_ = p.Close()
}

func TestProducer_Async_OnEvent(t *testing.T) {
	mp := mocks.NewAsyncProducer(t, mockAsyncCfg())
	mp.ExpectInputAndSucceed()
	p, _ := newSaramaProducer(Options{Brokers: []string{"x"}, Topic: "t"}.withDefaults(),
		func([]string, *sarama.Config) (sarama.AsyncProducer, error) { return mp, nil })

	// OnEvent fires from Send's goroutine AND the drain goroutine, so the hook
	// must be concurrency-safe (the documented contract).
	var mu sync.Mutex
	var got []string
	p.SetOnEvent(func(e ProducerEvent) {
		mu.Lock()
		got = append(got, e.Name)
		mu.Unlock()
	})
	_ = p.Send(context.Background(), Message{Value: []byte("x")})
	waitUntil(t, func() bool { return p.Metrics().Success == 1 }, "success")
	_ = p.Close()
	has := func(s string) bool {
		mu.Lock()
		defer mu.Unlock()
		return contains(got, s)
	}
	waitUntil(t, func() bool { return has("send") && has("success") && has("close") }, "events")
}

func TestProducer_Async_CloseIdempotent(t *testing.T) {
	mp := mocks.NewAsyncProducer(t, mockAsyncCfg())
	p, _ := newSaramaProducer(Options{Brokers: []string{"x"}, Topic: "t"}.withDefaults(),
		func([]string, *sarama.Config) (sarama.AsyncProducer, error) { return mp, nil })
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	if err := p.Close(); err != nil { // second Close is a no-op
		t.Errorf("second Close: %v", err)
	}
}

func TestProducer_Async_SendAfterClose(t *testing.T) {
	mp := mocks.NewAsyncProducer(t, mockAsyncCfg())
	p, _ := newSaramaProducer(Options{Brokers: []string{"x"}, Topic: "t"}.withDefaults(),
		func([]string, *sarama.Config) (sarama.AsyncProducer, error) { return mp, nil })
	_ = p.Close()
	if err := p.Send(context.Background(), Message{Value: []byte("x")}); err != ErrProducerClosed {
		t.Errorf("Send after Close: got %v want ErrProducerClosed", err)
	}
}

// TestProducer_Async_SuccessEvent_HasPartitionOffset verifies the async success
// event surfaces the broker-assigned Partition/Offset (the info a sarama
// Successes() channel would give), so the async path loses nothing.
func TestProducer_Async_SuccessEvent_HasPartitionOffset(t *testing.T) {
	mp := mocks.NewAsyncProducer(t, mockAsyncCfg())
	mp.ExpectInputAndSucceed()
	mp.ExpectInputAndSucceed()
	p, _ := newSaramaProducer(Options{Brokers: []string{"x"}, Topic: "t"}.withDefaults(),
		func([]string, *sarama.Config) (sarama.AsyncProducer, error) { return mp, nil })

	var mu sync.Mutex
	var successes []ProducerEvent
	p.SetOnEvent(func(e ProducerEvent) {
		if e.Name == "success" {
			mu.Lock()
			successes = append(successes, e)
			mu.Unlock()
		}
	})
	_ = p.Send(context.Background(), Message{Value: []byte("a")})
	_ = p.Send(context.Background(), Message{Value: []byte("bb")})
	waitUntil(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(successes) == 2
	}, "2 successes")
	_ = p.Close()

	mu.Lock()
	defer mu.Unlock()
	if len(successes) != 2 {
		t.Fatalf("got %d successes want 2", len(successes))
	}
	// the mock assigns partition (via partitioner) + incrementing offset
	for _, e := range successes {
		if e.Offset < 0 {
			t.Errorf("success event Offset<0: %+v", e)
		}
		if e.Bytes == 0 {
			t.Errorf("success event Bytes=0: %+v", e)
		}
	}
	// two messages -> two distinct offsets (mock increments)
	if successes[0].Offset == successes[1].Offset {
		t.Errorf("offsets not distinct: %d == %d", successes[0].Offset, successes[1].Offset)
	}
}

func TestProducer_ValidateNoBrokers(t *testing.T) {
	if _, err := NewProducer(); err == nil {
		t.Error("NewProducer with no brokers should error")
	}
}

func TestProducer_VersionParseError(t *testing.T) {
	_, err := NewProducer(WithBrokers("x"), WithVersion("not-a-version"))
	if err == nil {
		t.Fatal("invalid version should error")
	}
}
