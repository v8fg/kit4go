package featureflag

import "testing"

func BenchmarkEnabled(b *testing.B) {
	f := New(WithEnabled(true), WithPercentage(50))
	b.ReportAllocs()
	for b.Loop() {
		_ = f.Enabled("user-42")
	}
}

func BenchmarkEnabled_Allowlisted(b *testing.B) {
	f := New(WithEnabled(true), WithAllowlist("vip"))
	for b.Loop() {
		_ = f.Enabled("vip")
	}
}
