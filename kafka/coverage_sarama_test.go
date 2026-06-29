//go:build !franzgo

package kafka

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/IBM/sarama"
	"github.com/IBM/sarama/mocks"
)

// ---- sync producer coverage ----

func TestCoverage_SyncSendBatch(t *testing.T) {
	mp := mocks.NewSyncProducer(t, mockAsyncCfg())
	mp.ExpectSendMessageAndSucceed()
	mp.ExpectSendMessageAndSucceed()
	sp, _ := newSaramaSyncProducer(
		Options{Brokers: []string{"x"}, Topic: "t"}.withDefaults(),
		func([]string, *sarama.Config) (sarama.SyncProducer, error) { return mp, nil },
	)
	if err := sp.SendBatch(context.Background(), []Message{
		{Value: []byte("a")}, {Value: []byte("b")},
	}); err != nil {
		t.Fatalf("SendBatch: %v", err)
	}
	_ = sp.Close()
}

func TestCoverage_SyncSetOnEvent(t *testing.T) {
	sp := &saramaSyncProducer{opts: Options{Topic: "t"}}
	var got string
	sp.SetOnEvent(func(e ProducerEvent) { got = e.Name })
	sp.fire(ProducerEvent{Name: "send"})
	if got != "send" {
		t.Errorf("fire: %q want send", got)
	}
	sp.SetOnEvent(nil)
	sp.fire(ProducerEvent{Name: "should-not-fire"})
}

func TestCoverage_NewSyncProducer_NoBrokers(t *testing.T) {
	_, err := NewSyncProducer()
	if err == nil {
		t.Error("NewSyncProducer with no brokers should error")
	}
}

func TestCoverage_SyncSendError(t *testing.T) {
	mp := mocks.NewSyncProducer(t, mockAsyncCfg())
	mp.ExpectSendMessageAndFail(sarama.ErrOutOfBrokers)
	sp, _ := newSaramaSyncProducer(
		Options{Brokers: []string{"x"}, Topic: "t"}.withDefaults(),
		func([]string, *sarama.Config) (sarama.SyncProducer, error) { return mp, nil },
	)
	_, _, err := sp.Send(context.Background(), Message{Value: []byte("x")})
	if err == nil {
		t.Error("Send should fail")
	}
	_ = sp.Close()
}

// ---- consumer group accessors (direct struct, no broker needed) ----

func TestCoverage_ConsumerGroupNameBackend(t *testing.T) {
	cg := &saramaConsumerGroup{opts: Options{GroupID: "my-group"}}
	if cg.Name() != "my-group" {
		t.Errorf("Name=%q want my-group", cg.Name())
	}
	if cg.Backend() != "sarama" {
		t.Errorf("Backend=%q want sarama", cg.Backend())
	}
}

func TestCoverage_ConsumerGroupSetOnEvent(t *testing.T) {
	cg := &saramaConsumerGroup{opts: Options{GroupID: "g"}}
	var got string
	cg.SetOnEvent(func(e ConsumerEvent) { got = e.Name })
	cg.fire(ConsumerEvent{Name: "ack"})
	if got != "ack" {
		t.Errorf("fire: %q want ack", got)
	}
	cg.SetOnEvent(nil)
	cg.fire(ConsumerEvent{Name: "x"})
}

func TestCoverage_ConsumerGroupClosedConsume(t *testing.T) {
	cg := &saramaConsumerGroup{opts: Options{GroupID: "g"}.withDefaults(), closed: true}
	err := cg.Consume(context.Background(), []string{"t"}, func(Message) error { return nil })
	if err != ErrProducerClosed {
		t.Errorf("Consume on closed CG: err=%v want ErrProducerClosed", err)
	}
}

func TestCoverage_PushErr(t *testing.T) {
	cg := &saramaConsumerGroup{opts: Options{GroupID: "g"}.withDefaults()}
	cg.pushErr(errBoom)
}

// ---- partition consumer accessors ----

func TestCoverage_PartitionConsumerAccessors(t *testing.T) {
	pc := &saramaPartitionConsumer{opts: Options{Topic: "mytopic"}}
	if pc.Name() != "mytopic" {
		t.Errorf("Name=%q want mytopic", pc.Name())
	}
	if pc.Backend() != "sarama" {
		t.Errorf("Backend=%q want sarama", pc.Backend())
	}
	var got string
	pc.SetOnEvent(func(e ConsumerEvent) { got = e.Name })
	pc.fire(ConsumerEvent{Name: "message"})
	if got != "message" {
		t.Errorf("fire: %q want message", got)
	}
	pc.SetOnEvent(nil)
	pc.fire(ConsumerEvent{Name: "x"})
}

func TestCoverage_PartitionConsumerClosedConsume(t *testing.T) {
	pc := &saramaPartitionConsumer{opts: Options{Topic: "t"}.withDefaults(), closed: true}
	err := pc.Consume(context.Background(), func(Message) error { return nil })
	if err != ErrProducerClosed {
		t.Errorf("Consume on closed PC: err=%v want ErrProducerClosed", err)
	}
}

// ---- handler mappers (full coverage with headers/key) ----

func TestCoverage_HandlerMappers(t *testing.T) {
	msg := Message{
		Topic:   "t",
		Key:     []byte("key"),
		Value:   []byte("val"),
		Headers: []Header{{Key: []byte("hk"), Value: []byte("hv")}},
	}
	pm := toSaramaProducerMessage(msg, "t")
	if pm.Topic != "t" || pm.Key.Length() != 3 || pm.Value.Length() != 3 {
		t.Errorf("toSaramaProducerMessage: topic=%q keyLen=%d valLen=%d", pm.Topic, pm.Key.Length(), pm.Value.Length())
	}
	if len(pm.Headers) != 1 {
		t.Errorf("headers len=%d want 1", len(pm.Headers))
	}
	// nil key/value
	pm2 := toSaramaProducerMessage(Message{}, "fallback")
	if pm2.Topic != "fallback" || pm2.Key != nil {
		t.Errorf("fallback: topic=%q key=%v", pm2.Topic, pm2.Key)
	}

	// fromSaramaConsumerMessage with + without headers
	cm := &sarama.ConsumerMessage{
		Topic:   "ct",
		Headers: []*sarama.RecordHeader{{Key: []byte("h1"), Value: []byte("v1")}},
	}
	m := fromSaramaConsumerMessage(cm)
	if m.Topic != "ct" || len(m.Headers) != 1 || string(m.Headers[0].Key) != "h1" {
		t.Errorf("fromSaramaConsumerMessage: %+v", m)
	}
	m2 := fromSaramaConsumerMessage(&sarama.ConsumerMessage{Topic: "nh"})
	if len(m2.Headers) != 0 {
		t.Errorf("no headers: %+v", m2.Headers)
	}
}

// ---- encLen nil ----

func TestCoverage_EncLenNil(t *testing.T) {
	if got := encLen(nil); got != 0 {
		t.Errorf("encLen(nil)=%d want 0", got)
	}
}

// ---- config: sync=true + version error ----

func TestCoverage_BuildConfigSyncTrue(t *testing.T) {
	o := Options{Brokers: []string{"x"}, Topic: "t"}.withDefaults()
	cfg, err := buildSaramaConfig(o, true)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Producer.Flush.Frequency != 0 || cfg.Producer.Flush.Messages != 0 {
		t.Errorf("sync config Flush should be off: freq=%v msgs=%d", cfg.Producer.Flush.Frequency, cfg.Producer.Flush.Messages)
	}
}

func TestCoverage_BuildConfigVersionError(t *testing.T) {
	o := Options{Brokers: []string{"x"}, Version: "not-a-version"}
	_, err := buildSaramaConfig(o, false)
	if err == nil {
		t.Error("invalid version should error")
	}
}

// ---- stubConsumerGroup for testing Close/Consume/drainErrors/newSaramaConsumerGroup ----

type stubConsumerGroup struct {
	mu         sync.Mutex
	errCh      chan error
	consumeErr error
	cancel     context.CancelFunc
	calls      int
	closed     bool
}

func (s *stubConsumerGroup) Consume(ctx context.Context, topics []string, h sarama.ConsumerGroupHandler) error {
	s.mu.Lock()
	s.calls++
	cancel := s.cancel
	ce := s.consumeErr
	s.mu.Unlock()
	if cancel != nil && s.calls == 1 {
		cancel()
	}
	return ce
}
func (s *stubConsumerGroup) Errors() <-chan error      { return s.errCh }
func (s *stubConsumerGroup) Close() error              { s.closed = true; close(s.errCh); return nil }
func (s *stubConsumerGroup) Pause(map[string][]int32)  {}
func (s *stubConsumerGroup) PauseAll()                 {}
func (s *stubConsumerGroup) Resume(map[string][]int32) {}
func (s *stubConsumerGroup) ResumeAll()                {}

func TestCoverage_ConsumerGroupFullLifecycle(t *testing.T) {
	stub := &stubConsumerGroup{errCh: make(chan error, 2)}
	cg, err := newSaramaConsumerGroup(
		Options{Brokers: []string{"x"}, GroupID: "g"}.withDefaults(),
		func([]string, string, *sarama.Config) (sarama.ConsumerGroup, error) { return stub, nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	// drainErrors: push error → forwarded to cg.Errors()
	stub.errCh <- errBoom
	time.Sleep(100 * time.Millisecond)
	select {
	case e := <-cg.Errors():
		if !errors.Is(e, errBoom) {
			t.Errorf("drainErrors forwarded %v want errBoom", e)
		}
	default:
		t.Error("drainErrors didn't forward")
	}
	// Close
	if err := cg.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if !stub.closed {
		t.Error("stub not closed")
	}
}

func TestCoverage_ConsumerGroupConsumeRebalance(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	stub := &stubConsumerGroup{errCh: make(chan error, 1), cancel: cancel}
	cg := &saramaConsumerGroup{opts: Options{GroupID: "g"}.withDefaults(), cg: stub}
	err := cg.Consume(ctx, []string{"t"}, func(Message) error { return nil })
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Consume rebalance: err=%v want Canceled", err)
	}
	if stub.calls < 1 {
		t.Error("stub.Consume not called")
	}
}

func TestCoverage_ConsumerGroupConsumeError(t *testing.T) {
	stub := &stubConsumerGroup{errCh: make(chan error, 1), consumeErr: errBoom}
	cg := &saramaConsumerGroup{opts: Options{GroupID: "g"}.withDefaults(), cg: stub}
	var eventFired string
	cg.SetOnEvent(func(e ConsumerEvent) { eventFired = e.Name })
	err := cg.Consume(context.Background(), []string{"t"}, func(Message) error { return nil })
	if !errors.Is(err, errBoom) {
		t.Errorf("Consume error: err=%v want errBoom", err)
	}
	if eventFired != "error" {
		t.Errorf("event: %q want error", eventFired)
	}
}

func TestCoverage_ConsumerGroupCloseIdempotent(t *testing.T) {
	stub := &stubConsumerGroup{errCh: make(chan error, 1)}
	cg := &saramaConsumerGroup{opts: Options{GroupID: "g"}.withDefaults(), cg: stub}
	_ = cg.Close()
	_ = cg.Close() // should not panic
}

func TestCoverage_NewConsumerGroupNoBrokers(t *testing.T) {
	_, err := NewConsumerGroup() // no brokers → validate error
	if err == nil {
		t.Error("NewConsumerGroup with no brokers should error")
	}
}

func TestCoverage_NewConsumerGroupNoGroupID(t *testing.T) {
	_, err := NewConsumerGroup(WithBrokers("x")) // no group_id → validate error
	if err == nil {
		t.Error("NewConsumerGroup without GroupID should error")
	}
}

func TestCoverage_NewPartitionConsumerNoTopic(t *testing.T) {
	_, err := NewPartitionConsumer(WithBrokers("x")) // no topic → validate error
	if err == nil {
		t.Error("NewPartitionConsumer without Topic should error")
	}
}

func TestCoverage_PartitionConsumerCallbackNilChannels(t *testing.T) {
	pc := &saramaPartitionConsumer{opts: Options{Topic: "t", DeliveryMode: "callback"}.withDefaults()}
	if ch := pc.Messages(); ch != nil {
		t.Errorf("callback Messages: %v want nil", ch)
	}
	_ = pc.Errors()
}
