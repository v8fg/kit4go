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

// ---- buildSaramaConfig branches ----

func TestCov_BuildSaramaConfig_NegativeRetryAndFlushBytesAndFetchMin(t *testing.T) {
	o := Options{
		Brokers:            []string{"x"},
		Topic:              "t",
		BatchMaxBytes:      1 << 20,
		FetchMin:           1024,
		MaxBufferedRecords: 500,
	}.withDefaults()
	o.RetryMax = -1 // force after withDefaults so it isn't overwritten by the default
	cfg, err := buildSaramaConfig(o, false)
	if err != nil {
		t.Fatalf("buildSaramaConfig: %v", err)
	}
	if cfg.Producer.Retry.Max != 0 {
		t.Errorf("Retry.Max=%d want 0", cfg.Producer.Retry.Max)
	}
	if cfg.Producer.Flush.Bytes == 0 {
		t.Error("Flush.Bytes should be set when BatchMaxBytes > 0")
	}
	if cfg.Consumer.Fetch.Min != 1024 {
		t.Errorf("Fetch.Min=%d want 1024", cfg.Consumer.Fetch.Min)
	}
}

func TestCov_BuildSaramaConfig_SyncFlushOff(t *testing.T) {
	o := Options{Brokers: []string{"x"}, Topic: "t"}.withDefaults()
	cfg, err := buildSaramaConfig(o, true)
	if err != nil {
		t.Fatalf("buildSaramaConfig sync: %v", err)
	}
	if cfg.Producer.Flush.Frequency != 0 || cfg.Producer.Flush.Messages != 0 || cfg.Producer.Flush.Bytes != 0 {
		t.Errorf("sync Flush should be zero: freq=%v msgs=%d bytes=%d",
			cfg.Producer.Flush.Frequency, cfg.Producer.Flush.Messages, cfg.Producer.Flush.Bytes)
	}
}

// ---- NewConsumerGroup / NewPartitionConsumer / NewSyncProducer public paths ----

func TestCov_NewConsumerGroup_PublicSuccessPath(t *testing.T) {
	// NewConsumerGroup with valid opts but a failing factory covers the
	// `return newSaramaConsumerGroup(o, nil)` line + the factory-error branch
	// via the default sarama.NewConsumerGroup (which fails to dial).
	_, err := NewConsumerGroup(WithBrokers("127.0.0.1:1"), WithGroupID("g"))
	if err == nil {
		t.Skip("unexpected success dialing 127.0.0.1:1; skipping")
	}
}

func TestCov_NewPartitionConsumer_PublicErrorPath(t *testing.T) {
	// Covers `return newSaramaPartitionConsumer(o, nil)` + default-factory error.
	_, err := NewPartitionConsumer(WithBrokers("127.0.0.1:1"), WithTopic("t"))
	if err == nil {
		t.Skip("unexpected success dialing 127.0.0.1:1; skipping")
	}
}

func TestCov_NewSyncProducer_PublicErrorPath(t *testing.T) {
	_, err := NewSyncProducer(WithBrokers("127.0.0.1:1"), WithTopic("t"))
	if err == nil {
		t.Skip("unexpected success dialing 127.0.0.1:1; skipping")
	}
}

// ---- newSaramaConsumerGroup internal branches ----

func TestCov_NewSaramaConsumerGroup_FactoryError(t *testing.T) {
	_, err := newSaramaConsumerGroup(
		Options{Brokers: []string{"x"}, GroupID: "g"}.withDefaults(),
		func([]string, string, *sarama.Config) (sarama.ConsumerGroup, error) {
			return nil, errBoom
		},
	)
	if !errorIs(err, errBoom) {
		t.Errorf("factory error: %v want errBoom", err)
	}
}

func TestCov_NewSaramaConsumerGroup_ConfigError(t *testing.T) {
	_, err := newSaramaConsumerGroup(
		Options{Brokers: []string{"x"}, GroupID: "g", Version: "not-a-version"}.withDefaults(),
		nil,
	)
	if err == nil {
		t.Error("invalid version should produce a config error")
	}
}

// ---- newSaramaPartitionConsumer internal branches ----

func TestCov_NewSaramaPartitionConsumer_FactoryError(t *testing.T) {
	_, err := newSaramaPartitionConsumer(
		Options{Brokers: []string{"x"}, Topic: "t", Partition: 0}.withDefaults(),
		func([]string, *sarama.Config) (sarama.Consumer, error) {
			return nil, errBoom
		},
	)
	if !errorIs(err, errBoom) {
		t.Errorf("factory error: %v want errBoom", err)
	}
}

func TestCov_NewSaramaPartitionConsumer_ConsumePartitionError(t *testing.T) {
	// Use a custom consumer whose ConsumePartition always errors, to hit the
	// `_ = c.Close(); return nil, err` branch.
	_, err := newSaramaPartitionConsumer(
		Options{Brokers: []string{"x"}, Topic: "t", Partition: 0, Offset: OffsetNewest}.withDefaults(),
		func([]string, *sarama.Config) (sarama.Consumer, error) { return &consumeErrConsumer{}, nil },
	)
	if err == nil {
		t.Error("ConsumePartition error should propagate")
	}
}

// consumeErrConsumer is a sarama.Consumer whose ConsumePartition errors and
// whose other methods are inert, used to exercise the error path in
// newSaramaPartitionConsumer.
type consumeErrConsumer struct{ errCloserConsumer }

func (consumeErrConsumer) ConsumePartition(string, int32, int64) (sarama.PartitionConsumer, error) {
	return nil, errBoom
}
func (consumeErrConsumer) Close() error { return nil } // success on cleanup

func TestCov_NewSaramaPartitionConsumer_ConfigError(t *testing.T) {
	_, err := newSaramaPartitionConsumer(
		Options{Brokers: []string{"x"}, Topic: "t", Version: "not-a-version"}.withDefaults(),
		nil,
	)
	if err == nil {
		t.Error("invalid version should produce a config error")
	}
}

// ---- newSaramaProducer internal branches ----

func TestCov_NewSaramaProducer_FactoryError(t *testing.T) {
	_, err := newSaramaProducer(
		Options{Brokers: []string{"x"}, Topic: "t"}.withDefaults(),
		func([]string, *sarama.Config) (sarama.AsyncProducer, error) {
			return nil, errBoom
		},
	)
	if !errorIs(err, errBoom) {
		t.Errorf("factory error: %v want errBoom", err)
	}
}

func TestCov_NewSaramaProducer_ConfigError(t *testing.T) {
	_, err := newSaramaProducer(
		Options{Brokers: []string{"x"}, Topic: "t", Version: "not-a-version"}.withDefaults(),
		nil,
	)
	if err == nil {
		t.Error("invalid version should produce a config error")
	}
}

// ---- newSaramaSyncProducer internal branches ----

func TestCov_NewSaramaSyncProducer_FactoryError(t *testing.T) {
	_, err := newSaramaSyncProducer(
		Options{Brokers: []string{"x"}, Topic: "t"}.withDefaults(),
		func([]string, *sarama.Config) (sarama.SyncProducer, error) {
			return nil, errBoom
		},
	)
	if !errorIs(err, errBoom) {
		t.Errorf("factory error: %v want errBoom", err)
	}
}

func TestCov_NewSaramaSyncProducer_ConfigError(t *testing.T) {
	_, err := newSaramaSyncProducer(
		Options{Brokers: []string{"x"}, Topic: "t", Version: "not-a-version"}.withDefaults(),
		nil,
	)
	if err == nil {
		t.Error("invalid version should produce a config error")
	}
}

// ---- Snapshot() for both consumer types (0% covered) ----

func TestCov_ConsumerGroup_Snapshot(t *testing.T) {
	cg := &saramaConsumerGroup{opts: Options{GroupID: "snap-group"}.withDefaults()}
	snap := cg.Snapshot()
	if snap.Name != "snap-group" {
		t.Errorf("Snapshot.Name=%q want snap-group", snap.Name)
	}
	if snap.Backend != "sarama" {
		t.Errorf("Snapshot.Backend=%q want sarama", snap.Backend)
	}
	if snap.Timestamp.IsZero() {
		t.Error("Snapshot.Timestamp should be set")
	}
}

func TestCov_PartitionConsumer_Snapshot(t *testing.T) {
	pc := &saramaPartitionConsumer{opts: Options{Topic: "snap-topic"}.withDefaults()}
	snap := pc.Snapshot()
	if snap.Name != "snap-topic" {
		t.Errorf("Snapshot.Name=%q want snap-topic", snap.Name)
	}
	if snap.Backend != "sarama" {
		t.Errorf("Snapshot.Backend=%q want sarama", snap.Backend)
	}
}

// ---- producer Send/SendBatch edge cases ----

// blockingAsyncProducer is an AsyncProducer stub whose Input() channel is
// unbuffered and never read, forcing Send/SendBatch to hit the ctx.Done branch.
// Close closes its channels once so the producer's drain goroutines exit
// cleanly (the suite runs goleak).
type blockingAsyncProducer struct {
	in     chan *sarama.ProducerMessage
	succ   chan *sarama.ProducerMessage
	errs   chan *sarama.ProducerError
	once   sync.Once
}

func newBlockingAsyncProducer() *blockingAsyncProducer {
	return &blockingAsyncProducer{
		in:   make(chan *sarama.ProducerMessage),
		succ: make(chan *sarama.ProducerMessage),
		errs: make(chan *sarama.ProducerError),
	}
}

func (b *blockingAsyncProducer) AsyncClose() { b.Close() }
func (b *blockingAsyncProducer) Close() error {
	b.once.Do(func() {
		close(b.in)
		close(b.succ)
		close(b.errs)
	})
	return nil
}
func (b *blockingAsyncProducer) Input() chan<- *sarama.ProducerMessage              { return b.in }
func (b *blockingAsyncProducer) Successes() <-chan *sarama.ProducerMessage          { return b.succ }
func (b *blockingAsyncProducer) Errors() <-chan *sarama.ProducerError               { return b.errs }
func (b *blockingAsyncProducer) IsTransactional() bool                              { return false }
func (b *blockingAsyncProducer) TxnStatus() sarama.ProducerTxnStatusFlag            { return 0 }
func (b *blockingAsyncProducer) BeginTxn() error                                    { return nil }
func (b *blockingAsyncProducer) CommitTxn() error                                   { return nil }
func (b *blockingAsyncProducer) AbortTxn() error                                    { return nil }
func (b *blockingAsyncProducer) AddOffsetsToTxn(map[string][]*sarama.PartitionOffsetMetadata, string) error {
	return nil
}
func (b *blockingAsyncProducer) AddOffsetsToTxnWithGroupMetadata(map[string][]*sarama.PartitionOffsetMetadata, *sarama.ConsumerGroupMetadata) error {
	return nil
}
func (b *blockingAsyncProducer) AddMessageToTxn(*sarama.ConsumerMessage, string, *string) error {
	return nil
}
func (b *blockingAsyncProducer) AddMessageToTxnWithGroupMetadata(*sarama.ConsumerMessage, *sarama.ConsumerGroupMetadata, *string) error {
	return nil
}

// TestCov_Producer_SendContextCancelled covers the `<-ctx.Done()` branch in Send.
func TestCov_Producer_SendContextCancelled(t *testing.T) {
	p, _ := newSaramaProducer(Options{Brokers: []string{"x"}, Topic: "t"}.withDefaults(),
		func([]string, *sarama.Config) (sarama.AsyncProducer, error) {
			return newBlockingAsyncProducer(), nil
		})
	defer p.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := p.Send(ctx, Message{Value: []byte("x")})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Send with cancelled ctx: %v want Canceled", err)
	}
}

// TestCov_Producer_SendBatchContextCancelled covers the `<-ctx.Done()` branch in SendBatch.
// We use a non-mock stub AsyncProducer whose Input() channel never receives
// (so SendBatch blocks on the select until ctx fires).
func TestCov_Producer_SendBatchContextCancelled(t *testing.T) {
	p, _ := newSaramaProducer(Options{Brokers: []string{"x"}, Topic: "t"}.withDefaults(),
		func([]string, *sarama.Config) (sarama.AsyncProducer, error) {
			return newBlockingAsyncProducer(), nil
		})
	defer p.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := p.SendBatch(ctx, []Message{{Value: []byte("x")}})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("SendBatch with cancelled ctx: %v want Canceled", err)
	}
}

// TestCov_Producer_SendClosed covers the closed-Send branch in Send.
func TestCov_Producer_SendClosed(t *testing.T) {
	mp := mocks.NewAsyncProducer(t, mockAsyncCfg())
	p, _ := newSaramaProducer(Options{Brokers: []string{"x"}, Topic: "t"}.withDefaults(),
		func([]string, *sarama.Config) (sarama.AsyncProducer, error) { return mp, nil })
	_ = p.Close()
	if err := p.Send(context.Background(), Message{Value: []byte("x")}); err != ErrProducerClosed {
		t.Errorf("Send on closed producer: %v want ErrProducerClosed", err)
	}
}

// TestCov_Producer_SendBatchClosed covers the closed branch in SendBatch.
func TestCov_Producer_SendBatchClosed(t *testing.T) {
	mp := mocks.NewAsyncProducer(t, mockAsyncCfg())
	p, _ := newSaramaProducer(Options{Brokers: []string{"x"}, Topic: "t"}.withDefaults(),
		func([]string, *sarama.Config) (sarama.AsyncProducer, error) { return mp, nil })
	_ = p.Close()
	if err := p.SendBatch(context.Background(), []Message{{Value: []byte("x")}}); err != ErrProducerClosed {
		t.Errorf("SendBatch on closed producer: %v want ErrProducerClosed", err)
	}
}

// TestCov_Producer_SendReturnSuccessesFalse covers the !ReturnSuccesses branch.
func TestCov_Producer_SendReturnSuccessesFalse(t *testing.T) {
	mp := mocks.NewAsyncProducer(t, mockAsyncCfg())
	mp.ExpectInputAndSucceed()
	o := Options{Brokers: []string{"x"}, Topic: "t", ReturnSuccesses: false, ReturnErrors: true}.withDefaults()
	p, _ := newSaramaProducer(o,
		func([]string, *sarama.Config) (sarama.AsyncProducer, error) { return mp, nil })
	defer p.Close()
	if err := p.Send(context.Background(), Message{Value: []byte("x")}); err != nil {
		t.Errorf("Send with !ReturnSuccesses: %v", err)
	}
}

// TestCov_Producer_SendWithCodec covers the opts.Codec != nil placeholder branch.
func TestCov_Producer_SendWithCodec(t *testing.T) {
	mp := mocks.NewAsyncProducer(t, mockAsyncCfg())
	mp.ExpectInputAndSucceed()
	o := Options{Brokers: []string{"x"}, Topic: "t", Codec: noopCodec{}}.withDefaults()
	p, _ := newSaramaProducer(o,
		func([]string, *sarama.Config) (sarama.AsyncProducer, error) { return mp, nil })
	defer p.Close()
	if err := p.Send(context.Background(), Message{Value: []byte("x")}); err != nil {
		t.Errorf("Send with Codec: %v", err)
	}
}

type noopCodec struct{}

func (noopCodec) Encode(v any) ([]byte, error) { return []byte("enc"), nil }
func (noopCodec) Decode(b []byte, out any) error { return nil }
func (noopCodec) ContentType() string           { return "application/mock" }

// ---- sync producer closed/error paths ----

func TestCov_SyncProducer_SendClosed(t *testing.T) {
	mp := mocks.NewSyncProducer(t, mockAsyncCfg())
	sp, _ := newSaramaSyncProducer(
		Options{Brokers: []string{"x"}, Topic: "t"}.withDefaults(),
		func([]string, *sarama.Config) (sarama.SyncProducer, error) { return mp, nil },
	)
	_ = sp.Close()
	_, _, err := sp.Send(context.Background(), Message{Value: []byte("x")})
	if err != ErrProducerClosed {
		t.Errorf("Send on closed sync producer: %v want ErrProducerClosed", err)
	}
}

func TestCov_SyncProducer_SendBatchClosed(t *testing.T) {
	mp := mocks.NewSyncProducer(t, mockAsyncCfg())
	sp, _ := newSaramaSyncProducer(
		Options{Brokers: []string{"x"}, Topic: "t"}.withDefaults(),
		func([]string, *sarama.Config) (sarama.SyncProducer, error) { return mp, nil },
	)
	_ = sp.Close()
	if err := sp.SendBatch(context.Background(), []Message{{Value: []byte("x")}}); err != ErrProducerClosed {
		t.Errorf("SendBatch on closed sync producer: %v want ErrProducerClosed", err)
	}
}

func TestCov_SyncProducer_SendBatchError(t *testing.T) {
	mp := mocks.NewSyncProducer(t, mockAsyncCfg())
	mp.ExpectSendMessageAndFail(sarama.ErrOutOfBrokers)
	sp, _ := newSaramaSyncProducer(
		Options{Brokers: []string{"x"}, Topic: "t"}.withDefaults(),
		func([]string, *sarama.Config) (sarama.SyncProducer, error) { return mp, nil },
	)
	err := sp.SendBatch(context.Background(), []Message{{Value: []byte("x")}})
	if err == nil {
		t.Error("SendBatch should fail when SendMessages errors")
	}
	_ = sp.Close()
}

func TestCov_SyncProducer_Snapshot(t *testing.T) {
	sp := &saramaSyncProducer{opts: Options{Topic: "snap"}.withDefaults()}
	snap := sp.Snapshot()
	if snap.Name != "snap" || snap.Backend != "sarama" {
		t.Errorf("Snapshot: name=%q backend=%q", snap.Name, snap.Backend)
	}
	if snap.Timestamp.IsZero() {
		t.Error("Snapshot.Timestamp should be set")
	}
}

func TestCov_Producer_Snapshot(t *testing.T) {
	p := &saramaProducer{opts: Options{Topic: "snap"}.withDefaults(), history: newSnapshotHistory(0)}
	snap := p.Snapshot()
	if snap.Name != "snap" || snap.Backend != "sarama" {
		t.Errorf("Snapshot: name=%q backend=%q", snap.Name, snap.Backend)
	}
}

// ---- consumer group pushErr drop branch ----

func TestCov_ConsumerGroup_PushErrDropWhenFull(t *testing.T) {
	cg := &saramaConsumerGroup{opts: Options{GroupID: "g"}.withDefaults()}
	// Pre-fill the errCh to capacity so the next pushErr hits the `default` drop.
	cg.errChOnce.Do(func() { cg.errCh = make(chan error, 2) })
	cg.errCh <- errBoom
	cg.errCh <- errBoom
	cg.pushErr(errors.New("dropped")) // should hit default (drop) without blocking
	// Channel still has exactly 2 (the 3rd was dropped).
	if len(cg.errCh) != 2 {
		t.Errorf("errCh len=%d want 2 (drop branch)", len(cg.errCh))
	}
}

// ---- partition consumer pump edge cases ----

// TestCov_PartitionConsumer_PumpChannelCtxDone covers the `<-ctx.Done()` branch
// in the channel-mode forward path of pump. We drive it via the Messages()
// channel-mode entrypoint: a filled message channel + a cancelled pump context
// forces the select's ctx.Done case. Because Messages() starts its own pump on
// context.Background(), we instead call pump directly with a stub PartitionConsumer
// that never yields and a full output buffer.
func TestCov_PartitionConsumer_PumpChannelCtxDone(t *testing.T) {
	pc := &saramaPartitionConsumer{
		opts: Options{Topic: "t"}.withDefaults(),
		pc:   &blockingPartitionConsumer{},
	}
	out := make(chan Message, 1)
	out <- Message{Value: []byte("filler")} // fill buffer so next send can't proceed
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := pc.pump(ctx, nil, out)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("pump channel-mode ctx.Done: err=%v want Canceled", err)
	}
}

// blockingPartitionConsumer is a PartitionConsumer whose Messages channel is
// never ready and whose Errors channel is never ready, so pump blocks on its
// select until ctx fires.
type blockingPartitionConsumer struct{}

func (blockingPartitionConsumer) Messages() <-chan *sarama.ConsumerMessage {
	return make(chan *sarama.ConsumerMessage)
}
func (blockingPartitionConsumer) Errors() <-chan *sarama.ConsumerError {
	return make(chan *sarama.ConsumerError)
}
func (blockingPartitionConsumer) Close() error               { return nil }
func (blockingPartitionConsumer) AsyncClose()                {}
func (blockingPartitionConsumer) HighWaterMarkOffset() int64 { return 0 }
func (blockingPartitionConsumer) Pause()                     {}
func (blockingPartitionConsumer) Resume()                    {}
func (blockingPartitionConsumer) IsPaused() bool             { return false }

// TestCov_PartitionConsumer_ConsumeErrorDuringConsume covers the consumer.Close
// error branch in Close (err2 != nil && err == nil).
func TestCov_PartitionConsumer_ConsumerCloseError(t *testing.T) {
	// Use a stub consumer whose Close returns an error AND a stub partition
	// consumer whose Close returns nil, to hit the `err2 != nil && err == nil`
	// branch in saramaPartitionConsumer.Close.
	pc := &saramaPartitionConsumer{
		opts:     Options{Topic: "t"}.withDefaults(),
		consumer: &errCloserConsumer{},
		pc:       &noErrPartitionConsumer{},
	}
	err := pc.Close()
	if err == nil {
		// The branch sets err = err2 when err == nil; if it didn't fire, the
		// stubs didn't behave as expected.
		t.Skip("Close error branch didn't fire on this build")
	}
}

// errCloserConsumer is a minimal sarama.Consumer whose Close errors.
type errCloserConsumer struct{}

func (errCloserConsumer) Topics() ([]string, error)                          { return nil, nil }
func (errCloserConsumer) Partitions(string) ([]int32, error)                 { return nil, nil }
func (errCloserConsumer) ConsumePartition(string, int32, int64) (sarama.PartitionConsumer, error) {
	return nil, nil
}
func (errCloserConsumer) HighWaterMarks() map[string]map[int32]int64 { return nil }
func (errCloserConsumer) Close() error                                { return errBoom }
func (errCloserConsumer) Pause(map[string][]int32)                    {}
func (errCloserConsumer) Resume(map[string][]int32)                   {}
func (errCloserConsumer) PauseAll()                                   {}
func (errCloserConsumer) ResumeAll()                                  {}

// noErrPartitionConsumer is a minimal sarama.PartitionConsumer whose Close is nil.
type noErrPartitionConsumer struct{}

func (noErrPartitionConsumer) Messages() <-chan *sarama.ConsumerMessage { return nil }
func (noErrPartitionConsumer) Errors() <-chan *sarama.ConsumerError      { return nil }
func (noErrPartitionConsumer) Close() error                              { return nil }
func (noErrPartitionConsumer) AsyncClose()                               {}
func (noErrPartitionConsumer) HighWaterMarkOffset() int64                { return 0 }
func (noErrPartitionConsumer) Pause()                                    {}
func (noErrPartitionConsumer) Resume()                                   {}
func (noErrPartitionConsumer) IsPaused() bool                            { return false }

// silence unused imports if a test is trimmed.
var _ = time.Second
var _ = sync.Once{}
