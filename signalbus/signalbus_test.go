package signalbus

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

// orderSink records handler invocations as "id:arg" so tests can assert
// registration-order dispatch.
type orderSink struct {
	mu   sync.Mutex
	got  []string
	hook func(arg string) // optional per-record side effect (e.g. re-entrant Send)
}

func (s *orderSink) record(id, arg string) {
	s.mu.Lock()
	s.got = append(s.got, id+":"+arg)
	hook := s.hook
	s.mu.Unlock()
	if hook != nil {
		hook(arg)
	}
}

func (s *orderSink) snapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.got))
	copy(out, s.got)
	return out
}

func makeHandler(id string, sink *orderSink) Handler {
	return func(args ...any) {
		arg := ""
		if len(args) > 0 {
			if s, ok := args[0].(string); ok {
				arg = s
			}
		}
		sink.record(id, arg)
	}
}

// TestConnectAndSendOrder verifies Connect subscribes and Send invokes handlers
// in registration order, with the args passed through.
func TestConnectAndSendOrder(t *testing.T) {
	b := New()
	sink := &orderSink{}

	b.Connect("evt", makeHandler("a", sink))
	b.Connect("evt", makeHandler("b", sink))
	b.Connect("evt", makeHandler("c", sink))

	if n := b.Len("evt"); n != 3 {
		t.Fatalf("Len = %d, want 3", n)
	}

	b.Send("evt", "x")

	want := []string{"a:x", "b:x", "c:x"}
	if got := sink.snapshot(); !equal(got, want) {
		t.Fatalf("dispatch order = %v, want %v", got, want)
	}
}

// TestSendNoSubscribers verifies a Send on an unknown signal is a no-op.
func TestSendNoSubscribers(t *testing.T) {
	b := New()
	// Must not panic and must return.
	b.Send("nobody", "a", "b", 1, 2)
	if n := b.Len("nobody"); n != 0 {
		t.Fatalf("Len = %d, want 0", n)
	}
}

// TestSendPassesAllArgs verifies the full variadic arg list reaches the handler.
func TestSendPassesAllArgs(t *testing.T) {
	b := New()
	var seen []any
	b.Connect("evt", func(args ...any) {
		seen = args
	})
	b.Send("evt", "s", 42, true)
	if len(seen) != 3 {
		t.Fatalf("got %d args, want 3", len(seen))
	}
	if s, _ := seen[0].(string); s != "s" {
		t.Errorf("arg0 = %v, want s", seen[0])
	}
	if n, _ := seen[1].(int); n != 42 {
		t.Errorf("arg1 = %v, want 42", seen[1])
	}
	if ok, _ := seen[2].(bool); !ok {
		t.Errorf("arg2 = %v, want true", seen[2])
	}
}

// TestDisconnectFunc verifies the returned disconnect func removes exactly one
// handler and is idempotent.
func TestDisconnectFunc(t *testing.T) {
	b := New()
	sink := &orderSink{}

	disconnectA := b.Connect("evt", makeHandler("a", sink))
	b.Connect("evt", makeHandler("b", sink))
	b.Connect("evt", makeHandler("c", sink))

	if n := b.Len("evt"); n != 3 {
		t.Fatalf("Len = %d, want 3 before disconnect", n)
	}

	disconnectA() // remove exactly "a"
	if n := b.Len("evt"); n != 2 {
		t.Fatalf("Len = %d, want 2 after disconnect", n)
	}

	b.Send("evt", "y")
	want := []string{"b:y", "c:y"}
	if got := sink.snapshot(); !equal(got, want) {
		t.Fatalf("after disconnect = %v, want %v", got, want)
	}

	// Idempotent: second call is a no-op.
	disconnectA()
	if n := b.Len("evt"); n != 2 {
		t.Fatalf("Len = %d, want 2 after idempotent disconnect", n)
	}
}

// TestDisconnectFuncIsolated verifies two handlers with the same func value can
// be removed independently (entry ids, not func identity, drive disconnect).
func TestDisconnectFuncIsolated(t *testing.T) {
	b := New()
	sink := &orderSink{}

	// Same underlying record sink but different id labels, so disconnect targets
	// the right subscription even though the closures are structurally similar.
	d1 := b.Connect("evt", makeHandler("h1", sink))
	d2 := b.Connect("evt", makeHandler("h2", sink))

	d1()
	if n := b.Len("evt"); n != 1 {
		t.Fatalf("Len = %d, want 1 after d1", n)
	}
	d2()
	if n := b.Len("evt"); n != 0 {
		t.Fatalf("Len = %d, want 0 after d2", n)
	}
}

// TestDisconnectFuncNeverCalled verifies the disconnect func may be discarded.
func TestDisconnectFuncNeverCalled(t *testing.T) {
	b := New()
	_ = b.Connect("evt", makeHandler("h", &orderSink{})) // discard disconnect
	if n := b.Len("evt"); n != 1 {
		t.Fatalf("Len = %d, want 1", n)
	}
}

// TestConnectNilHandler verifies a nil handler is ignored and its disconnect is
// a safe no-op (covers the nil-guard branch).
func TestConnectNilHandler(t *testing.T) {
	b := New()
	disconnect := b.Connect("evt", nil)
	if n := b.Len("evt"); n != 0 {
		t.Fatalf("Len = %d, want 0 for nil handler", n)
	}
	disconnect() // must not panic
	b.Send("evt", "x")
}

// TestDisconnectAll verifies Disconnect(name) removes every handler for name.
func TestDisconnectAll(t *testing.T) {
	b := New()
	sink := &orderSink{}

	b.Connect("evt", makeHandler("a", sink))
	b.Connect("evt", makeHandler("b", sink))
	b.Connect("other", makeHandler("c", sink))

	if n := b.Len("evt"); n != 2 {
		t.Fatalf("Len(evt) = %d, want 2", n)
	}

	b.Disconnect("evt")
	if n := b.Len("evt"); n != 0 {
		t.Fatalf("Len(evt) = %d, want 0 after Disconnect", n)
	}
	// "other" untouched.
	if n := b.Len("other"); n != 1 {
		t.Fatalf("Len(other) = %d, want 1 (Disconnect must be name-scoped)", n)
	}

	// Disconnect on an already-empty name is a no-op (covers the branch).
	b.Disconnect("evt")
	b.Disconnect("never-existed")

	b.Send("evt", "z") // no handlers — must not panic
	if got := sink.snapshot(); len(got) != 0 {
		t.Fatalf("expected no dispatch, got %v", got)
	}
}

// TestLen verifies Len reflects connect/disconnect churn for multiple names.
func TestLen(t *testing.T) {
	b := New()
	if n := b.Len("x"); n != 0 {
		t.Fatalf("Len = %d, want 0", n)
	}

	d1 := b.Connect("x", makeHandler("a", &orderSink{}))
	b.Connect("x", makeHandler("b", &orderSink{}))
	b.Connect("y", makeHandler("c", &orderSink{}))
	if n := b.Len("x"); n != 2 {
		t.Fatalf("Len(x) = %d, want 2", n)
	}
	if n := b.Len("y"); n != 1 {
		t.Fatalf("Len(y) = %d, want 1", n)
	}

	d1()
	if n := b.Len("x"); n != 1 {
		t.Fatalf("Len(x) = %d, want 1", n)
	}
}

// TestReentrantSendDuringHandler verifies a handler that itself calls Send on
// the same Bus does not deadlock (dispatch happens outside the lock), and that
// the inner Send sees only the handlers present when IT runs.
func TestReentrantSendDuringHandler(t *testing.T) {
	b := New()
	sink := &orderSink{}

	// First handler on "outer" triggers a Send on "inner".
	sink.hook = func(_ string) {
		// Only the "outer" handlers record into the shared sink; guard against
		// recursion by clearing the hook before re-entering.
		sink.mu.Lock()
		h := sink.hook
		sink.hook = nil
		sink.mu.Unlock()
		if h != nil {
			b.Send("inner", "from-outer")
		}
	}

	b.Connect("outer", makeHandler("outer-a", sink))
	b.Connect("inner", makeHandler("inner-a", sink))

	b.Send("outer", "go")

	// outer-a runs, then (re-entrantly) inner-a runs.
	want := []string{"outer-a:go", "inner-a:from-outer"}
	if got := sink.snapshot(); !equal(got, want) {
		t.Fatalf("re-entrant dispatch = %v, want %v", got, want)
	}
}

// TestReentrantConnectDuringHandler verifies a handler can Connect a new handler
// mid-dispatch without deadlock; the new handler is not visible to the in-flight
// Send but is visible to a later Send.
func TestReentrantConnectDuringHandler(t *testing.T) {
	b := New()
	sink := &orderSink{}
	var added atomic.Bool

	b.Connect("evt", func(args ...any) {
		makeHandler("first", sink)(args...)
		if !added.Swap(true) {
			b.Connect("evt", makeHandler("second", sink))
		}
	})

	b.Send("evt", "1")
	want := []string{"first:1"} // "second" was added AFTER the snapshot
	if got := sink.snapshot(); !equal(got, want) {
		t.Fatalf("in-flight Send = %v, want %v (new handler must not join mid-dispatch)", got, want)
	}

	b.Send("evt", "2")
	want = []string{"first:1", "first:2", "second:2"}
	if got := sink.snapshot(); !equal(got, want) {
		t.Fatalf("post-connect Send = %v, want %v", got, want)
	}
}

// TestReentrantDisconnectDuringHandler verifies a handler can disconnect itself
// (or a sibling) mid-dispatch without deadlock; the in-flight Send still
// completes its snapshot.
func TestReentrantDisconnectDuringHandler(t *testing.T) {
	b := New()
	sink := &orderSink{}

	var discB func()
	b.Connect("evt", makeHandler("a", sink))
	discB = b.Connect("evt", func(args ...any) {
		makeHandler("b", sink)(args...)
		discB() // remove self mid-dispatch
	})
	b.Connect("evt", makeHandler("c", sink))

	b.Send("evt", "x")
	// All three ran (snapshot taken before disconnect); b removed itself.
	want := []string{"a:x", "b:x", "c:x"}
	if got := sink.snapshot(); !equal(got, want) {
		t.Fatalf("in-flight dispatch = %v, want %v", got, want)
	}
	if n := b.Len("evt"); n != 2 {
		t.Fatalf("Len after self-disconnect = %d, want 2", n)
	}

	sink.got = nil
	b.Send("evt", "y")
	want = []string{"a:y", "c:y"}
	if got := sink.snapshot(); !equal(got, want) {
		t.Fatalf("post-disconnect Send = %v, want %v", got, want)
	}
}

// TestPanicInHandlerDoesNotAbort verifies a panicking handler is recovered and
// the remaining handlers still run.
func TestPanicInHandlerDoesNotAbort(t *testing.T) {
	b := New()
	sink := &orderSink{}

	b.Connect("evt", makeHandler("before", sink))
	b.Connect("evt", func(_ ...any) {
		panic(errors.New("boom"))
	})
	b.Connect("evt", makeHandler("after", sink))

	b.Send("evt", "z")

	want := []string{"before:z", "after:z"}
	if got := sink.snapshot(); !equal(got, want) {
		t.Fatalf("dispatch after panic = %v, want %v", got, want)
	}
	if r := b.Recovered(); r != 1 {
		t.Fatalf("Recovered = %d, want 1", r)
	}
}

// TestPanicHook verifies the SetPanicHook fires with name + handler id + value.
func TestPanicHook(t *testing.T) {
	b := New()
	sink := &orderSink{}

	var hookMu sync.Mutex
	var gotName string
	var gotValue any
	b.Connect("evt", makeHandler("safe", sink))
	b.Connect("evt", func(_ ...any) { panic("kaboom") })

	// Capture the handler id by recording it indirectly: SetPanicHook receives
	// the id, so just observe name+value and that an id (>0) was passed.
	var gotID uint64
	b.SetPanicHook(func(name string, handlerID uint64, r any) {
		hookMu.Lock()
		defer hookMu.Unlock()
		gotName, gotID, gotValue = name, handlerID, r
	})

	b.Send("evt", "x")

	if gotName != "evt" {
		t.Errorf("hook name = %q, want evt", gotName)
	}
	if gotValue != "kaboom" {
		t.Errorf("hook value = %v, want kaboom", gotValue)
	}
	if gotID == 0 {
		t.Error("hook handlerID = 0, want non-zero id")
	}
	// Sanity: the id falls in the [1,2] range (safe=1, panicker=2).
	if gotID < 1 || gotID > 2 {
		t.Errorf("hook handlerID = %d, want 1 or 2", gotID)
	}
}

// TestRecoveredCountsAcrossSignals verifies the counter is bus-wide.
func TestRecoveredCountsAcrossSignals(t *testing.T) {
	b := New()
	b.Connect("a", func(_ ...any) { panic(1) })
	b.Connect("b", func(_ ...any) { panic(2) })
	b.Send("a")
	b.Send("b")
	if r := b.Recovered(); r != 2 {
		t.Fatalf("Recovered = %d, want 2", r)
	}
}

// TestConcurrent verifies -race cleanliness under concurrent Connect/Send/Len.
// It does not assert specific output; its job is to surface data races.
func TestConcurrent(t *testing.T) {
	b := New()
	var wg sync.WaitGroup
	const workers = 8

	// Publishers and subscribers hammer the same signal concurrently.
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				b.Send("evt", i)
			}
		}()
	}
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				disc := b.Connect("evt", func(args ...any) {})
				_ = b.Len("evt")
				if i%7 == 0 {
					disc()
				}
			}
		}(w)
	}
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				b.Disconnect("evt")
			}
		}()
	}

	wg.Wait()
	// Final state is well-defined only in that the Bus is not corrupted: a Send
	// after Wait must not panic.
	b.Send("evt", "done")
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
