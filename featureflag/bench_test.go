package featureflag

import "testing"

func BenchmarkEnabled(b *testing.B) {
	f := New(WithEnabled(true), WithPercentage(50))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = f.Enabled("user-42")
	}
}

func BenchmarkEnabled_Allowlisted(b *testing.B) {
	f := New(WithEnabled(true), WithAllowlist("vip"))
	for i := 0; i < b.N; i++ {
		_ = f.Enabled("vip")
	}
}
