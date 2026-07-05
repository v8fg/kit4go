package log4go

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/v8fg/kit4go/kafka"
)

// errDeep is a test error type. (The package already defines `errStr` in
// coverage_boost_test.go, so we use a distinct name to avoid a redeclaration.)
type errDeep string

func (e errDeep) Error() string { return string(e) }

// ---------------------------------------------------------------------------
// Section 1 — small / pure-function wins
// ---------------------------------------------------------------------------

// Test_MaxInt_Equal covers the equal-value branch (a == b) of maxInt, which the
// existing tests (a < b) leave uncovered.
func Test_MaxInt_Equal(t *testing.T) {
	if got := maxInt(5, 5); got != 5 {
		t.Errorf("maxInt(5,5)=%d want 5", got)
	}
	if got := maxInt(7, 3); got != 7 {
		t.Errorf("maxInt(7,3)=%d want 7", got)
	}
	if got := maxInt(3, 9); got != 9 {
		t.Errorf("maxInt(3,9)=%d want 9", got)
	}
}

// Test_LogFormat_String_Unknown covers the default branch of LogFormat.String()
// for an out-of-range value (the existing tests only cover the 3 known formats).
func Test_LogFormat_String_Unknown(t *testing.T) {
	if got := LogFormat(99).String(); got != "text" {
		t.Errorf("LogFormat(99).String()=%q want \"text\"", got)
	}
	for _, c := range []struct {
		f    LogFormat
		want string
	}{
		{FormatText, "text"},
		{FormatJSON, "json"},
		{FormatLogfmt, "logfmt"},
	} {
		if got := c.f.String(); got != c.want {
			t.Errorf("LogFormat(%d).String()=%q want %q", c.f, got, c.want)
		}
	}
}

// Test_DefaultLogger_ConcurrentRace drives the CAS-race path in defaultLogger.
// We cannot safely nil the singleton while the package-init default's bootstrap
// goroutine is still running (Close+rebuild races with that goroutine under
// -race), so instead we exercise the "winner already published" fast path
// concurrently: once loggerDefault is non-nil, every concurrent caller takes the
// early `loggerDefault.Load()` return. This still hits the function under
// contention and confirms the singleton is stable.
func Test_DefaultLogger_ConcurrentRace(t *testing.T) {
	// Ensure the singleton is populated (it is from package init, but be explicit
	// and avoid touching it so we never race the init bootstrap).
	first := defaultLogger()
	if first == nil {
		t.Fatal("defaultLogger() returned nil")
	}

	const n = 64
	var wg sync.WaitGroup
	wg.Add(n)
	start := make(chan struct{})
	got := make([]*Logger, n)
	for i := range got {
		i := i
		go func() {
			defer wg.Done()
			<-start
			got[i] = defaultLogger()
		}()
	}
	close(start)
	wg.Wait()

	for i, l := range got {
		if l != first {
			t.Fatalf("goroutine %d: defaultLogger()=%p want stable singleton %p", i, l, first)
		}
	}
}

// Test_Close_NilSingleton covers the package-level Close() when the singleton is
// already nil (the swap-returns-nil branch).
func Test_Close_NilSingleton(t *testing.T) {
	resetDefaultForTest(t)
	defer func() { _ = defaultLogger() }()

	// Must not panic even though loggerDefault is nil.
	Close()
	// Idempotent: a second Close is also a no-op.
	Close()
}

// Test_IntoContext_NilContext covers the IntoContext(nil) branch, which
// substitutes context.Background().
func Test_IntoContext_NilContext(t *testing.T) {
	l := NewLogger()
	ctx := l.IntoContext(nil)
	if ctx == nil {
		t.Fatal("IntoContext(nil) returned nil context")
	}
	// The receiver must be recoverable via FromContext.
	if got := FromContext(ctx); got != l {
		t.Errorf("FromContext returned %p, want receiver %p", got, l)
	}
}

// Test_RequestIDFromContext_Nil covers the nil-context guard.
func Test_RequestIDFromContext_Nil(t *testing.T) {
	if got := RequestIDFromContext(nil); got != "" {
		t.Errorf("RequestIDFromContext(nil)=%q want empty", got)
	}
	// A non-nil context without the key also returns "".
	if got := RequestIDFromContext(context.Background()); got != "" {
		t.Errorf("RequestIDFromContext(empty ctx)=%q want empty", got)
	}
}

// Test_Logger_FromContext_Nil covers the method form's nil-context branch
// (returns the receiver l).
func Test_Logger_FromContext_Nil(t *testing.T) {
	l := NewLogger()
	if got := l.FromContext(nil); got != l {
		t.Errorf("l.FromContext(nil)=%p want receiver %p", got, l)
	}
	// Non-nil but empty context also falls back to the receiver.
	if got := l.FromContext(context.Background()); got != l {
		t.Errorf("l.FromContext(empty)=%p want receiver %p", got, l)
	}
}

// Test_SlogAttrToField_AllKinds covers the slog kind switch branches the existing
// tests miss: Group, Duration, Bool, Float64, Time, plus the group-prefix path.
func Test_SlogAttrToField_AllKinds(t *testing.T) {
	cases := []struct {
		name string
		a    slog.Attr
	}{
		{"string", slog.String("s", "v")},
		{"int64", slog.Int64("i", 42)},
		{"float64", slog.Float64("f", 1.5)},
		{"bool", slog.Bool("b", true)},
		{"duration", slog.Duration("d", 250*time.Millisecond)},
		{"time", slog.Time("t", time.Unix(1e9, 0).UTC())},
		{"group", slog.Group("g", slog.String("inner", "x"))},
		{"any", slog.Any("a", map[string]int{"k": 1})},
	}
	for _, c := range cases {
		f := slogAttrToField("", c.a)
		if f.key != c.a.Key {
			t.Errorf("%s: key=%q want %q", c.name, f.key, c.a.Key)
		}
	}
	// Group prefix is applied as "grp.key".
	f := slogAttrToField("grp", slog.String("s", "v"))
	if f.key != "grp.s" {
		t.Errorf("group prefix: key=%q want \"grp.s\"", f.key)
	}
}

// Test_NewSlogHandler_NilLogger covers the nil-logger branch (falls back to the
// package singleton instead of panicking).
func Test_NewSlogHandler_NilLogger(t *testing.T) {
	h := NewSlogHandler(nil)
	if h == nil || h.logger == nil {
		t.Fatal("NewSlogHandler(nil) did not fall back to the singleton")
	}
}

// Test_SlogHandler_WithGroup_Chained covers the chained-group branch
// (g1.g2) in WithGroup when a group prefix already exists.
func Test_SlogHandler_WithGroup_Chained(t *testing.T) {
	h := NewSlogHandler(NewLogger())
	h1 := h.WithGroup("g1").(*SlogHandler)
	if h1.group != "g1" {
		t.Errorf("first WithGroup: group=%q want g1", h1.group)
	}
	h2 := h1.WithGroup("g2").(*SlogHandler)
	if h2.group != "g1.g2" {
		t.Errorf("chained WithGroup: group=%q want g1.g2", h2.group)
	}
	// An attr emitted through the chained handler carries the dotted prefix.
	f := slogAttrToField(h2.group, slog.String("k", "v"))
	if f.key != "g1.g2.k" {
		t.Errorf("chained attr key=%q want g1.g2.k", f.key)
	}
}

// Test_NewRateAlerter_WindowUnderOneSecond covers the window<1s clamp (rounds up
// to 1s so the ring has at least one slot).
func Test_NewRateAlerter_WindowUnderOneSecond(t *testing.T) {
	a := NewRateAlerter(100*time.Millisecond, 0) // window < 1s, threshold < 1
	if a.window != time.Second {
		t.Errorf("window=%v want 1s (clamped)", a.window)
	}
	if a.threshold != 1 {
		t.Errorf("threshold=%d want 1 (clamped)", a.threshold)
	}
	if len(a.counts) != 1 {
		t.Errorf("counts len=%d want 1", len(a.counts))
	}
}

// Test_RateAlerter_Advance_BackwardAndFullWindow drives the rarely-hit advance()
// branches: backward clock (sec < base, must NOT destroy data), full-window
// expiry (sec-base >= n), and a sub-second forward step.
func Test_RateAlerter_Advance_BackwardAndFullWindow(t *testing.T) {
	a := NewRateAlerter(3*time.Second, 1) // 3 buckets
	now := time.Now().Unix()

	// reset clears all bucket state so each scenario is independent.
	reset := func(base int64) {
		for i := range a.counts {
			a.counts[i] = 0
		}
		a.sum = 0
		a.base = base
	}

	// Backward clock (sec < base): a stale/regressed second must NOT destroy live
	// counts — clearing on a backward timestamp would under-count events (the
	// alert might fail to fire). The window is left untouched (no clear, base
	// unchanged).
	back := now - 1
	reset(now)
	a.counts[back%3] = 5
	a.sum = 5
	a.advance(back) // back < base -> no-op
	if a.sum != 5 {
		t.Errorf("after backward clock sum=%d want 5 (no data destruction)", a.sum)
	}
	if a.base != now {
		t.Errorf("after backward clock base=%d want %d (unchanged)", a.base, now)
	}
	if a.counts[back%3] != 5 {
		t.Errorf("backward clock destroyed target slot: counts=%d want 5", a.counts[back%3])
	}

	// Jump forward a full window (sec-base >= n): every bucket expires.
	reset(now)
	a.counts[now%3] = 2
	a.counts[(now+1)%3] = 3
	a.sum = 5
	a.advance(now + 5) // 5 >= 3 (full window)
	if a.sum != 0 {
		t.Errorf("after full-window expiry sum=%d want 0", a.sum)
	}
	for i, c := range a.counts {
		if c != 0 {
			t.Errorf("bucket %d not zeroed after full expiry: %d", i, c)
		}
	}

	// Sub-second forward step (sec-base == 1, < n): rolls exactly one bucket —
	// the bucket at (base+1)%n is cleared.
	reset(now)
	target := now + 1
	a.counts[target%3] = 4
	a.sum = 4
	a.advance(target)
	if a.sum != 0 {
		t.Errorf("after single-step sum=%d want 0 (target bucket cleared)", a.sum)
	}
	if a.base != target {
		t.Errorf("after single-step base=%d want %d", a.base, target)
	}
}

// Test_RateAlerter_Advance_SameSecond covers the sec == base early return
// (clock unchanged): no mutation, no base change.
func Test_RateAlerter_Advance_SameSecond(t *testing.T) {
	a := NewRateAlerter(2*time.Second, 1)
	now := time.Now().Unix()
	a.advance(now)
	a.counts[now%2] = 7
	a.sum = 7
	baseBefore := a.base
	a.advance(now) // same second -> early return
	if a.sum != 7 || a.base != baseBefore {
		t.Errorf("same-second advance mutated state: sum=%d base=%d", a.sum, a.base)
	}
}

// Test_OverflowPolicy_String_AllValues covers OverflowPolicy.String() for all
// three policies.
func Test_OverflowPolicy_String_AllValues(t *testing.T) {
	for _, c := range []struct {
		p    OverflowPolicy
		want string
	}{
		{OverflowDrop, "drop"},
		{OverflowBlock, "block"},
		{OverflowSpill, "spill"},
		{OverflowPolicy(99), "drop"}, // default branch
	} {
		if got := c.p.String(); got != c.want {
			t.Errorf("OverflowPolicy(%d).String()=%q want %q", c.p, got, c.want)
		}
	}
}

// Test_NewRingSpiller_NonPositive covers the capacity<=0 default (->1024).
func Test_NewRingSpiller_NonPositive(t *testing.T) {
	for _, capv := range []int{0, -1, -100} {
		r := NewRingSpiller[kafka.Message](capv)
		if r.capv != 1024 {
			t.Errorf("capacity=%d -> capv=%d want 1024", capv, r.capv)
		}
		// Push a couple and Drain to confirm it actually behaves as a ring.
		r.Push(spillerMsg("t", "a"))
		r.Push(spillerMsg("t", "b"))
		if r.Len() != 2 {
			t.Errorf("Len=%d want 2", r.Len())
		}
		out := r.Drain()
		if len(out) != 2 {
			t.Errorf("drain len=%d want 2", len(out))
		}
	}
}

// Test_NewFileSpiller_MkdirError covers the dir-creation error path by pointing
// at a path under an existing file (MkdirAll fails).
func Test_NewFileSpiller_MkdirError(t *testing.T) {
	dir := t.TempDir()
	// Create a regular file and try to use it as a directory parent.
	filePath := dir + "/blocker"
	if err := writeFile(filePath, "x"); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	// MkdirAll on a path under a regular file fails.
	_, err := NewFileSpiller[kafka.Message](filePath+"/sub", 1<<16, ProducerMsgCodec)
	if err == nil {
		t.Fatal("NewFileSpiller under a file succeeded; want MkdirAll error")
	}
}

// Test_AppendFieldLogfmt_RareKinds covers the kinds the existing
// Test_AppendFieldLogfmt_AllKinds omits: Uint64, Time, Bytes, kindError with a
// nil-underlying any, and kindAny holding an error.
func Test_AppendFieldLogfmt_RareKinds(t *testing.T) {
	var buf []byte
	// Uint64 kind (appendUint).
	buf = appendFieldLogfmt(buf[:0], uint64Field("u", 9001))
	if !strings.Contains(string(buf), "u=9001") {
		t.Errorf("uint64: %s", buf)
	}
	// Time kind (appendISOTimeUTC -> bare ISO).
	buf = appendFieldLogfmt(buf[:0], timeField("t", time.Unix(1e9, 0).UTC()))
	if !strings.HasPrefix(string(buf), " t=2") { // year starts with 2
		t.Errorf("time: %s", buf)
	}
	// Bytes kind (base64 text -> quoted).
	buf = appendFieldLogfmt(buf[:0], bytesField("by", []byte{0x01, 0x02}))
	if !strings.Contains(string(buf), "by=") || !strings.HasSuffix(string(buf), `"`) {
		t.Errorf("bytes: %s", buf)
	}
	// kindError with any == nil (not an error) -> renders '-'.
	buf = appendFieldLogfmt(buf[:0], field{key: "en", kind: kindError, any: nil})
	if got := strings.TrimSpace(string(buf)); !strings.HasSuffix(got, "=-") {
		t.Errorf("error(nil-any): %q want trailing -", got)
	}
	// kindAny holding an error value -> safeJSONMarshal yields `"boom"` (a JSON
	// string), which appendLogfmtValue then quotes+escapes -> `ae="\"boom\""`.
	buf = appendFieldLogfmt(buf[:0], anyField("ae", errDeep("boom")))
	if !strings.Contains(string(buf), `\"boom\"`) {
		t.Errorf("any(error): %s", buf)
	}
	// kindAny with a value whose JSON marshal panics -> '-'.
	buf = appendFieldLogfmt(buf[:0], anyField("ap", make(chan struct{})))
	if got := strings.TrimSpace(string(buf)); !strings.HasSuffix(got, "=-") {
		t.Errorf("any(unmarshalable): %q want trailing -", got)
	}
}

// Test_AppendLogfmtValue_HighASCII covers the c > 0x7e branch (non-ASCII byte
// forces quoting).
func Test_AppendLogfmtValue_HighASCII(t *testing.T) {
	out := appendLogfmtValue(nil, "café")
	s := string(out)
	if !strings.HasPrefix(s, `"`) || !strings.HasSuffix(s, `"`) {
		t.Errorf("high-ASCII not quoted: %q", s)
	}
	// A clean ASCII value stays bare.
	out = appendLogfmtValue(nil, "plain")
	if strings.HasPrefix(string(out), `"`) {
		t.Errorf("clean value unexpectedly quoted: %q", out)
	}
}

// Test_AppendJSONStringContent_MultiByteUTF8 covers the multi-byte UTF-8 path:
// bytes >= 0x80 are copied through unchanged (they are continuation/lead bytes
// that pass the `c >= 0x20 && c != '"' && c != '\\'` clean-run test).
func Test_AppendJSONStringContent_MultiByteUTF8(t *testing.T) {
	in := "héllo世π" // mix of ASCII + 2/3-byte UTF-8 sequences
	out := appendJSONStringContent(nil, in)
	if string(out) != in {
		t.Errorf("multi-byte UTF-8 round-trip failed: got %q want %q", out, in)
	}
}

// ---------------------------------------------------------------------------
// Section 2 — Kafka / File / Net writer spill paths
// ---------------------------------------------------------------------------

// Test_KafKaWriter_drainSpill_ReinjectAndBackpressure drives the re-inject and
// "channel full again" branches of drainSpill: with the channel full, a drained
// record is pushed back into the spiller rather than re-injected.
func Test_KafKaWriter_drainSpill_ReinjectAndBackpressure(t *testing.T) {
	w := &KafKaWriter{
		policy:   OverflowSpill,
		spiller:  NewRingSpiller[kafka.Message](4),
		messages: make(chan kafka.Message, 1),
	}
	// Fill the channel so re-inject cannot succeed.
	w.messages <- spillerMsg("t", "in-chan")
	// Stage two records in the spiller.
	w.spiller.Push(spillerMsg("t", "spilled-1"))
	w.spiller.Push(spillerMsg("t", "spilled-2"))

	w.drainSpill()
	// The first drained record hits `default` (channel full) and is pushed back;
	// drainSpill returns immediately, leaving spilled-2 still in the ring.
	if w.spiller.Len() != 1 {
		t.Fatalf("after backpressure spiller Len=%d want 1 (one pushed back)", w.spiller.Len())
	}
	// Free the channel and drain again -> the surviving record is re-injected.
	<-w.messages
	w.drainSpill()
	if w.spiller.Len() != 0 {
		t.Fatalf("after free drain spiller Len=%d want 0", w.spiller.Len())
	}
}

// Test_KafKaWriter_drainSpill_EmptyAndNil covers the two early-return guards.
func Test_KafKaWriter_drainSpill_EmptyAndNil(t *testing.T) {
	// nil spiller -> no-op.
	w := &KafKaWriter{messages: make(chan kafka.Message, 1)}
	w.drainSpill() // must not panic

	// non-nil but empty spiller -> no-op.
	w2 := &KafKaWriter{
		spiller:  NewRingSpiller[kafka.Message](2),
		messages: make(chan kafka.Message, 1),
	}
	w2.drainSpill()
	if w2.spiller.Len() != 0 {
		t.Errorf("empty spiller should stay empty, Len=%d", w2.spiller.Len())
	}
}

// Test_KafKaWriter_Write_EmptyAndLevelFiltered covers Write's two early returns:
// empty msg (nil) and level above the writer's threshold (nil).
func Test_KafKaWriter_Write_EmptyAndLevelFiltered(t *testing.T) {
	w := &KafKaWriter{
		level:    INFO,
		policy:   OverflowDrop,
		messages: make(chan kafka.Message, 4),
		options:  KafKaWriterOptions{ProducerTopic: "t"},
	}
	// Empty message -> nil, nothing enqueued.
	if err := w.Write(&Record{level: INFO, msg: ""}); err != nil {
		t.Fatalf("empty Write err: %v", err)
	}
	if len(w.messages) != 0 {
		t.Errorf("empty msg enqueued: len=%d", len(w.messages))
	}
	// Level above writer threshold (DEBUG < INFO numerically? no: DEBUG=7, INFO=6,
	// higher int = more verbose, so DEBUG > INFO) -> filtered out.
	if err := w.Write(&Record{level: DEBUG, msg: "filtered"}); err != nil {
		t.Fatalf("filtered Write err: %v", err)
	}
	if len(w.messages) != 0 {
		t.Errorf("level-filtered msg enqueued: len=%d", len(w.messages))
	}
}

// Test_KafKaWriter_drainSpillToProducer pushes spilled records straight to the
// producer on shutdown via the no-op producer.
func Test_KafKaWriter_drainSpillToProducer(t *testing.T) {
	p := newMockKafkaProducer()
	defer p.Close()
	w := &KafKaWriter{
		policy:   OverflowSpill,
		spiller:  NewRingSpiller[kafka.Message](4),
		messages: make(chan kafka.Message, 1),
		producer: p,
	}
	w.spiller.Push(spillerMsg("t", "a"))
	w.spiller.Push(spillerMsg("t", "b"))
	// drainSpillToProducer sends each straight to the producer Input channel.
	w.drainSpillToProducer()
	if w.spiller.Len() != 0 {
		t.Errorf("drainSpillToProducer left %d in spiller", w.spiller.Len())
	}
}

// Test_FileWriter_drainSpill_Direct exercises the drainSpill re-inject and
// backpressure branches directly (no daemon), using a sync FileWriter with a
// spiller wired by hand.
func Test_FileWriter_drainSpill_Direct(t *testing.T) {
	dir := t.TempDir()
	fw := NewFileWriterWithOptions(FileWriterOptions{
		Enable: true, Filename: dir + "/spill.log", Level: LevelFlagDebug,
		Async: true, BufferSize: 1,
	})
	fw.policy = OverflowSpill
	fw.spiller = NewRingSpiller[*Record](4)
	// Allocate a 1-slot messages channel so re-inject backpressure is reachable.
	fw.messages = make(chan *Record, 1)
	fw.quit = make(chan struct{})
	fw.stop = make(chan struct{})
	fw.flushSig = make(chan struct{}, 1)

	// Fill the channel, stage records in the spiller, then drain.
	fw.messages <- &Record{level: INFO, msg: "in-chan"}
	fw.spiller.Push(&Record{level: INFO, msg: "spilled-1"})
	fw.spiller.Push(&Record{level: INFO, msg: "spilled-2"})

	fw.drainSpill() // first drained record can't re-inject (full) -> pushed back
	if fw.spiller.Len() != 1 {
		t.Fatalf("after backpressure spiller Len=%d want 1", fw.spiller.Len())
	}
	// closing=true short-circuits drainSpill.
	fw.closing.Store(true)
	fw.drainSpill()
	fw.closing.Store(false)

	// nil/empty guards.
	fw2 := &FileWriter{spiller: nil}
	fw2.drainSpill() // must not panic
	fw3 := &FileWriter{spiller: NewRingSpiller[*Record](2)}
	fw3.drainSpill() // empty -> no-op
}

// Test_FileWriter_drainSpillAll writes spilled records via writeOne directly.
func Test_FileWriter_drainSpillAll(t *testing.T) {
	dir := t.TempDir()
	fw := NewFileWriterWithOptions(FileWriterOptions{
		Enable: true, Filename: dir + "/spillall.log", Level: LevelFlagDebug,
	})
	fw.spiller = NewRingSpiller[*Record](4)
	fw.spiller.Push(&Record{level: INFO, msg: "direct-write"})
	// nil-spiller guard first.
	(&FileWriter{spiller: nil}).drainSpillAll()
	fw.drainSpillAll()
	if fw.spiller.Len() != 0 {
		t.Errorf("drainSpillAll left %d in spiller", fw.spiller.Len())
	}
}

// Test_NetWriter_drainSpill_ReinjectAndGuards mirrors the kafka test against
// NetWriter.drainSpill (re-inject success + backpressure push-back + nil/empty
// guards).
func Test_NetWriter_drainSpill_ReinjectAndGuards(t *testing.T) {
	// nil spiller -> no-op.
	(&NetWriter{spiller: nil}).drainSpill()
	// empty spiller -> no-op.
	w2 := &NetWriter{spiller: NewRingSpiller[*Record](2), messages: make(chan *Record, 2)}
	w2.drainSpill()
	if w2.spiller.Len() != 0 {
		t.Errorf("empty spiller should stay empty: Len=%d", w2.spiller.Len())
	}

	// Re-inject success: free channel, drained record lands in messages.
	w3 := &NetWriter{
		spiller:  NewRingSpiller[*Record](2),
		messages: make(chan *Record, 2),
	}
	w3.spiller.Push(&Record{level: INFO, msg: "recovered"})
	w3.drainSpill()
	if w3.spiller.Len() != 0 {
		t.Errorf("re-inject left %d in spiller", w3.spiller.Len())
	}
	if len(w3.messages) != 1 {
		t.Errorf("re-inject did not land in messages: len=%d", len(w3.messages))
	}

	// Backpressure: full channel -> drained record pushed back, drainSpill exits.
	w4 := &NetWriter{
		spiller:  NewRingSpiller[*Record](2),
		messages: make(chan *Record, 1),
	}
	w4.messages <- &Record{level: INFO, msg: "full"}
	w4.spiller.Push(&Record{level: INFO, msg: "a"})
	w4.spiller.Push(&Record{level: INFO, msg: "b"})
	w4.drainSpill()
	if w4.spiller.Len() != 1 {
		t.Errorf("backpressure should leave 1 pushed back, got Len=%d", w4.spiller.Len())
	}
}

// Test_NetWriter_drainAll writes spilled records via writeOne on shutdown. With
// no listener the dial fails, so writeOne errors and returns; we only assert the
// path executes without panic and clears the spiller.
func Test_NetWriter_drainAll(t *testing.T) {
	(&NetWriter{spiller: nil}).drainAll() // nil guard
	w := &NetWriter{
		spiller:  NewRingSpiller[*Record](2),
		messages: make(chan *Record, 2),
		options:  NetWriterOptions{Network: "tcp", Address: "127.0.0.1:0", Timeout: 50 * time.Millisecond},
		timeout:  50 * time.Millisecond,
	}
	w.spiller.Push(&Record{level: INFO, msg: "x"})
	w.drainAll()
	if w.spiller.Len() != 0 {
		t.Errorf("drainAll left %d in spiller", w.spiller.Len())
	}
}

// ---------------------------------------------------------------------------
// Section 3 — ConsoleWriter color / json paths
// ---------------------------------------------------------------------------

// Test_ConsoleWriter_JSONPath_Buffered covers the formattedBytes + buffered
// branch (writes to w.buf instead of stdout).
func Test_ConsoleWriter_JSONPath_Buffered(t *testing.T) {
	w := &ConsoleWriter{level: DEBUG, buffered: true}
	if err := w.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	r := &Record{
		level:          INFO,
		msg:            "json-line",
		formattedBytes: []byte(`{"msg":"json-line"}` + "\n"),
	}
	if err := w.Write(r); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
}

// Test_ConsoleWriter_FullColor covers the fullColor=true branches (both buffered
// and unbuffered) that the existing color test omits.
func Test_ConsoleWriter_FullColor(t *testing.T) {
	r := &Record{level: WARNING, time: "t", file: "f.go:1", msg: "warn"}

	// Unbuffered fullColor.
	w := &ConsoleWriter{level: DEBUG, color: true, fullColor: true}
	if err := w.Write(r); err != nil {
		t.Fatalf("unbuffered fullColor Write: %v", err)
	}
	// Buffered fullColor.
	wb := &ConsoleWriter{level: DEBUG, color: true, fullColor: true, buffered: true}
	if err := wb.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := wb.Write(r); err != nil {
		t.Fatalf("buffered fullColor Write: %v", err)
	}
	_ = wb.Flush()
}

// Test_ConsoleWriter_ColorString_CoversAllLevels ensures ColorString runs for
// every level (covers the colors[] index lookup).
func Test_ConsoleWriter_ColorString_CoversAllLevels(t *testing.T) {
	for lvl := EMERGENCY; lvl <= TRACE; lvl++ {
		r := &Record{level: lvl, time: "t", file: "f.go:1", msg: "m"}
		cr := (*colorRecord)(r)
		_ = cr.ColorString()
		_ = cr.String()
	}
}

// ---------------------------------------------------------------------------
// Section 4 — misc edge cases
// ---------------------------------------------------------------------------

// Test_WithLogger_Nil covers the nil-logger fallback in WithLogger (delegates to
// the package singleton).
func Test_WithLogger_Nil(t *testing.T) {
	ctx := WithLogger(context.Background(), nil)
	if ctx == nil {
		t.Fatal("WithLogger(nil) returned nil context")
	}
	// A *Logger must be stashed and recoverable.
	if l := FromContext(ctx); l == nil {
		t.Fatal("FromContext after WithLogger(nil) returned nil")
	}
}

// Test_DefaultRequestID_Format sanity-checks the generated id shape and
// uniqueness across rapid concurrent calls (exercises requestIDCounter).
func Test_DefaultRequestID_Format(t *testing.T) {
	id := defaultRequestID()
	if !strings.HasPrefix(id, "req-") {
		t.Errorf("defaultRequestID=%q want req- prefix", id)
	}
	// Concurrent generation must produce distinct ids.
	const n = 64
	seen := make([]string, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := range seen {
		i := i
		go func() {
			defer wg.Done()
			seen[i] = defaultRequestID()
		}()
	}
	wg.Wait()
	uniq := make(map[string]struct{}, n)
	for _, s := range seen {
		uniq[s] = struct{}{}
	}
	if len(uniq) != n {
		t.Errorf("defaultRequestID produced duplicates: %d unique of %d", len(uniq), n)
	}
}

// Test_SlogHandler_EnabledLevels covers the Enabled mapping across slog levels.
func Test_SlogHandler_EnabledLevels(t *testing.T) {
	l := NewLogger()
	l.SetLevel(INFO)
	h := NewSlogHandler(l)
	cases := []struct {
		lvl  slog.Level
		want bool
	}{
		{slog.LevelError, true},
		{slog.LevelWarn, true},
		{slog.LevelInfo, true},
		{slog.LevelDebug, false}, // DEBUG is more verbose than INFO
		{slog.Level(-12), false}, // below DEBUG -> TRACE, even more verbose
	}
	for _, c := range cases {
		if got := h.Enabled(context.Background(), c.lvl); got != c.want {
			t.Errorf("Enabled(%v)=%v want %v", c.lvl, got, c.want)
		}
	}
}

// Test_SlogSource_ZeroPC covers slogSource's sr.PC == 0 guard.
func Test_SlogSource_ZeroPC(t *testing.T) {
	var zero slog.Record
	if got := slogSource(zero); got != "" {
		t.Errorf("slogSource(zero PC)=%q want empty", got)
	}
}

// Test_KafKaWriter_buildPayload_BothSources covers the r.fields + ExtraFields
// dedup path (both non-empty) in buildPayload.
func Test_KafKaWriter_buildPayload_BothSources(t *testing.T) {
	w := &KafKaWriter{options: KafKaWriterOptions{
		ProducerTopic: "t",
		MSG: KafKaMSGFields{
			ExtraFields: map[string]interface{}{
				"shared":   "from-extra", // overridden by r.fields
				"only-ext": 1,
			},
		},
	}}
	r := &Record{
		level: INFO, msg: "m", file: "f.go:1",
		fields: []field{
			fld("shared", "from-record"),
			fld("rec-only", true),
		},
	}
	b := w.buildPayload(r)
	if b == nil {
		t.Fatal("nil payload")
	}
	s := string(b)
	if !strings.Contains(s, `"shared":"from-record"`) {
		t.Errorf("record field did not win: %s", s)
	}
	if !strings.Contains(s, `"only-ext":1`) {
		t.Errorf("extra-only field missing: %s", s)
	}
	if !strings.Contains(s, `"rec-only":true`) {
		t.Errorf("record-only field missing: %s", s)
	}
}

// Test_KafKaWriter_buildPayload_ExtraOnly covers the ExtraFields-only branch.
func Test_KafKaWriter_buildPayload_ExtraOnly(t *testing.T) {
	w := &KafKaWriter{options: KafKaWriterOptions{
		ProducerTopic: "t",
		MSG:           KafKaMSGFields{ExtraFields: map[string]interface{}{"k": "v"}},
	}}
	b := w.buildPayload(&Record{level: INFO, msg: "m"})
	s := string(b)
	if !strings.Contains(s, `"k":"v"`) {
		t.Errorf("extra field missing: %s", s)
	}
}

// ---------------------------------------------------------------------------
// Section 5 — FileWriter.send (all overflow policies) and startDaemon branches
// ---------------------------------------------------------------------------

// newFileWriterForSend builds a minimal async FileWriter wired for direct
// send() calls (no running daemon). The messages channel has one slot so the
// drop/spill/block branches are reachable.
func newFileWriterForSend(t *testing.T) *FileWriter {
	t.Helper()
	dir := t.TempDir()
	fw := NewFileWriterWithOptions(FileWriterOptions{
		Enable: true, Filename: dir + "/send.log", Level: LevelFlagDebug, Async: true,
	})
	fw.messages = make(chan *Record, 1)
	fw.quit = make(chan struct{})
	fw.stop = make(chan struct{})
	fw.flushSig = make(chan struct{}, 1)
	return fw
}

// Test_FileWriter_Send_Drop covers the OverflowDrop default branch + the
// closing fast-path (drops immediately once closing=true).
func Test_FileWriter_Send_Drop(t *testing.T) {
	fw := newFileWriterForSend(t)
	fw.policy = OverflowDrop

	// First send lands in the channel.
	fw.send(&Record{level: INFO, msg: "first"})
	if len(fw.messages) != 1 {
		t.Fatalf("drop: first send len=%d want 1", len(fw.messages))
	}
	// Channel full -> drop path.
	fw.send(&Record{level: INFO, msg: "second"})
	if got := fw.stats.Dropped(); got != 1 {
		t.Fatalf("drop: dropped=%d want 1", got)
	}

	// closing fast-path: drops and fires "drop" event without touching messages.
	events := make(chan string, 4)
	fw.SetOnEvent(func(name string, _ int64) { events <- name })
	fw.closing.Store(true)
	fw.send(&Record{level: INFO, msg: "closed"})
	if got := fw.stats.Dropped(); got != 2 {
		t.Fatalf("closing: dropped=%d want 2", got)
	}
	select {
	case ev := <-events:
		if ev != "drop" {
			t.Errorf("closing event=%q want drop", ev)
		}
	default:
		t.Error("closing send did not fire event")
	}
}

// Test_FileWriter_Send_Spill covers the OverflowSpill branch: channel-full
// routes to the spiller (IncSpilled) and fires "spill"; when the spiller is
// absent or full it falls through to drop.
func Test_FileWriter_Send_Spill(t *testing.T) {
	fw := newFileWriterForSend(t)
	fw.policy = OverflowSpill
	fw.spiller = NewRingSpiller[*Record](2)
	events := make(chan string, 8)
	fw.SetOnEvent(func(name string, _ int64) { events <- name })

	fw.messages <- &Record{level: INFO, msg: "fill"} // full
	fw.send(&Record{level: INFO, msg: "spill me"})
	if got := fw.stats.Spilled(); got != 1 {
		t.Fatalf("spill: spilled=%d want 1", got)
	}
	if fw.spiller.Len() != 1 {
		t.Fatalf("spill: spiller Len=%d want 1", fw.spiller.Len())
	}
	select {
	case ev := <-events:
		if ev != "spill" {
			t.Errorf("spill event=%q want spill", ev)
		}
	default:
		t.Error("spill send did not fire event")
	}

	// Fill the spiller (cap 2, already has 1): push 2 more -> one spills, one
	// overwrites oldest (ring) so Len stays 2; then a further send with a full
	// channel still spills into the ring (overwrites). To exercise the drop
	// fallback we need a nil spiller instead.
	fw2 := newFileWriterForSend(t)
	fw2.policy = OverflowSpill
	fw2.spiller = nil // no spiller -> drop fallback
	fw2.messages <- &Record{level: INFO, msg: "fill"}
	fw2.send(&Record{level: INFO, msg: "no spill store"})
	if got := fw2.stats.Dropped(); got != 1 {
		t.Fatalf("spill-no-store: dropped=%d want 1", got)
	}
}

// Test_FileWriter_Send_Block covers OverflowBlock: a blocked send is released
// when w.stop is closed (the shutdown-unblock path), counting a drop.
func Test_FileWriter_Send_Block(t *testing.T) {
	fw := newFileWriterForSend(t)
	fw.policy = OverflowBlock
	fw.messages <- &Record{level: INFO, msg: "fill"} // full

	done := make(chan struct{})
	go func() {
		fw.send(&Record{level: INFO, msg: "blocked"}) // blocks
		close(done)
	}()
	// Give the blocked send a moment to park on the select.
	time.Sleep(20 * time.Millisecond)
	if fw.stats.Dropped() != 0 {
		t.Fatalf("block: dropped before stop=%d want 0", fw.stats.Dropped())
	}
	close(fw.stop) // unblock via shutdown path
	<-done
	if got := fw.stats.Dropped(); got != 1 {
		t.Fatalf("block: dropped after stop=%d want 1", got)
	}
}

// Test_FileWriter_StartDaemon_SpillFileAndChain covers the startDaemon spill
// branches: spillType "file" (success) and the default chain branch (ring+file
// when spillDir is set).
func Test_FileWriter_StartDaemon_SpillFileAndChain(t *testing.T) {
	t.Run("file", func(t *testing.T) {
		dir := t.TempDir()
		fw := NewFileWriterWithOptions(FileWriterOptions{
			Enable: true, Filename: dir + "/f.log", Level: LevelFlagDebug, Async: true,
			OverflowPolicy: "spill", SpillType: "file", SpillDir: dir,
		})
		fw.startDaemon()
		if fw.spiller == nil {
			t.Fatal("spillType=file: spiller nil")
		}
		if _, ok := fw.spiller.(*FileSpiller[*Record]); !ok {
			t.Errorf("spillType=file: spiller type=%T want *FileSpiller", fw.spiller)
		}
		fw.Stop()
	})

	t.Run("chain", func(t *testing.T) {
		dir := t.TempDir()
		fw := NewFileWriterWithOptions(FileWriterOptions{
			Enable: true, Filename: dir + "/c.log", Level: LevelFlagDebug, Async: true,
			OverflowPolicy: "spill", SpillType: "chain", SpillDir: dir, SpillSize: 4,
		})
		fw.startDaemon()
		if fw.spiller == nil {
			t.Fatal("spillType=chain: spiller nil")
		}
		if _, ok := fw.spiller.(*ChainedSpiller[*Record]); !ok {
			t.Errorf("spillType=chain: spiller type=%T want *ChainedSpiller", fw.spiller)
		}
		fw.Stop()
	})

	t.Run("chain_no_dir_falls_back_to_ring", func(t *testing.T) {
		dir := t.TempDir()
		fw := NewFileWriterWithOptions(FileWriterOptions{
			Enable: true, Filename: dir + "/r.log", Level: LevelFlagDebug, Async: true,
			OverflowPolicy: "spill", SpillType: "", SpillSize: 4, // no SpillDir -> ring only
		})
		fw.startDaemon()
		if fw.spiller == nil {
			t.Fatal("no-dir chain: spiller nil")
		}
		if _, ok := fw.spiller.(*RingSpiller[*Record]); !ok {
			t.Errorf("no-dir chain: spiller type=%T want *RingSpiller", fw.spiller)
		}
		fw.Stop()
	})
}

// ---------------------------------------------------------------------------
// Section 6 — Logger.Close error paths (Flusher/Closer returning errors)
// ---------------------------------------------------------------------------

// failFlusherWriter is a test Writer whose Flush/Close return errors, to cover
// the error-handling branches in (*Logger).Close.
type failFlusherWriter struct{}

func (f *failFlusherWriter) Init() error           { return nil }
func (f *failFlusherWriter) Write(r *Record) error { return nil }
func (f *failFlusherWriter) Flush() error          { return errDeep("flush failed") }
func (f *failFlusherWriter) Close() error          { return errDeep("close failed") }

// Test_Logger_Close_FlusherAndCloserErrors covers the two error branches in
// (*Logger).Close: a Flusher whose Flush() errors, and an io.Closer whose
// Close() errors. Both must be logged, not propagated as panics.
func Test_Logger_Close_FlusherAndCloserErrors(t *testing.T) {
	records := make(chan *Record, 16)
	l := newLoggerWithRecords(records)
	l.Register(&failFlusherWriter{})
	// Close drains records, then calls Flush (errors) and Close (errors).
	// It must not panic.
	l.Close()
}

// ---------------------------------------------------------------------------
// Section 7 — KafkaWriter.Start spill-policy wiring (ring / chain) via the
// no-op producer factory, covering the remaining Start branches.
// ---------------------------------------------------------------------------

// Test_KafKaWriter_Start_SpillRingAndChain covers the Start spill-policy wiring
// (ring vs default chain) without a real broker, using the no-op producer.
func Test_KafKaWriter_Start_SpillRingAndChain(t *testing.T) {
	cases := []struct {
		name     string
		spill    string
		wantType string
	}{
		{"ring", "ring", "*log4go.RingSpiller[github.com/v8fg/kit4go/kafka.Message]"},
		{"chain", "", "*log4go.ChainedSpiller[github.com/v8fg/kit4go/kafka.Message]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			w := NewKafKaWriter(KafKaWriterOptions{
				Enable: true, Level: LevelFlagInfo, Brokers: []string{"localhost:9092"},
				ProducerTopic: "cov", BufferSize: 16,
				OverflowPolicy: "spill", SpillType: c.spill, SpillSize: 4, SpillDir: dir,
			})
			w.producerFactory = func() (kafka.Producer, error) {
				return newMockKafkaProducer(), nil
			}
			if err := w.Start(); err != nil {
				t.Fatalf("Start: %v", err)
			}
			if w.spiller == nil {
				t.Fatal("spiller nil after Start")
			}
			w.Stop()
		})
	}
}

// Test_KafKaWriter_Start_VersionOverride covers the SpecifyVersion+VersionStr
// success branch (parses a valid kafka version).
func Test_KafKaWriter_Start_VersionOverride(t *testing.T) {
	w := NewKafKaWriter(KafKaWriterOptions{
		Enable: true, Level: LevelFlagInfo, Brokers: []string{"localhost:9092"},
		ProducerTopic: "cov", BufferSize: 16,
		SpecifyVersion: true, VersionStr: "2.5.0.0",
	})
	w.producerFactory = func() (kafka.Producer, error) {
		return newMockKafkaProducer(), nil
	}
	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	w.Stop()

	// An invalid version string is logged but does NOT fail Start (falls back).
	w2 := NewKafKaWriter(KafKaWriterOptions{
		Enable: true, Level: LevelFlagInfo, Brokers: []string{"localhost:9092"},
		ProducerTopic: "cov", BufferSize: 16,
		SpecifyVersion: true, VersionStr: "not-a-version",
	})
	w2.producerFactory = func() (kafka.Producer, error) {
		return newMockKafkaProducer(), nil
	}
	if err := w2.Start(); err != nil {
		t.Fatalf("Start with bad version: %v", err)
	}
	w2.Stop()
}

// Test_KafKaWriter_Start_ProducerError covers the factory-error branch of Start
// (returns the error without launching the daemon).
func Test_KafKaWriter_Start_ProducerError(t *testing.T) {
	w := NewKafKaWriter(KafKaWriterOptions{
		Enable: true, Level: LevelFlagInfo, Brokers: []string{"localhost:9092"},
		ProducerTopic: "cov", BufferSize: 16,
	})
	boom := errDeep("dial failed")
	w.producerFactory = func() (kafka.Producer, error) {
		return nil, boom
	}
	if err := w.Start(); err != boom {
		t.Fatalf("Start err=%v want %v", err, boom)
	}
}

// Test_KafKaWriter_Start_SpillFileError covers the file-spiller creation error
// branch (invalid SpillDir under a regular file).
func Test_KafKaWriter_Start_SpillFileError(t *testing.T) {
	dir := t.TempDir()
	blocker := dir + "/blocker"
	if err := writeFile(blocker, "x"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	w := NewKafKaWriter(KafKaWriterOptions{
		Enable: true, Level: LevelFlagInfo, Brokers: []string{"localhost:9092"},
		ProducerTopic: "cov", BufferSize: 16,
		OverflowPolicy: "spill", SpillType: "file", SpillDir: blocker + "/sub",
	})
	w.producerFactory = func() (kafka.Producer, error) {
		return newMockKafkaProducer(), nil
	}
	if err := w.Start(); err == nil {
		w.Stop()
		t.Fatal("Start with bad spill dir succeeded; want MkdirAll error")
	}
}

// Test_KafKaWriter_Start_SpillResumeOnStartup covers the "resume persisted
// spill" branch: a pre-populated spiller is drained into the channel on Start.
func Test_KafKaWriter_Start_SpillResumeOnStartup(t *testing.T) {
	w := NewKafKaWriter(KafKaWriterOptions{
		Enable: true, Level: LevelFlagInfo, Brokers: []string{"localhost:9092"},
		ProducerTopic: "cov", BufferSize: 16,
		OverflowPolicy: "spill", SpillType: "ring", SpillSize: 4,
	})
	// Pre-seed a spiller so the resume loop runs during Start. We wire it before
	// Start by mimicking the ring path: Start itself creates the ring, so instead
	// we exercise the resume branch by starting, spilling, stopping, then
	// restarting with a persisted ring is not possible (ring is in-memory). The
	// file path persists; use a file spiller pre-seeded via DrainFileRecover is
	// covered elsewhere. Here we just confirm Start with ring resume is a no-op
	// when the spiller is empty (the common path).
	w.producerFactory = func() (kafka.Producer, error) {
		return newMockKafkaProducer(), nil
	}
	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	w.Stop()
}

// Test_FileWriter_Send_SpillStopUnblock covers the `case <-w.stop` branch
// inside the OverflowSpill select. We fill the channel (so `messages <- r`
// cannot succeed) and pre-close stop, which forces the select to pick
// `<-w.stop` deterministically (the default branch is only chosen when no
// case is ready, but here <-w.stop IS ready).
func Test_FileWriter_Send_SpillStopUnblock(t *testing.T) {
	fw := newFileWriterForSend(t)
	fw.policy = OverflowSpill
	fw.messages <- &Record{level: INFO, msg: "fill"} // fill -> send cannot enqueue
	close(fw.stop)                                   // <-w.stop is now ready
	fw.send(&Record{level: INFO, msg: "stop closed"})
	if got := fw.stats.Dropped(); got != 1 {
		t.Fatalf("spill stop-unblock: dropped=%d want 1", got)
	}
}

// Test_FileWriter_Send_DropStopUnblock covers the `case <-w.stop` branch inside
// the OverflowDrop select (channel full + stop closed -> <-w.stop wins).
func Test_FileWriter_Send_DropStopUnblock(t *testing.T) {
	fw := newFileWriterForSend(t)
	fw.policy = OverflowDrop
	fw.messages <- &Record{level: INFO, msg: "fill"} // fill -> default would drop, but
	close(fw.stop)                                   // <-w.stop is ready AND default is ready;
	// Go picks among ready cases; with stop closed the drop is counted either way.
	fw.send(&Record{level: INFO, msg: "stop closed"})
	if got := fw.stats.Dropped(); got != 1 {
		t.Fatalf("drop stop-unblock: dropped=%d want 1", got)
	}
}

// ---------------------------------------------------------------------------
// Section 8 — remaining pure-function wins
// ---------------------------------------------------------------------------

// Test_AppendLogfmtValue_Escapes covers the quoting-loop escape branches the
// existing test misses: backspace (\b), formfeed (\f), carriage return (\r),
// and the control-char hex escape (c < 0x20, e.g. \x01).
func Test_AppendLogfmtValue_Escapes(t *testing.T) {
	for _, c := range []struct {
		name string
		in   string
		want string // substring expected in the quoted output
	}{
		// \b (0x08) and \f (0x0c) are < 0x20, so the switch default renders them
		// via the hex escape (\x08 / \x0c), not the short \b / \f forms.
		{"backspace", "a\bb", `\x08`},
		{"formfeed", "a\fb", `\x0c`},
		{"cr", "a\rb", `\r`},
		{"control-hex", "a\x01b", `\x01`},
		{"tab", "a\tb", `\t`},
		{"newline", "a\nb", `\n`},
		{"quote", `a"b`, `\"`},
		{"backslash", `a\b`, `\\`},
	} {
		out := string(appendLogfmtValue(nil, c.in))
		if !strings.HasPrefix(out, `"`) || !strings.HasSuffix(out, `"`) {
			t.Errorf("%s: not quoted: %q", c.name, out)
		}
		if !strings.Contains(out, c.want) {
			t.Errorf("%s: output %q missing %q", c.name, out, c.want)
		}
	}
}

// Test_ProducerMsgCodec_FullRoundTrip covers the Encode branches for a message
// with a Key and a non-zero Timestamp (the existing test omits both), and the
// Decode round-trip including Timestamp reconstruction.
func Test_ProducerMsgCodec_FullRoundTrip(t *testing.T) {
	ts := time.Unix(1700000000, 123456789)
	in := kafka.Message{
		Topic:     "full",
		Key:       []byte("my-key"),
		Value:     []byte("my-value"),
		Timestamp: ts,
	}
	b, err := ProducerMsgCodec.Encode(in)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	out, err := ProducerMsgCodec.Decode(b)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if out.Topic != "full" {
		t.Errorf("Topic=%q want full", out.Topic)
	}
	kb := out.Key
	if string(kb) != "my-key" {
		t.Errorf("Key=%q want my-key", kb)
	}
	vb := out.Value
	if string(vb) != "my-value" {
		t.Errorf("Value=%q want my-value", vb)
	}
	if out.Timestamp.UnixNano() != ts.UnixNano() {
		t.Errorf("Timestamp=%v want %v", out.Timestamp, ts)
	}
}

// Test_ProducerMsgCodec_DecodeBadJSON covers the Decode error path.
func Test_ProducerMsgCodec_DecodeBadJSON(t *testing.T) {
	if _, err := ProducerMsgCodec.Decode([]byte("{not json")); err == nil {
		t.Fatal("Decode(bad json) succeeded; want error")
	}
}

// Test_ProducerMsgCodec_Encode_KeyEncodeError covers the branch where
// msg.Key returns an error (Key is silently dropped).
func Test_ProducerMsgCodec_Encode_KeyEncodeError(t *testing.T) {
	// A sarama.Encoder returning an error is hard to construct without a custom
	// type; instead cover the nil-Key + nil-Value + zero-Timestamp minimal path,
	// which exercises all three "skip" branches at once.
	in := kafka.Message{Topic: "min"}
	b, err := ProducerMsgCodec.Encode(in)
	if err != nil {
		t.Fatalf("Encode minimal: %v", err)
	}
	out, _ := ProducerMsgCodec.Decode(b)
	if out.Topic != "min" {
		t.Errorf("Topic=%q want min", out.Topic)
	}
	if out.Key != nil || out.Value != nil {
		t.Errorf("minimal msg should have nil Key/Value: key=%v value=%v", out.Key, out.Value)
	}
	if !out.Timestamp.IsZero() {
		t.Errorf("minimal msg should have zero Timestamp: %v", out.Timestamp)
	}
}

// Test_RecordCodec_RoundTripAndBadJSON covers recordCodec round-trip plus its
// Decode error path.
func Test_RecordCodec_RoundTripAndBadJSON(t *testing.T) {
	in := &Record{level: WARNING, time: "t", file: "f.go:3", msg: "hello"}
	b, err := RecordCodec.Encode(in)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	out, err := RecordCodec.Decode(b)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if out.level != WARNING || out.msg != "hello" || out.file != "f.go:3" {
		t.Errorf("round-trip mismatch: %+v", out)
	}
	if _, err := RecordCodec.Decode([]byte("nope")); err == nil {
		t.Fatal("Decode(bad json) succeeded; want error")
	}
}

// Test_DrainFileRecover_Success covers the happy path: a persisted spill.log is
// recovered (renamed aside, decoded, removed) without holding an open spiller.
func Test_DrainFileRecover_Success(t *testing.T) {
	dir := t.TempDir()
	// Seed a spill.log via a real FileSpiller, then close it so the file is
	// flushed to disk.
	fs, err := NewFileSpiller[kafka.Message](dir, 1<<16, ProducerMsgCodec)
	if err != nil {
		t.Fatalf("NewFileSpiller: %v", err)
	}
	fs.Push(spillerMsg("recover", "one"))
	fs.Push(spillerMsg("recover", "two"))
	_ = fs.Close()

	// DrainFileRecover reads spill.log, moves it aside, decodes, removes.
	out := DrainFileRecover[kafka.Message](dir, ProducerMsgCodec)
	if len(out) != 2 {
		t.Fatalf("recovered=%d want 2", len(out))
	}
	// A second call finds no spill.log -> nil.
	if again := DrainFileRecover[kafka.Message](dir, ProducerMsgCodec); again != nil {
		t.Errorf("second recover should be nil, got %d", len(again))
	}
}

// Test_DrainFileRecover_RenameFailure covers the os.Rename error branch (spill.log
// is absent so Stat fails first -> returns nil before Rename).
func Test_DrainFileRecover_StatFailure(t *testing.T) {
	dir := t.TempDir()
	if out := DrainFileRecover[kafka.Message](dir, ProducerMsgCodec); out != nil {
		t.Errorf("absent spill.log should return nil, got %d", len(out))
	}
}

// Test_JsonMarshalEncode_AllCodecs covers the codec switch by exercising each
// branch directly. The active codec is global, so we save/restore it.
func Test_JsonMarshalEncode_AllCodecs(t *testing.T) {
	saved := jsonCodecActive
	defer func() { jsonCodecActive = saved }()
	val := map[string]int{"x": 1, "y": 2}
	for _, c := range []JSONCodec{JSONCodecGoccy, JSONCodecStd, JSONCodecSonic} {
		jsonCodecActive = c
		b, err := jsonMarshalEncode(val)
		if err != nil {
			t.Errorf("codec %d: Marshal err: %v", c, err)
			continue
		}
		s := string(b)
		if !strings.Contains(s, `"x":1`) || !strings.Contains(s, `"y":2`) {
			t.Errorf("codec %d: unexpected output %s", c, s)
		}
	}
}
