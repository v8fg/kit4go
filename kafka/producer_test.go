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
	for i := 0; i < n; i++ {
		mp.ExpectInputAndSucceed()
	}
	p, err := newSaramaProducer(Options{Brokers: []string{"x"}, Topic: "t"}.withDefaults(),
		func([]string, *sarama.Config) (sarama.AsyncProducer, error) { return mp, nil })
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
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
