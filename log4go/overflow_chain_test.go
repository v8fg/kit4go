package log4go

import (
	"testing"

	"github.com/IBM/sarama"
)

// Test_ChainedSpiller_RingThenFile verifies ring fills first, then overflows
// to file without dropping, and Drain recovers from both levels.
func Test_ChainedSpiller_RingThenFile(t *testing.T) {
	dir := t.TempDir()
	ring := NewRingSpiller[*sarama.ProducerMessage](2) // ring cap 2
	file, err := NewFileSpiller[*sarama.ProducerMessage](dir, 1<<20, ProducerMsgCodec)
	if err != nil {
		t.Fatal(err)
	}
	c := NewChainedSpiller[*sarama.ProducerMessage](ring, file)
	defer c.Close()

	// push 5: ring holds 2 (a,b), file holds c,d,e — no drop
	for _, v := range []string{"a", "b", "c", "d", "e"} {
		if !c.Push(spillerMsg("t", v)) {
			t.Fatalf("push %v rejected", v)
		}
	}
	if c.Len() != 5 {
		t.Fatalf("Len=%d want 5 (ring+file)", c.Len())
	}
	out := c.Drain()
	if len(out) != 5 {
		t.Fatalf("drain=%d want 5", len(out))
	}
}

// Test_ChainedSpiller_FileCapDrop verifies that once ring+file are full, Push
// returns false (drop) — total space is bounded.
func Test_ChainedSpiller_FileCapDrop(t *testing.T) {
	dir := t.TempDir()
	ring := NewRingSpiller[*sarama.ProducerMessage](2)
	file, _ := NewFileSpiller[*sarama.ProducerMessage](dir, 80, ProducerMsgCodec) // tiny file cap
	c := NewChainedSpiller[*sarama.ProducerMessage](ring, file)
	defer c.Close()

	accepted := 0
	for i := 0; i < 100; i++ {
		if c.Push(spillerMsg("t", "0123456789")) {
			accepted++
		}
	}
	if accepted >= 100 {
		t.Fatal("should hit file cap and reject some pushes (bounded space)")
	}
	if accepted < 3 {
		t.Fatalf("accepted=%d, too few", accepted)
	}
}

// Test_FileSpiller_RecoverAcrossInstances simulates an interrupted process:
// instance A writes spill and closes WITHOUT draining; instance B (same dir)
// drains and recovers the persisted records (resume from interruption).
func Test_FileSpiller_RecoverAcrossInstances(t *testing.T) {
	dir := t.TempDir()

	// instance A: push, then close without drain (persist spill.log on disk)
	a, err := NewFileSpiller[*sarama.ProducerMessage](dir, 1<<20, ProducerMsgCodec)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"x", "y", "z"} {
		if !a.Push(spillerMsg("t", v)) {
			t.Fatalf("push %v failed", v)
		}
	}
	if err := a.Close(); err != nil { // flush to disk, leave spill.log behind
		t.Fatal(err)
	}

	// instance B (same dir, e.g. after restart): Drain recovers persisted records
	b, err := NewFileSpiller[*sarama.ProducerMessage](dir, 1<<20, ProducerMsgCodec)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	out := b.Drain()
	if len(out) != 3 {
		t.Fatalf("recovered=%d want 3 (interrupted records)", len(out))
	}
}

// Test_RecordCodec_RoundTrip verifies the *Record codec used by FileWriter spill.
func Test_RecordCodec_RoundTrip(t *testing.T) {
	r := &Record{level: ERROR, time: "2026-06-25 00:00:00", file: "f.go:9", msg: "boom"}
	b, err := RecordCodec.Encode(r)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := RecordCodec.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if r2.level != r.level || r2.msg != r.msg || r2.file != r.file || r2.time != r.time {
		t.Errorf("round-trip mismatch: %+v vs %+v", r, r2)
	}
}

// Test_ProducerMsgCodec_RoundTrip verifies the *sarama.ProducerMessage codec.
func Test_ProducerMsgCodec_RoundTrip(t *testing.T) {
	m := spillerMsg("topic-x", "payload")
	b, err := ProducerMsgCodec.Encode(m)
	if err != nil {
		t.Fatal(err)
	}
	m2, err := ProducerMsgCodec.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if m2.Topic != m.Topic {
		t.Errorf("topic %q vs %q", m2.Topic, m.Topic)
	}
	val, _ := m2.Value.Encode()
	if string(val) != "payload" {
		t.Errorf("value %q", val)
	}
}
