package log4go

import (
	"testing"

	"github.com/v8fg/kit4go/kafka"
)

func spillerMsg(topic, val string) kafka.Message {
	return kafka.Message{Topic: topic, Value: []byte(val)}
}

// Test_RingSpiller_PushDrain verifies FIFO ordering, overwrite-oldest when full,
// and that Drain clears the ring.
func Test_RingSpiller_PushDrain(t *testing.T) {
	r := NewRingSpiller[kafka.Message](3)
	for _, v := range []string{"a", "b", "c", "d"} { // "d" overwrites oldest "a"
		r.Push(spillerMsg("t", v))
	}
	if r.Len() != 3 {
		t.Fatalf("Len=%d want 3", r.Len())
	}
	out := r.Drain()
	if len(out) != 3 {
		t.Fatalf("drain len=%d want 3", len(out))
	}
	// oldest two survivors are b,c,d in FIFO order
	want := []string{"b", "c", "d"}
	for i, w := range want {
		got := out[i].Value
		if string(got) != w {
			t.Errorf("out[%d]=%q want %q", i, got, w)
		}
	}
	if r.Len() != 0 || r.Drain() != nil {
		t.Fatal("ring not cleared after drain")
	}
}

// Test_RingSpiller_BoundedMemory verifies the ring never grows beyond capacity.
func Test_RingSpiller_BoundedMemory(t *testing.T) {
	r := NewRingSpiller[kafka.Message](16)
	for i := 0; i < 100000; i++ {
		r.Push(spillerMsg("t", "x"))
	}
	if r.Len() != 16 {
		t.Fatalf("ring grew unbounded: Len=%d want 16", r.Len())
	}
}

// Test_FileSpiller_PushDrain verifies disk persistence + recovery.
func Test_FileSpiller_PushDrain(t *testing.T) {
	dir := t.TempDir()
	f, err := NewFileSpiller[kafka.Message](dir, 1<<20, ProducerMsgCodec)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for _, v := range []string{"x", "y", "z"} {
		if !f.Push(spillerMsg("t", v)) {
			t.Fatalf("push %v failed", v)
		}
	}
	if f.Len() != 3 {
		t.Fatalf("Len=%d want 3", f.Len())
	}
	out := f.Drain()
	if len(out) != 3 {
		t.Fatalf("drain=%d want 3", len(out))
	}
	want := []string{"x", "y", "z"}
	for i, w := range want {
		got := out[i].Value
		if string(got) != w {
			t.Errorf("out[%d]=%q want %q", i, got, w)
		}
	}
	if f.Len() != 0 {
		t.Fatalf("after drain Len=%d want 0", f.Len())
	}
	// file spiller must keep accepting after a drain
	if !f.Push(spillerMsg("t", "again")) {
		t.Fatal("push after drain failed")
	}
}

// Test_FileSpiller_MaxBytes verifies the byte cap stops Push (no unbounded disk).
func Test_FileSpiller_MaxBytes(t *testing.T) {
	dir := t.TempDir()
	f, err := NewFileSpiller[kafka.Message](dir, 50, ProducerMsgCodec) // tiny cap
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	pushed := 0
	for i := 0; i < 100; i++ {
		if f.Push(spillerMsg("t", "0123456789")) {
			pushed++
		}
	}
	if pushed == 100 {
		t.Fatal("should hit maxBytes cap and reject some pushes")
	}
	if pushed == 0 {
		t.Fatal("should accept at least one push")
	}
}

func Test_OverflowStats(t *testing.T) {
	var s OverflowStats
	s.IncDropped()
	s.IncDropped()
	s.IncSpilled()
	if s.Dropped() != 2 || s.Spilled() != 1 {
		t.Fatalf("stats dropped=%d spilled=%d", s.Dropped(), s.Spilled())
	}
}

func Test_ParseOverflowPolicy(t *testing.T) {
	cases := map[string]OverflowPolicy{
		"":      OverflowDrop,
		"drop":  OverflowDrop,
		"block": OverflowBlock,
		"spill": OverflowSpill,
		"xx":    OverflowDrop,
	}
	for in, want := range cases {
		if got := ParseOverflowPolicy(in); got != want {
			t.Errorf("ParseOverflowPolicy(%q)=%v want %v", in, got, want)
		}
	}
}

// Benchmark_RingSpiller_Push measures spill-store throughput.
func Benchmark_RingSpiller_Push(b *testing.B) {
	r := NewRingSpiller[kafka.Message](1024)
	m := spillerMsg("t", "x")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Push(m)
	}
}

// Benchmark_FileSpiller_Push measures disk spill throughput.
func Benchmark_FileSpiller_Push(b *testing.B) {
	f, err := NewFileSpiller[kafka.Message](b.TempDir(), 1<<30, ProducerMsgCodec)
	if err != nil {
		b.Fatal(err)
	}
	defer f.Close()
	m := spillerMsg("t", "0123456789")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.Push(m)
	}
}
