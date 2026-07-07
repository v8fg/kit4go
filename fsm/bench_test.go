package fsm

import "testing"

func BenchmarkSend(b *testing.B) {
	m, _ := New("s0",
		Rule{From: "s0", Event: "e", To: "s1"},
		Rule{From: "s1", Event: "e", To: "s0"},
	)
	b.ReportAllocs()
	for b.Loop() {
		_ = m.Send("e", nil)
	}
}

func BenchmarkCan(b *testing.B) {
	m, _ := New("s0", Rule{From: "s0", Event: "e", To: "s1"})
	for b.Loop() {
		_ = m.Can("e", nil)
	}
}
