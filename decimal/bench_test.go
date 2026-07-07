package decimal

import "testing"

func BenchmarkAdd(b *testing.B) {
	a := MustParse("12.34", 2)
	c := MustParse("0.56", 2)
	b.ReportAllocs()
	for b.Loop() {
		_, _ = a.Add(c)
	}
}

func BenchmarkMulDecimal(b *testing.B) {
	a := MustParse("12.50", 2)
	c := MustParse("0.08", 2)
	b.ReportAllocs()
	for b.Loop() {
		_ = a.MulDecimal(c)
	}
}

func BenchmarkParse(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_, _ = Parse("12345.67", 2)
	}
}

func BenchmarkString(b *testing.B) {
	d := MustParse("12345.67", 2)
	for b.Loop() {
		_ = d.String()
	}
}
