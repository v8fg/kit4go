package log4go

import (
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Test_CallerCache_Hit verifies the PC->file:line memoization: the second call
// at the same site returns the cached string (cache size grows by one, not two),
// and the value matches a fresh runtime resolution.
func Test_CallerCache_Hit(t *testing.T) {
	pcs := make([]uintptr, 1)
	n := runtime.Callers(1, pcs)
	if n < 1 {
		t.Fatal("Callers failed")
	}
	pc := pcs[0]

	before := len(callerCache)
	s1 := callerFileLine(pc, false, false)
	after1 := len(callerCache)
	s2 := callerFileLine(pc, false, false) // should hit cache

	if s1 != s2 {
		t.Errorf("cache inconsistent: %q vs %q", s1, s2)
	}
	// callerCache is a process-wide shared map that the background logger (and
	// other tests) insert into concurrently, so map-size deltas are inherently
	// racy. Verify presence of THIS key under the lock instead. `before`/`after1`
	// are kept only as a best-effort signal in the failure message.
	callerCacheMu.RLock()
	_, cached := callerCache[callerKey{pc: pc, fullPath: false, withFunc: false}]
	callerCacheMu.RUnlock()
	if !cached {
		t.Errorf("first call did not cache the caller key (size %d -> %d)", before, after1)
	}

	// value must match a fresh runtime resolution (base.go:line)
	fn := runtime.FuncForPC(pc)
	file, line := fn.FileLine(pc)
	if i := strings.LastIndexByte(file, '/'); i >= 0 {
		file = file[i+1:]
	}
	want := file + ":" + strconv.Itoa(line)
	if s1 != want {
		t.Errorf("callerFileLine=%q want %q", s1, want)
	}
}

// Test_CallerCache_Variants verifies fullPath/withFunc produce distinct cached
// strings (the key includes them so configs don't collide).
func Test_CallerCache_Variants(t *testing.T) {
	pcs := make([]uintptr, 1)
	runtime.Callers(1, pcs)
	pc := pcs[0]

	base := callerFileLine(pc, false, false)
	full := callerFileLine(pc, true, false)
	withFn := callerFileLine(pc, false, true)

	if base == full {
		t.Error("base and fullPath should differ")
	}
	if base == withFn {
		t.Error("base and withFunc should differ")
	}
	if !strings.Contains(withFn, " ") {
		t.Errorf("withFunc result %q should contain a func name", withFn)
	}
}

// Test_DeliverRecordToWriter_CallerFormat is an end-to-end check that the cached
// caller path still produces the canonical "<file:line>" in the record.
func Test_DeliverRecordToWriter_CallerFormat(t *testing.T) {
	records := make(chan *Record, 4)
	root := newLoggerWithRecords(records)
	defer root.Close()
	root.SetLevel(DEBUG)
	root.hasCaller.Store(true)

	cw := &captureWriter{}
	root.Register(cw)
	root.Info("hi") // exercises the caller cache path

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && cw.Len() == 0 {
		runtime.Gosched()
	}
	if cw.Len() == 0 {
		t.Fatal("record never reached writer")
	}
	cw.mu.Lock()
	r := cw.records[0]
	cw.mu.Unlock()
	if r.file == "" {
		t.Fatal("caller file empty (caller cache path broke)")
	}
	if !strings.Contains(r.file, "caller_cache_test.go") {
		t.Errorf("caller file=%q want this test file", r.file)
	}
}
