package kafka

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/IBM/sarama"
)

// stubSession records MarkMessage calls; other ConsumerGroupSession methods are
// no-ops (only MarkMessage matters for the handler adapter).
type stubSession struct {
	marked atomic.Int32
}

func (s *stubSession) Claims() map[string][]int32                  { return nil }
func (s *stubSession) MemberID() string                            { return "" }
func (s *stubSession) GenerationID() int32                         { return 0 }
func (s *stubSession) MarkOffset(string, int32, int64, string)     {}
func (s *stubSession) ResetOffset(string, int32, int64, string)    {}
func (s *stubSession) MarkMessage(*sarama.ConsumerMessage, string) { s.marked.Add(1) }
func (s *stubSession) Commit()                                     {}
func (s *stubSession) Context() context.Context                    { return context.Background() }

// stubClaim yields a controlled stream of messages via Messages().
type stubClaim struct {
	msgs chan *sarama.ConsumerMessage
}

func (c *stubClaim) Topic() string                            { return "t" }
func (c *stubClaim) Partition() int32                         { return 0 }
func (c *stubClaim) InitialOffset() int64                     { return 0 }
func (c *stubClaim) HighWaterMarkOffset() int64               { return 0 }
func (c *stubClaim) Messages() <-chan *sarama.ConsumerMessage { return c.msgs }

// runHandler feeds msgs through the cgHandler adapter and waits for it to
// finish (claim.Messages() is closed).
func runHandler(t *testing.T, h sarama.ConsumerGroupHandler, msgs []*sarama.ConsumerMessage) (*stubSession, int32) {
	t.Helper()
	ch := make(chan *sarama.ConsumerMessage, len(msgs))
	for _, m := range msgs {
		ch <- m
	}
	close(ch)
	sess := &stubSession{}
	claim := &stubClaim{msgs: ch}
	if err := h.(*cgHandler).ConsumeClaim(sess, claim); err != nil {
		t.Fatalf("ConsumeClaim: %v", err)
	}
	return sess, sess.marked.Load()
}

func TestConsumerGroupHandler_ACK(t *testing.T) {
	parent := &saramaConsumerGroup{}
	h := &cgHandler{parent: parent, handler: func(Message) error { return nil }}
	msgs := []*sarama.ConsumerMessage{
		{Topic: "t", Value: []byte("a")},
		{Topic: "t", Value: []byte("b")},
		{Topic: "t", Value: []byte("c")},
	}
	sess, marked := runHandler(t, h, msgs)
	if marked != 3 {
		t.Errorf("MarkMessage called %d times want 3", marked)
	}
	if sess == nil {
		t.Fatal("nil session")
	}
	if got := parent.Metrics(); got.Received != 3 || got.Acked != 3 || got.Failed != 0 {
		t.Errorf("Metrics=%+v want received=3 acked=3 failed=0", got)
	}
}

func TestConsumerGroupHandler_NACK(t *testing.T) {
	parent := &saramaConsumerGroup{}
	h := &cgHandler{parent: parent, handler: func(Message) error { return errBoom }}
	msgs := []*sarama.ConsumerMessage{{Topic: "t", Value: []byte("x")}}
	_, marked := runHandler(t, h, msgs)
	if marked != 0 {
		t.Errorf("MarkMessage called %d times want 0 (NACK must not commit)", marked)
	}
	if got := parent.Metrics(); got.Received != 1 || got.Acked != 0 || got.Failed != 1 {
		t.Errorf("Metrics=%+v want received=1 acked=0 failed=1", got)
	}
}

func TestConsumerGroupHandler_Mixed(t *testing.T) {
	parent := &saramaConsumerGroup{}
	n := 0
	h := &cgHandler{parent: parent, handler: func(Message) error {
		n++
		if n == 2 {
			return errBoom // 2nd NACKs
		}
		return nil
	}}
	msgs := []*sarama.ConsumerMessage{{}, {}, {}}
	_, marked := runHandler(t, h, msgs)
	if marked != 2 { // 2 ACKed, 1 NACKed
		t.Errorf("MarkMessage=%d want 2", marked)
	}
	if got := parent.Metrics(); got.Received != 3 || got.Acked != 2 || got.Failed != 1 {
		t.Errorf("Metrics=%+v want received=3 acked=2 failed=1", got)
	}
}

func TestConsumerGroupHandler_SetupCleanup(t *testing.T) {
	h := &cgHandler{handler: func(Message) error { return nil }}
	if err := h.Setup(nil); err != nil {
		t.Errorf("Setup: %v", err)
	}
	if err := h.Cleanup(nil); err != nil {
		t.Errorf("Cleanup: %v", err)
	}
}

func TestConsumerGroup_Validate(t *testing.T) {
	if _, err := NewConsumerGroup(WithBrokers("x")); err == nil {
		t.Error("missing group_id should error")
	}
}
