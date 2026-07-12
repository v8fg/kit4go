//go:build franzgo

package kafka

import (
	"context"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kfake"
)

// kfakeCluster starts a 1-broker in-process Kafka cluster for franz-go unit
// tests. No docker needed — runs in CI. Returns the cluster + its broker addrs.
func kfakeCluster(t *testing.T) (*kfake.Cluster, []string) {
	t.Helper()
	cl, err := kfake.NewCluster(
		kfake.NumBrokers(1),
		kfake.AllowAutoTopicCreation(),
		kfake.DefaultNumPartitions(1),
	)
	if err != nil {
		t.Fatalf("kfake.NewCluster: %v", err)
	}
	t.Cleanup(func() { cl.Close() })
	return cl, cl.ListenAddrs()
}

// TestFranzgoKfake_AsyncProduce covers Send + SendBatch + Close (flushAndClose)
// + Metrics + Snapshot against a real in-process broker.
func TestFranzgoKfake_AsyncProduce(t *testing.T) {
	_, addrs := kfakeCluster(t)
	ctx := context.Background()

	prod, err := NewProducer(WithBrokers(addrs...), WithTopic("kp"))
	if err != nil {
		t.Fatal(err)
	}
	if err := prod.Send(ctx, Message{Value: []byte("a")}); err != nil {
		t.Fatal(err)
	}
	if err := prod.SendBatch(ctx, []Message{
		{Value: []byte("b")}, {Value: []byte("c")},
	}); err != nil {
		t.Fatal(err)
	}
	prod.Close() // flushAndClose — drains in-flight

	m := prod.Metrics()
	if m.Enqueued < 3 {
		t.Errorf("Enqueued=%d want >=3", m.Enqueued)
	}
	s := prod.Snapshot()
	if s.Backend != "franz-go" || s.Timestamp.IsZero() {
		t.Errorf("Snapshot: %+v", s)
	}
}

// TestFranzgoKfake_AsyncClosed covers the ErrProducerClosed error path.
func TestFranzgoKfake_AsyncClosed(t *testing.T) {
	_, addrs := kfakeCluster(t)
	prod, err := NewProducer(WithBrokers(addrs...), WithTopic("kc"))
	if err != nil {
		t.Fatal(err)
	}
	prod.Close()
	if err := prod.Send(context.Background(), Message{Value: []byte("x")}); err != ErrProducerClosed {
		t.Errorf("Send after Close: err=%v want ErrProducerClosed", err)
	}
}

// TestFranzgoKfake_SyncProduce covers SyncProducer Send + SendBatch + Close.
func TestFranzgoKfake_SyncProduce(t *testing.T) {
	_, addrs := kfakeCluster(t)
	ctx := context.Background()
	sp, err := NewSyncProducer(WithBrokers(addrs...), WithTopic("ks"))
	if err != nil {
		t.Fatal(err)
	}
	part, off, err := sp.Send(ctx, Message{Value: []byte("sync1")})
	if err != nil {
		t.Fatalf("sync Send: %v", err)
	}
	if part < 0 || off < 0 {
		t.Errorf("sync Send: partition=%d offset=%d", part, off)
	}
	if err := sp.SendBatch(ctx, []Message{
		{Value: []byte("s2")}, {Value: []byte("s3")},
	}); err != nil {
		t.Fatalf("sync SendBatch: %v", err)
	}
	m := sp.Metrics()
	if m.Enqueued < 3 {
		t.Errorf("sync Enqueued=%d want >=3", m.Enqueued)
	}
	_ = sp.Snapshot() // sync Snapshot has Timestamp
	sp.Close()
}

// TestFranzgoKfake_ProduceConsumeRoundTrip covers ConsumerGroup Consume + Close.
func TestFranzgoKfake_ProduceConsumeRoundTrip(t *testing.T) {
	_, addrs := kfakeCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	prod, err := NewProducer(WithBrokers(addrs...), WithTopic("rt"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		_ = prod.Send(ctx, Message{Value: []byte("rt-msg")})
	}
	prod.Close()

	grp, err := NewConsumerGroup(
		WithBrokers(addrs...),
		WithGroupID("rt-group"),
		WithConsumerOffsetInitial(OffsetOldest),
	)
	if err != nil {
		t.Fatal(err)
	}

	var got int
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = grp.Consume(ctx, []string{"rt"}, func(m Message) error {
			got++
			if got >= 5 {
				cancel()
			}
			return nil
		})
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("consume timeout")
	}
	grp.Close()
	if got < 5 {
		t.Errorf("consumed %d, want >=5", got)
	}
}

// TestFranzgoKfake_PartitionConsumer covers PartitionConsumer Consume + Close.
func TestFranzgoKfake_PartitionConsumer(t *testing.T) {
	_, addrs := kfakeCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	prod, err := NewProducer(WithBrokers(addrs...), WithTopic("pc"))
	if err != nil {
		t.Fatal(err)
	}
	_ = prod.Send(ctx, Message{Value: []byte("pc-msg")})
	prod.Close()

	pcon, err := NewPartitionConsumer(
		WithBrokers(addrs...),
		WithTopic("pc"),
		WithPartition(0),
		WithOffset(OffsetOldest),
	)
	if err != nil {
		t.Fatal(err)
	}

	var got int
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = pcon.Consume(ctx, func(m Message) error {
			got++
			cancel()
			return nil
		})
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("partition consume timeout")
	}
	pcon.Close()
	if got < 1 {
		t.Errorf("partition consumed %d, want >=1", got)
	}
}

// Regression: a caller ranging over Messages() (channel mode) must unblock when
// Close stops the pump. Before the fix msgCh was never closed.
func TestFranzgoKfake_PartitionConsumerChannelMode_RangeUnblocksOnClose(t *testing.T) {
	_, addrs := kfakeCluster(t)
	pcon, err := NewPartitionConsumer(
		WithBrokers(addrs...),
		WithTopic("pcc"),
		WithPartition(0),
		WithOffset(OffsetOldest),
		WithDeliveryMode("channel"),
	)
	if err != nil {
		t.Fatal(err)
	}
	ch := pcon.Messages()
	if ch == nil {
		t.Fatal("Messages() should be non-nil in channel mode")
	}
	rangeDone := make(chan struct{})
	go func() {
		for range ch { // must exit when Close closes ch
		}
		close(rangeDone)
	}()
	time.Sleep(100 * time.Millisecond) // let the ranger + pump park
	if err := pcon.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case <-rangeDone:
	case <-time.After(3 * time.Second):
		t.Fatal("range over Messages() did not unblock after Close (channel never closed)")
	}
}
