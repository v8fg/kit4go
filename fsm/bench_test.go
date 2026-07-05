package fsm

import "testing"

func BenchmarkSend(b *testing.B) {
	m, _ := New("s0",
		Rule{From: "s0", Event: "e", To: "s1"},
		Rule{From: "s1", Event: "e", To: "s0"},
	)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = m.Send("e", nil)
	}
}

func BenchmarkCan(b *testing.B) {
	m, _ := New("s0", Rule{From: "s0", Event: "e", To: "s1"})
	for i := 0; i < b.N; i++ {
		_ = m.Can("e", nil)
	}
}
