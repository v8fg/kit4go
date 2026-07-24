package hotkey_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/v8fg/kit4go/hotkey"
)

// BenchmarkTouchSameKey measures the hot path: repeated Touch on one key (the
// common "record this request's key" call per request).
func BenchmarkTouchSameKey(b *testing.B) {
	d := hotkey.New(time.Hour, 100)
	b.ReportAllocs()
	for b.Loop() {
		d.Touch("hot-key")
	}
}

// BenchmarkTouchManyKeys rotates across many distinct keys — exercises the
// per-key hit buffer allocation + eviction path.
func BenchmarkTouchManyKeys(b *testing.B) {
	d := hotkey.New(time.Hour, 100)
	keys := make([]string, 10_000)
	for i := range keys {
		keys[i] = "k" + strconv.Itoa(i)
	}
	b.ReportAllocs()
	b.ResetTimer()
	i := 0
	for b.Loop() {
		d.Touch(keys[i%len(keys)])
		i++
	}
}
