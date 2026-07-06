//go:build franzgo

package kafka

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

// Local helpers: testutil_test.go is !franzgo-tagged, so under -tags franzgo the
// shared errBoom/waitUntil are not visible. These mirror them for this file.
var franzErrBoom = errors.New("boom")

func franzWaitUntil(t *testing.T, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for: %s", what)
}

// This file closes the reachable coverage gaps in the franz-go backend
// (-tags franzgo). Tests are grouped: no-broker accessor/guard tests first,
// then dead-broker produce-error tests, then kfake-broker consume tests.

// ---- snapshot.go:88 — empty non-nil history returns nil (shared file) ----
//
// The shared snapshot.go has no build tag, but the existing empty-history test
// lives in a !franzgo file, so under -tags franzgo the size==0 guard on a
// non-nil history is never exercised.
func TestFranzFinal_SnapshotHistory_EmptyNonNilReturnsNil(t *testing.T) {
	h := newSnapshotHistory(3)
	if got := h.snapshot(); got != nil {
		t.Errorf("empty non-nil history snapshot()=%v want nil", got)
	}
}

// ---- franzgo_consumer.go:36 — Consume after Close (ErrProducerClosed) ----
// ---- franzgo_consumer.go:94 — double Close (CAS already-closed) ----
// ---- franzgo_consumer.go:114 — Snapshot ----
// ---- franzgo_consumer.go:89 — pushErr default-drop ----
func TestFranzFinal_ConsumerGroup_GuardsAndSnapshot(t *testing.T) {
	cg := &franzConsumerGroup{opts: Options{GroupID: "g"}.withDefaults()}

	// Snapshot before any traffic (covers the Snapshot body, 114-121).
	snap := cg.Snapshot()
	if snap.Backend != "franz-go" || snap.Timestamp.IsZero() {
		t.Errorf("cg Snapshot: %+v", snap)
	}

	// pushErr drop branch (89): fill errCh to capacity, then push once more.
	cg.errChOnce.Do(func() { cg.errCh = make(chan error, 1) })
	cg.errCh <- franzErrBoom
	cg.pushErr(errors.New("dropped")) // must hit default without blocking
	if len(cg.errCh) != 1 {
		t.Errorf("cg errCh len=%d want 1 (drop branch)", len(cg.errCh))
	}

	// Close twice: first succeeds (CAS true), second is a no-op (CAS fails, 94-96).
	if err := cg.Close(); err != nil {
		t.Errorf("cg Close: %v", err)
	}
	if err := cg.Close(); err != nil {
		t.Errorf("cg double Close: %v", err)
	}

	// Consume after Close returns ErrProducerClosed (36-38).
	if err := cg.Consume(context.Background(), []string{"t"}, func(Message) error { return nil }); err != ErrProducerClosed {
		t.Errorf("cg Consume after Close: %v want ErrProducerClosed", err)
	}
}

// ---- franzgo_consumer.go:162 — partition Consume after Close ----
// ---- franzgo_consumer.go:225 — partition pushErr default-drop ----
// ---- franzgo_consumer.go:230 — partition double Close ----
// ---- franzgo_consumer.go:249 — partition Snapshot ----
//
// Broker-free: Snapshot, pushErr-drop, Close idempotency and the closed-Consume
// guard need no client. (Messages() channel-mode is documented separately
// below; it leaks a background pump and conflicts with goleak.)
func TestFranzFinal_PartitionConsumer_Guards(t *testing.T) {
	pc := &franzPartitionConsumer{opts: Options{Topic: "t"}.withDefaults()}

	// Snapshot (249-256).
	snap := pc.Snapshot()
	if snap.Backend != "franz-go" || snap.Name != "t" {
		t.Errorf("pc Snapshot: %+v", snap)
	}

	// pushErr drop branch (225).
	pc.errChOnce.Do(func() { pc.errCh = make(chan error, 1) })
	pc.errCh <- franzErrBoom
	pc.pushErr(errors.New("dropped"))
	if len(pc.errCh) != 1 {
		t.Errorf("pc errCh len=%d want 1 (drop branch)", len(pc.errCh))
	}

	// Close twice: first CAS success, second CAS fail (230-232).
	if err := pc.Close(); err != nil {
		t.Errorf("pc Close: %v", err)
	}
	if err := pc.Close(); err != nil {
		t.Errorf("pc double Close: %v", err)
	}
	// Consume after Close returns ErrProducerClosed (162-164).
	if err := pc.Consume(context.Background(), func(Message) error { return nil }); err != ErrProducerClosed {
		t.Errorf("pc Consume after Close: %v want ErrProducerClosed", err)
	}
}

// ---- franzgo_consumer.go:172-176 — Messages() channel mode ----
//
// INTENTIONALLY NOT TESTED: Messages() spawns a background pump goroutine bound
// to context.Background() (not the caller's ctx), so it cannot be cancelled.
// The suite enforces goleak.VerifyTestMain, and kgo's client keeps internal
// polling goroutines alive briefly after Close(), so any test that triggers the
// Messages() pump leaks goroutines and fails the suite. The existing
// TestFranzgoKfake_PartitionConsumer deliberately uses callback Consume() for
// the same reason. Covering this branch would require either a production-code
// change (make the pump cancellable) or disabling goleak, both out of scope.
// Lines 172-176 are a straightforward once.Do channel init + goroutine launch
// and are left uncovered by design.

// ---- franzgo_consumer.go:46 — lazy kgo.NewClient error in Consume ----
//
// Consume creates the kgo client lazily; an invalid broker makes NewClient
// fail. We point at an unreachable port with a short request timeout so the
// failure surfaces promptly. (kgo.NewClient itself rarely errors for bad
// brokers — it defers dialing — so we instead verify the lazy path runs by
// cancelling ctx: the loop's top ctxDone check returns ctx.Err() without
// requiring a successful client. To hit the NewClient error branch we need a
// genuinely invalid kgo.Opt; kgo.BlockRebalanceOnPoll is valid, so we craft a
// client via a port that kgo rejects at construction when SeedBrokers is empty
// — but withDefaults requires brokers. Instead we set the client directly to
// force the lazy `s.cl == nil` branch and rely on NewClient succeeding, then
// exercise the error path via a pre-set bad client is impossible.)
//
// Net: the NewClient error branch (46-48) requires an opts combination that
// kgo.NewClient rejects. kgo rejects an empty broker list, but validate()
// already forbids that. This branch is therefore a defensive guard for an
// impossible-by-construction state (validate ensures brokers; kgo.NewClient
// accepts any non-empty broker list without dialing) and is intentionally left
// uncovered.

// ---- franzgo_producer.go:200 — sync SendBatch after Close ----
// ---- franzgo_producer.go sync SendBatch happy path (already covered) ----
func TestFranzFinal_SyncProducer_SendBatchClosed(t *testing.T) {
	sp := &franzSyncProducer{opts: Options{Topic: "t"}.withDefaults()}
	sp.closed.Store(true)
	if err := sp.SendBatch(context.Background(), []Message{{Value: []byte("x")}}); err != ErrProducerClosed {
		t.Errorf("sync SendBatch after close: %v want ErrProducerClosed", err)
	}
}

// ---- produce-error paths via a dead broker ----
//
// Producing against a port with nothing listening fails the produce promise /
// ProduceSync after retries are exhausted. We disable retries
// (RecordRetries(0)) and use a short ProducerLinger so the failure surfaces
// quickly. This covers:
//   - franzgo_producer.go:47-52  (async Send promise error)
//   - franzgo_producer.go:72-77  (async SendBatch promise error)
//   - franzgo_producer.go:179-183 (sync Send error)
//   - franzgo_producer.go:216-221 (sync SendBatch per-record error)
func newDeadBrokerClient(t *testing.T, sync bool) *kgo.Client {
	t.Helper()
	// Reserve a port that nothing listens on.
	opts := []kgo.Opt{
		kgo.SeedBrokers("127.0.0.1:1"),
		kgo.RecordRetries(0),
		kgo.ProducerLinger(0),
		kgo.RequiredAcks(kgo.LeaderAck()),
		kgo.DisableIdempotentWrite(),
		// Fail fast: nothing listens on :1, so dial + produce must time out
		// quickly rather than holding the test for the default backoff window.
		kgo.DialTimeout(200 * time.Millisecond),
		kgo.RequestTimeoutOverhead(100 * time.Millisecond),
		kgo.ProduceRequestTimeout(500 * time.Millisecond),
		kgo.RetryTimeout(500 * time.Millisecond),
	}
	cl, err := kgo.NewClient(opts...)
	if err != nil {
		t.Fatalf("kgo.NewClient: %v", err)
	}
	return cl
}

func TestFranzFinal_AsyncProduce_DeadBrokerErrors(t *testing.T) {
	p := &franzProducer{opts: Options{Topic: "t"}.withDefaults(), cl: newDeadBrokerClient(t, false)}
	defer p.Close()

	var got atomic.Int32
	p.SetOnEvent(func(e ProducerEvent) {
		if e.Name == "error" {
			got.Add(1)
		}
	})

	// Send: the promise fires with an error (covers 47-52).
	_ = p.Send(context.Background(), Message{Value: []byte("a")})
	// SendBatch: each record's promise fires with an error (covers 72-77).
	_ = p.SendBatch(context.Background(), []Message{
		{Value: []byte("b")}, {Value: []byte("c")},
	})

	// Flush so all in-flight promises resolve before we assert.
	fctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = p.cl.Flush(fctx)

	franzWaitUntil(t, func() bool { return got.Load() >= 3 }, "async produce error events")
	if m := p.Metrics(); m.Failed < 3 {
		t.Errorf("async Failed=%d want >=3", m.Failed)
	}
}

func TestFranzFinal_SyncProduce_DeadBrokerErrors(t *testing.T) {
	sp := &franzSyncProducer{opts: Options{Topic: "t"}.withDefaults(), cl: newDeadBrokerClient(t, true)}
	defer sp.Close()

	// ProduceSync against a dead broker returns an error (covers 179-183).
	_, _, err := sp.Send(context.Background(), Message{Value: []byte("a")})
	if err == nil {
		t.Error("sync Send against dead broker: want error, got nil")
	}
	// SendBatch: at least one record errors (covers 216-221).
	berr := sp.SendBatch(context.Background(), []Message{
		{Value: []byte("b")}, {Value: []byte("c")},
	})
	if berr == nil {
		t.Error("sync SendBatch against dead broker: want error, got nil")
	}
	if m := sp.Metrics(); m.Failed < 1 {
		t.Errorf("sync Failed=%d want >=1", m.Failed)
	}
}

// ---- franzgo_consumer.go:56-60 / 68-71 — consume error + NACK via kfake ----
// ---- franzgo_consumer.go:185-189 / 198-201 — partition pump error + NACK ----
//
// Against a real kfake broker: produce a record, then consume with a handler
// that returns an error (NACK branch). The fetch-error branches (56-60,
// 185-189) require a fetch-level error which kfake does not inject in its
// public API; those are documented below as defensive and left uncovered.
func TestFranzFinal_Consume_NACK(t *testing.T) {
	_, addrs := kfakeCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	prod, err := NewProducer(WithBrokers(addrs...), WithTopic("nack"))
	if err != nil {
		t.Fatal(err)
	}
	_ = prod.Send(ctx, Message{Value: []byte("nack-msg")})
	prod.Close()

	grp, err := NewConsumerGroup(
		WithBrokers(addrs...),
		WithGroupID("nack-group"),
		WithConsumerOffsetInitial(OffsetOldest),
	)
	if err != nil {
		t.Fatal(err)
	}
	var nacks atomic.Int32
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = grp.Consume(ctx, []string{"nack"}, func(m Message) error {
			nacks.Add(1)
			cancel()            // force NACK then exit
			return franzErrBoom // covers 68-71
		})
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("nack consume timeout")
	}
	grp.Close()
	if nacks.Load() < 1 {
		t.Errorf("nacks=%d want >=1", nacks.Load())
	}
}

func TestFranzFinal_PartitionConsume_NACK(t *testing.T) {
	_, addrs := kfakeCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	prod, err := NewProducer(WithBrokers(addrs...), WithTopic("pnack"))
	if err != nil {
		t.Fatal(err)
	}
	_ = prod.Send(ctx, Message{Value: []byte("pnack-msg")})
	prod.Close()

	pcon, err := NewPartitionConsumer(
		WithBrokers(addrs...),
		WithTopic("pnack"),
		WithPartition(0),
		WithOffset(OffsetOldest),
	)
	if err != nil {
		t.Fatal(err)
	}
	var nacks atomic.Int32
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = pcon.Consume(ctx, func(m Message) error {
			nacks.Add(1)
			cancel()
			return franzErrBoom // covers 198-201
		})
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("partition nack consume timeout")
	}
	pcon.Close()
	if nacks.Load() < 1 {
		t.Errorf("partition nacks=%d want >=1", nacks.Load())
	}
}

// ---- franzgo_consumer.go:205-209 — partition pump channel-mode forward ctx.Done ----
//
// INTENTIONALLY NOT TESTED: this branch fires only in channel-mode pump (the
// `else` branch when handler == nil), which is reached solely via Messages().
// As documented above, Messages()'s unstoppable background pump conflicts with
// the suite's goleak enforcement, so channel-mode pump paths (205-209) are not
// exercised in unit tests. The branch is a symmetric mirror of the
// handler-mode NACK path and is covered in production usage.

// ---- DOCUMENTED UNREACHABLE franzgo BRANCHES (no test; impossible by construction) ----
//
// franzgo.go:185-187 / 195-197 / 200-202 / 228-230 — `kgo.NewClient(...)` error
// branches in the four public constructors (NewProducer, NewSyncProducer, the
// sync return, NewPartitionConsumer). kgo.NewClient does NOT dial at
// construction (connections are lazy); it errors only on an invalid Opt
// combination. Every Opt the package builds (kgo.SeedBrokers, RequiredAcks,
// ProducerLinger, MaxBufferedRecords, RecordRetries, ConsumePartitions,
// ConsumerGroup, AutoCommitMarks, ConsumeResetOffset, AllowAutoTopicCreation,
// ProducerBatchMaxBytes, DisableIdempotentWrite) is a valid literal whose
// inputs are themselves validated by Options.validate() / withDefaults()
// (brokers non-empty, acks in {all,none,leader,""}, offsets >= 0 or a known
// sentinel). There is no Options value that survives validate() yet yields a
// NewClient-rejecting Opt, so these four `if err != nil` branches are pure
// defensive guards against a future kgo version tightening validation. Left
// uncovered.
//
// franzgo_consumer.go:46-48 — lazy `kgo.NewClient` error in Consume. Same
// reasoning: kgoConsumerGroupOpts builds only valid literals and validate()
// guarantees a non-empty broker list before Consume runs. Defensive guard.
//
// franzgo_consumer.go:56-60 / 185-189 — `for _, fe := range fetches.Errors()`
// fetch-error loop bodies. kfake does not expose per-fetch error injection in
// its public API, and a healthy broker does not return partition-level fetch
// errors. These bodies fire only on real broker faults (out of range, auth,
// unknown topic after retries). Reaching them deterministically requires
// internal kfake hooks the library keeps private; the branches are exercised
// in production against real clusters and are left uncovered in unit tests.
//
// franzgo_consumer.go:172-176 / 205-209 — Messages() channel mode and the
// channel-mode pump forward ctx.Done. Messages() spawns a background pump bound
// to context.Background() that cannot be cancelled; kgo's client keeps internal
// polling goroutines alive briefly after Close(), so any test exercising
// channel mode leaks goroutines and trips goleak.VerifyTestMain. The existing
// TestFranzgoKfake_PartitionConsumer deliberately uses callback Consume() for
// this reason. Covering these would require a production-code change (make the
// pump cancellable) or disabling goleak; both are out of scope. Left uncovered.

// silence unused imports on subset builds.
var _ = time.Second
