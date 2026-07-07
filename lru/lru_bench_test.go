package lru

import "testing"

// BenchmarkGet measures a cache hit (promotes the entry, write lock).
func BenchmarkGet(b *testing.B) {
	c := New[string, int](WithMaxSize[string, int](1024))
	for i := 0; i < 1024; i++ {
		c.Set(key(i), i)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = c.Get("500")
	}
}

// BenchmarkGetParallel measures Get under contention.
func BenchmarkGetParallel(b *testing.B) {
	c := New[string, int](WithMaxSize[string, int](1024))
	for i := 0; i < 1024; i++ {
		c.Set(key(i), i)
	}
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			c.Get(key(i & 1023))
			i++
		}
	})
}

// BenchmarkPeek measures a non-promoting read.
func BenchmarkPeek(b *testing.B) {
	c := New[string, int](WithMaxSize[string, int](1024))
	for i := 0; i < 1024; i++ {
		c.Set(key(i), i)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = c.Peek("500")
	}
}

// BenchmarkSet measures an insert/refresh of a hot key.
func BenchmarkSet(b *testing.B) {
	c := New[string, int](WithMaxSize[string, int](1024))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Set("k", i)
	}
}

// BenchmarkSetEvict measures Set when the cache is full and each insert evicts
// the LRU entry (the steady-state eviction path).
func BenchmarkSetEvict(b *testing.B) {
	c := New[string, int](WithMaxSize[string, int](1024))
	for i := 0; i < 1024; i++ {
		c.Set(key(i), i)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Set(key(1024+i), i)
	}
}

func key(i int) string {
	const digits = "0123456789abcdefghijklmnopqrstuvwxyz"
	var buf [4]byte
	buf[0] = digits[(i>>12)&31]
	buf[1] = digits[(i>>8)&31]
	buf[2] = digits[(i>>4)&31]
	buf[3] = digits[i&31]
	return string(buf[:])
}
