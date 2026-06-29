package kafka

import (
	"testing"
	"time"
)

// TestOptions_AllWithSetters exercises every With* functional option (many were
// 0% covered — the setters are trivial but untested). Applies them all and
// checks each lands on the right Options field.
func TestOptions_AllWithSetters(t *testing.T) {
	o := applyOptions([]Option{
		WithName("n"),
		WithProducerTimeout(5 * time.Second),
		WithChannelBufferSize(64),
		WithProducerLinger(2 * time.Millisecond),
		WithAcks(AcksAll),
		WithMaxBufferedRecords(500),
		WithBatchMaxBytes(4096),
		WithSnapshotHistory(30),
		WithConsumerOffsetInitial(OffsetOldest),
		WithVersion("3.5.0"),
		WithTopic("t"),
		WithGroupID("g"),
		WithPartition(3),
		WithOffset(OffsetNewest),
		WithReturnSuccesses(false),
		WithRetryMax(7),
		WithCodec(CodecRaw{}),
		WithDeliveryMode("channel"),
	})
	checks := []struct {
		name string
		ok   bool
	}{
		{"Name", o.Name == "n"},
		{"ProducerTimeout", o.ProducerTimeout == 5*time.Second},
		{"ChannelBufferSize", o.ChannelBufferSize == 64},
		{"ProducerLinger", o.ProducerLinger == 2*time.Millisecond},
		{"Acks", o.Acks == AcksAll},
		{"MaxBufferedRecords", o.MaxBufferedRecords == 500},
		{"BatchMaxBytes", o.BatchMaxBytes == 4096},
		{"SnapshotHistory", o.SnapshotHistory == 30},
		{"ConsumerOffsetInitial", o.ConsumerOffsetInitial == OffsetOldest},
		{"Version", o.Version == "3.5.0"},
		{"Topic", o.Topic == "t"},
		{"GroupID", o.GroupID == "g"},
		{"Partition", o.Partition == 3},
		{"Offset", o.Offset == OffsetNewest},
		{"ReturnSuccesses", o.ReturnSuccesses == false},
		{"RetryMax", o.RetryMax == 7},
		{"Codec", o.Codec != nil},
		{"DeliveryMode", o.DeliveryMode == "channel"},
	}
	for _, c := range checks {
		if !c.ok {
			t.Errorf("option %s not applied correctly", c.name)
		}
	}
}

// TestComputeHelpers covers the clamp branches (success+failed > enqueued → 0).
func TestComputeHelpers(t *testing.T) {
	// normal
	if got := ComputeInFlight(100, 60, 30); got != 10 {
		t.Errorf("ComputeInFlight(100,60,30)=%d want 10", got)
	}
	// clamp: success+failed > enqueued
	if got := ComputeInFlight(50, 60, 0); got != 0 {
		t.Errorf("ComputeInFlight(50,60,0)=%d want 0 (clamped)", got)
	}
	// exact: success+failed == enqueued
	if got := ComputeInFlight(100, 60, 40); got != 0 {
		t.Errorf("ComputeInFlight(100,60,40)=%d want 0", got)
	}
	// BufferedBytes normal
	if got := ComputeBufferedBytes(1000, 600, 300); got != 100 {
		t.Errorf("ComputeBufferedBytes(1000,600,300)=%d want 100", got)
	}
	// clamp
	if got := ComputeBufferedBytes(500, 600, 0); got != 0 {
		t.Errorf("ComputeBufferedBytes(500,600,0)=%d want 0 (clamped)", got)
	}
}

// TestNameOr covers the fallback branch.
func TestNameOr(t *testing.T) {
	if got := nameOr("a", "b"); got != "a" {
		t.Errorf("nameOr(a,b)=%q want a", got)
	}
	if got := nameOr("", "b"); got != "b" {
		t.Errorf("nameOr('',b)=%q want b", got)
	}
}

// TestWithSnapshotHistoryClamp covers the negative clamp.
func TestWithSnapshotHistoryClamp(t *testing.T) {
	o := applyOptions([]Option{WithSnapshotHistory(-5)})
	if o.SnapshotHistory != 0 {
		t.Errorf("WithSnapshotHistory(-5)=%d want 0 (clamped)", o.SnapshotHistory)
	}
}
