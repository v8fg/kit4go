//go:build !franzgo

package kafka

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/IBM/sarama"
)

// This file closes the last reachable coverage gaps in the sarama backend of
// the kafka package. Each test targets one previously-uncovered block; the
// genuinely unreachable defensive branches are documented inline rather than
// exercised, per the "do not contort tests for impossible branches" rule.

// ---- sarama_config.go:35 — valid Version string parses and is assigned ----

// TestCovFinal_BuildSaramaConfig_ValidVersion covers the `ver = v` success
// branch when Options.Version is a real Kafka version string. Existing tests
// only exercised the empty-Version default and the parse-error path.
func TestCovFinal_BuildSaramaConfig_ValidVersion(t *testing.T) {
	o := Options{Brokers: []string{"x"}, Topic: "t", Version: "2.8.0"}
	cfg, err := buildSaramaConfig(o, false)
	if err != nil {
		t.Fatalf("buildSaramaConfig with valid version: %v", err)
	}
	// sarama.KafkaVersion is a [10]int array; compare by value, and prove the
	// default (V2_5_0_0) was NOT used (the `ver = v` branch ran).
	if cfg.Version != sarama.V2_8_0_0 {
		t.Errorf("Version=%v want V2_8_0_0 (parsed branch)", cfg.Version)
	}
	if cfg.Version == sarama.V2_5_0_0 {
		t.Error("Version is still the default V2_5_0_0; parsed-assignment branch did not run")
	}
}

// ---- sarama_consumer_group.go:104 — top-of-loop ctx.Err() check ----
// ---- sarama_consumer_group.go:116 — clean-return rebalance accounting ----
//
// Consume's loop checks ctx.Err() at the TOP of each iteration (line 104) and
// again after a clean (nil) return from the underlying sarama Consume (line
// 113). To hit line 104 the loop must iterate more than once: the first
// iteration's underlying Consume returns nil (a rebalance) WITHOUT cancelling
// ctx, so the post-call check at 113 passes and line 116 records the rebalance;
// the loop then re-enters and the top-of-loop check at 104 fires because we
// cancel between iterations.
//
// rebalanceThenCancelStub.Consume always returns nil (a clean rebalance) and
// never cancels ctx itself. The test cancels ctx from the rebalance event
// handler so the loop's second top-of-loop ctx.Err() check (line 104) observes
// the cancellation. Returning nil each iteration makes Consume's loop believe a
// rebalance happened and re-iterate.
type rebalanceThenCancelStub struct {
	errCh  chan error
	calls  int32
	closed bool
}

func (s *rebalanceThenCancelStub) Consume(ctx context.Context, topics []string, h sarama.ConsumerGroupHandler) error {
	atomic.AddInt32(&s.calls, 1)
	_ = ctx
	_ = topics
	_ = h
	return nil
}
func (s *rebalanceThenCancelStub) Errors() <-chan error      { return s.errCh }
func (s *rebalanceThenCancelStub) Close() error              { s.closed = true; close(s.errCh); return nil }
func (s *rebalanceThenCancelStub) Pause(map[string][]int32)  {}
func (s *rebalanceThenCancelStub) PauseAll()                 {}
func (s *rebalanceThenCancelStub) Resume(map[string][]int32) {}
func (s *rebalanceThenCancelStub) ResumeAll()                {}

func TestCovFinal_ConsumerGroupConsume_TopLoopCtxDoneAndRebalance(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// stub.Consume always returns nil (a clean rebalance); it never cancels ctx
	// itself. Instead the rebalance event handler below cancels ctx AFTER line
	// 116 fires, so the loop's SECOND top-of-loop ctx.Err() check (line 104)
	// observes the cancellation and returns. This is the only ordering that
	// reaches line 104: line 116 must run first (it is after the first clean
	// return), and ctx cannot be cancelled before then or line 113 short-circuits.
	stub := &rebalanceThenCancelStub{errCh: make(chan error, 1)}
	cg := &saramaConsumerGroup{opts: Options{GroupID: "g"}.withDefaults(), cg: stub}

	var mu sync.Mutex
	var events []string
	cg.SetOnEvent(func(e ConsumerEvent) {
		mu.Lock()
		events = append(events, e.Name)
		if e.Name == "rebalance" {
			// Cancel now: line 116 has already executed, and the loop is about to
			// re-enter, so the top-of-loop ctx.Err() (line 104) will fire.
			cancel()
		}
		mu.Unlock()
	})

	err := cg.Consume(ctx, []string{"t"}, func(Message) error { return nil })
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Consume: err=%v want context.Canceled", err)
	}
	// fire() is called inline within Consume, so once Consume has returned all
	// synchronous events have been emitted.
	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, e := range events {
		if e == "rebalance" {
			found = true
		}
	}
	if !found {
		t.Errorf("rebalance event not fired; events=%v", events)
	}
	if got := atomic.LoadInt32(&stub.calls); got < 1 {
		t.Errorf("stub.Consume calls=%d want >=1", got)
	}
}

// ---- sarama_partition_consumer.go:131 — channel-mode forward ctx.Done ----
//
// pump's channel-mode select has an inner `case <-ctx.Done()` (line 131) that
// fires only when a message has been received, the out channel is full, AND ctx
// is cancelled while waiting to forward. The existing
// TestCov_PartitionConsumer_PumpChannelCtxDone misses this because its stub
// never delivers a message, so pump's OUTER ctx.Done (line 142) fires instead.
// Here the stub yields exactly one message; the full out buffer plus a
// cancelled ctx forces the inner branch.
type yieldingPartitionConsumer struct {
	msgs chan *sarama.ConsumerMessage
}

func (y *yieldingPartitionConsumer) Messages() <-chan *sarama.ConsumerMessage { return y.msgs }
func (y *yieldingPartitionConsumer) Errors() <-chan *sarama.ConsumerError {
	return make(chan *sarama.ConsumerError)
}
func (y *yieldingPartitionConsumer) Close() error               { return nil }
func (y *yieldingPartitionConsumer) AsyncClose()                {}
func (y *yieldingPartitionConsumer) HighWaterMarkOffset() int64 { return 0 }
func (y *yieldingPartitionConsumer) Pause()                     {}
func (y *yieldingPartitionConsumer) Resume()                    {}
func (y *yieldingPartitionConsumer) IsPaused() bool             { return false }

func TestCovFinal_PartitionConsumer_PumpForwardCtxDone(t *testing.T) {
	msgs := make(chan *sarama.ConsumerMessage, 1)
	msgs <- &sarama.ConsumerMessage{Topic: "t", Value: []byte("payload")}
	pc := &saramaPartitionConsumer{
		opts: Options{Topic: "t"}.withDefaults(),
		pc:   &yieldingPartitionConsumer{msgs: msgs},
	}
	out := make(chan Message, 1)
	out <- Message{Value: []byte("filler")} // fill buffer so the forward send cannot proceed

	// Run pump on a live (uncancelled) ctx so the OUTER select reliably picks
	// the Messages case (ctx.Done is not ready). pump then blocks inside the
	// inner `out <- msg` select because `out` is full. Cancelling ctx at that
	// point makes the inner `case <-ctx.Done()` (line 131) the only ready case.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- pc.pump(ctx, nil, out) }()

	// Wait until pump is blocked forwarding: the filler is still in `out` and
	// pump has consumed the message (msgs drained). Poll msgs length.
	waitUntil(t, func() bool { return len(msgs) == 0 }, "pump to consume the message")
	// Give pump a moment to settle into the inner select before we cancel.
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("pump forward ctx.Done: err=%v want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("pump did not return after ctx cancel")
	}
}

// ---- sarama_partition_consumer.go:158 — pushErr default-drop branch ----
//
// pushErr uses a non-blocking select with a `default` drop when errCh is full.
// TestCov_ConsumerGroup_PushErrDropWhenFull covers the consumer-group variant;
// this covers the partition-consumer variant.
func TestCovFinal_PartitionConsumer_PushErrDropWhenFull(t *testing.T) {
	pc := &saramaPartitionConsumer{opts: Options{Topic: "t"}.withDefaults()}
	pc.errChOnce.Do(func() { pc.errCh = make(chan error, 1) })
	pc.errCh <- errBoom               // fill to capacity
	pc.pushErr(errors.New("dropped")) // must hit the default branch without blocking
	if len(pc.errCh) != 1 {
		t.Errorf("errCh len=%d want 1 (drop branch)", len(pc.errCh))
	}
}

// ---- sarama_producer.go:65 — nil factory defaults to sarama.NewAsyncProducer ----
//
// newSaramaProducer(o, nil) is the path taken by the public NewProducer. The
// default-factory assignment (line 65) only runs when config building succeeds
// AND factory is nil. We pass nil factory and a broker that refuses to dial so
// the subsequent factory(...) call fails — covering line 65 without needing a
// live broker.
func TestCovFinal_NewSaramaProducer_NilFactoryDefault(t *testing.T) {
	_, err := newSaramaProducer(
		Options{Brokers: []string{"127.0.0.1:1"}, Topic: "t"}.withDefaults(),
		nil, // force the `factory == nil` branch (line 65)
	)
	if err == nil {
		t.Skip("unexpected success dialing 127.0.0.1:1; skipping")
	}
}

// ---- snapshot.go:88 — size==0 early return on a non-nil history ----
//
// Every existing snapshot test records at least one sample first, so the
// `size == 0 → return nil` guard on a non-nil history is never exercised.
func TestCovFinal_SnapshotHistory_EmptyNonNilReturnsNil(t *testing.T) {
	h := newSnapshotHistory(4)
	if got := h.snapshot(); got != nil {
		t.Errorf("empty non-nil history snapshot()=%v want nil", got)
	}
}

// ---- DOCUMENTED UNREACHABLE BRANCHES (no test; impossible by construction) ----
//
// sarama_partition_consumer.go:136-138 — `case perr, ok := <-s.pc.Errors():
// if !ok { return nil }`. The !ok branch fires only if sarama closes a
// PartitionConsumer's Errors() channel independently of Messages(). Sarama
// never does this: Errors() and Messages() are drained together and the stream
// is terminated by closing Messages() (handled at line 113) or by ctx
// cancellation (line 142). The Errors channel is closed only as part of overall
// Close(), at which point Messages() also closes and pump has already returned.
// Reaching !ok here would require a sarama-internal invariant violation, so the
// branch is a pure defensive guard and is intentionally left uncovered.
//
// sarama_producer.go:99-103 — `if !s.opts.ReturnSuccesses { /* comment */ }`.
// This block contains only a clarifying comment and zero executable statements
// (the actual send is unconditional; the comment documents that success
// accounting is simply unavailable when ReturnSuccesses is false). go-cover
// attributes the `if` line as a block with no statements, so it can never show
// as covered. It is a documentation placeholder, not a real branch, and is left
// uncovered by design.

// silence unused imports if the suite is trimmed on a subset build.
var _ = time.Second
