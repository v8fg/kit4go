package log4go

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/v8fg/kit4go/kafka"
)

type errStr string

func (e errStr) Error() string { return string(e) }

func Test_FieldConstructors_All(t *testing.T) {
	cases := []struct {
		name string
		f    Field
	}{
		{"String", String("k", "v")}, {"Int", Int("k", 42)},
		{"Int64", Int64("k", 99)}, {"Uint64", Uint64("k", 7)},
		{"Bool", Bool("k", true)}, {"Float64", Float64("k", 1.5)},
		{"Duration", Duration("k", 5*time.Second)}, {"Time", Time("k", time.Unix(1e9, 0))},
		{"Bytes", Bytes("k", []byte("abc"))}, {"ErrorField", ErrorField("k", errStr("b"))},
		{"Any", Any("k", 42)}, {"Complex128", Complex128("k", 1+2i)}, {"Complex64", Complex64("k", 3+4i)},
	}
	for _, c := range cases {
		if c.f.Key() != "k" {
			t.Errorf("%s Key()=%q", c.name, c.f.Key())
		}
		_ = c.f.Kind()
		_ = c.f.Value()
	}
}

func Test_Logger_TypedWith_All(t *testing.T) {
	lg := newLoggerWithRecords(make(chan *Record, 8))
	defer lg.Close()
	lg.SetLevel(TRACE)
	cw := &captureWriter{}
	lg.Register(cw)
	lg.WithString("s", "v").WithInt("i", 1).WithInt64("i64", 2).
		WithUint64("u", 3).WithBool("b", true).WithFloat64("f", 1.5).
		WithDuration("d", time.Millisecond).WithTime("t", time.Unix(1e9, 0)).
		WithBytes("by", []byte("x")).WithError("e", errStr("err")).Trace("chain")
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && cw.Len() == 0 {
		runtime.Gosched()
	}
	if cw.Len() == 0 {
		t.Fatal("no record")
	}
}

func Test_PackageLevel_TypedWith(t *testing.T) {
	_ = WithString("k", "v")
	_ = WithInt("k", 1)
	_ = WithInt64("k", 1)
	_ = WithBool("k", true)
	_ = WithFloat64("k", 1.0)
	_ = WithDuration("k", time.Second)
	_ = WithTime("k", time.Now())
	_ = WithBytes("k", []byte("x"))
	_ = WithError("k", errStr("e"))
	_ = WithAttrs(String("k", "v"), Int("n", 1))
	_ = WithSampling(10, 100)
	_ = With("k", "v")
	_ = WithField("k2", 2)
	_ = WithFields(map[string]any{"k3": "v3"})
}

func Test_PackageLevel_Trace(t *testing.T) { Trace("pkg trace %d", 1) }

func Test_PackageLevel_Format(t *testing.T) {
	SetFormat(FormatJSON)
	if Format() != FormatJSON {
		t.Error("Format mismatch")
	}
	SetFormat(FormatText)
}

func Test_PackageLevel_SetContextExtractor(t *testing.T) {
	SetContextExtractor(func(_ context.Context) map[string]any { return nil })
}

func Test_PackageLevel_Panic(t *testing.T) {
	// Panic calls Sync (=Close) internally, so don't Close again after.
	lg := newLoggerWithRecords(make(chan *Record, 4))
	lg.SetLevel(EMERGENCY) // suppress output
	func() {
		defer func() { _ = recover() }()
		lg.Panic("pkg-style panic %d", 1)
	}()
	// lg already closed by Sync inside Panic
}

func Test_Logger_SetBaseField(t *testing.T) {
	lg := newLoggerWithRecords(make(chan *Record, 4))
	defer lg.Close()
	lg.SetBaseField("a", 1)
	lg.SetBaseField("b", "x")
	lg.SetLevel(DEBUG)
	cw := &captureWriter{}
	lg.Register(cw)
	lg.Info("test")
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && cw.Len() == 0 {
		runtime.Gosched()
	}
	if cw.Len() == 0 {
		t.Fatal("no record")
	}
}

func Test_Trace_Filtered(t *testing.T) {
	lg := newLoggerWithRecords(make(chan *Record, 4))
	defer lg.Close()
	lg.SetLevel(DEBUG)
	cw := &captureWriter{}
	lg.Register(cw)
	lg.Trace("filtered")
	time.Sleep(50 * time.Millisecond)
	if cw.Len() > 0 {
		t.Error("TRACE should be filtered")
	}
}

func Test_ColorRecord_AllLevels(t *testing.T) {
	for lvl := EMERGENCY; lvl <= TRACE; lvl++ {
		r := &colorRecord{time: "t", level: lvl, file: "f.go:1", msg: "m"}
		if !strings.Contains(r.String(), LevelFlags[lvl]) {
			t.Errorf("level %d String missing flag", lvl)
		}
		_ = r.ColorString()
	}
}

func Test_WebhookWriter_Setters(t *testing.T) {
	sink := &mockAlertSink{}
	w := NewWebhookWriter(sink, WebhookWriterOptions{Level: "error"})
	w.SetFilter(func(r *Record) bool { return true })
	w.SetGate(NewRateAlerter(time.Second, 1))
	w.SetRateFormatter(func(r *Record, ctx WebhookContext) (string, string) { return "k", "v" })
	_ = w.Write(&Record{level: ERROR, msg: "test"})
}

func Test_SetupLog(t *testing.T) {
	// Swap out singleton, test SetupLog, then close
	old := loggerDefault.Swap(nil)
	if old != nil {
		old.Close()
	}
	err := SetupLog(LogConfig{Level: LevelFlagInfo, Format: "json",
		ConsoleWriter: ConsoleWriterOptions{Enable: false}})
	if err != nil {
		t.Fatalf("SetupLog: %v", err)
	}
	Close()
}

func Test_ShardLogger_Trace(t *testing.T) {
	s := NewShardLogger(2)
	defer s.Close()
	s.SetLevel(TRACE)
	s.Trace("trace %d", 1)
}

func Test_SlogHandler_WithGroup(t *testing.T) {
	root := newLoggerWithRecords(make(chan *Record, 4))
	defer root.Close()
	root.SetLevel(DEBUG)
	cw := &captureWriter{}
	root.Register(cw)
	slog.New(NewSlogHandler(root)).WithGroup("g1").Info("grp", "id", 1)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && cw.Len() == 0 {
		runtime.Gosched()
	}
	if cw.Len() == 0 {
		t.Fatal("no record")
	}
}

func Test_JSONCodec_All(t *testing.T) {
	orig := GetJSONCodec()
	defer SetJSONCodec(orig)
	for _, codec := range []JSONCodec{JSONCodecGoccy, JSONCodecStd, JSONCodecSonic} {
		SetJSONCodec(codec)
		r := &Record{level: INFO, msg: "codec", unixNano: 1700000000_000000000,
			fields: []field{strField("k", "v")}}
		var m map[string]any
		if err := json.Unmarshal(r.JSON(), &m); err != nil {
			t.Errorf("codec %d: %v", codec, err)
		}
	}
}

func Test_FieldOf_Exhaustive(t *testing.T) {
	cases := []struct {
		val  any
		kind fieldKind
	}{
		{nil, kindAny}, {"s", kindString}, {true, kindBool},
		{int(1), kindInt}, {int64(1), kindInt64}, {int32(1), kindInt64},
		{int16(1), kindInt64}, {int8(1), kindInt64},
		{uint(1), kindUint}, {uint64(1), kindUint}, {uint32(1), kindUint},
		{uint16(1), kindUint}, {uint8(1), kindUint}, {uintptr(1), kindUint},
		{float64(1.5), kindFloat64}, {float32(1.5), kindFloat64},
		{time.Second, kindDuration}, {time.Unix(1, 0), kindTime},
		{[]byte("x"), kindBytes}, {complex128(1 + 2i), kindString},
		{complex64(3 + 4i), kindString}, {errStr("e"), kindError},
		{[]int{1}, kindAny},
	}
	for _, c := range cases {
		if f := fieldOf("k", c.val); f.kind != c.kind {
			t.Errorf("fieldOf(%T)=%v want %v", c.val, f.kind, c.kind)
		}
	}
}

func Test_Logger_FatalComponents(t *testing.T) {
	lg := newLoggerWithRecords(make(chan *Record, 4))
	cw := &captureWriter{}
	lg.Register(cw)
	lg.SetLevel(DEBUG)
	lg.deliverRecordToWriter(CRITICAL, "fatal %d", 42)
	lg.Sync()
	if cw.Len() == 0 {
		t.Fatal("CRITICAL not delivered")
	}
}

func Test_DrainFileRecover_EmptyDir(t *testing.T) {
	recovered := DrainFileRecover(t.TempDir(), producerMsgCodec{})
	if len(recovered) != 0 {
		t.Errorf("got %d want 0", len(recovered))
	}
}

func Test_RecordAlertLevel_All(t *testing.T) {
	if recordAlertLevel(EMERGENCY) != AlertError || recordAlertLevel(ERROR) != AlertError ||
		recordAlertLevel(WARNING) != AlertWarn || recordAlertLevel(INFO) != AlertInfo {
		t.Error("recordAlertLevel mapping wrong")
	}
}

func Test_Logger_CallerOptions(t *testing.T) {
	lg := newLoggerWithRecords(make(chan *Record, 4))
	defer lg.Close()
	lg.SetLevel(DEBUG)
	lg.WithCaller(true)
	lg.WithFullPath(true)
	lg.WithFuncName(true)
	cw := &captureWriter{}
	lg.Register(cw)
	lg.Info("caller")
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && cw.Len() == 0 {
		runtime.Gosched()
	}
	if cw.Len() == 0 {
		t.Fatal("no record")
	}
}

func Test_Logger_NoCaller(t *testing.T) {
	lg := newLoggerWithRecords(make(chan *Record, 4))
	defer lg.Close()
	lg.SetLevel(DEBUG)
	lg.WithCaller(false)
	cw := &captureWriter{}
	lg.Register(cw)
	lg.Info("nc")
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && cw.Len() == 0 {
		runtime.Gosched()
	}
	if cw.Len() > 0 {
		cw.mu.Lock()
		r := cw.records[0]
		cw.mu.Unlock()
		if r.file != "" {
			t.Errorf("file=%q want empty", r.file)
		}
	}
}

func Test_Logger_SetLevel_SetLayout(t *testing.T) {
	lg := newLoggerWithRecords(make(chan *Record, 4))
	defer lg.Close()
	lg.SetLevel(ERROR)
	if int32(ERROR) != lg.level.Load() {
		t.Error("SetLevel failed")
	}
	lg.SetLayout("2006-01-02 15:04:05.000")
	if !lg.hasSubSecond.Load() {
		t.Error("hasSubSecond should be true")
	}
}

func Test_AppendFieldJSON_AllKinds(t *testing.T) {
	fields := []field{
		strField("s", "v"), intField("i", 42), int64Field("i64", 99),
		uint64Field("u", 7), boolField("b", true), floatField("f", 1.5),
		durField("d", time.Second), timeField("t", time.Unix(1e9, 0)),
		errField("e", errStr("b")), anyField("a", map[string]int{"x": 1}),
		bytesField("by", []byte("abc")),
	}
	buf := appendFieldsJSONObject([]byte{}, fields)
	var m map[string]json.RawMessage
	if err := json.Unmarshal(buf, &m); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
}

func Test_AppendFieldLogfmt_AllKinds(t *testing.T) {
	fields := []field{
		strField("s", "v"), intField("i", 42), boolField("b", true),
		floatField("f", 1.5), durField("d", time.Second), errField("e", errStr("b")),
	}
	var buf []byte
	for _, f := range fields {
		buf = appendFieldLogfmt(buf, f)
	}
	if !strings.Contains(string(buf), "s=v") || !strings.Contains(string(buf), "i=42") {
		t.Errorf("logfmt output wrong: %s", buf)
	}
}

func Test_DefaultWebhookFormatter_NoFile(t *testing.T) {
	k, text := DefaultWebhookFormatter(&Record{level: ERROR, time: "t", msg: "m"})
	if k != "ERROR" || !strings.Contains(text, "m") {
		t.Errorf("k=%q text=%q", k, text)
	}
}

func Test_DefaultRateWebhookFormatter(t *testing.T) {
	_, withCount := DefaultRateWebhookFormatter(&Record{level: ERROR, msg: "m"}, WebhookContext{RateCount: 5})
	if !strings.Contains(withCount, "[5 in window]") {
		t.Error("missing prefix")
	}
	_, zeroCount := DefaultRateWebhookFormatter(&Record{level: INFO, msg: "m"}, WebhookContext{RateCount: 0})
	if strings.Contains(zeroCount, "in window") {
		t.Error("zero should not prefix")
	}
}

// ===================== Instance SetBaseField (single) =====================

func Test_Logger_SetBaseField_Single(t *testing.T) {
	lg := newLoggerWithRecords(make(chan *Record, 4))
	defer lg.Close()
	lg.SetBaseField("key1", "val1")
	lg.SetLevel(DEBUG)
	cw := &captureWriter{}
	lg.Register(cw)
	lg.Info("test")
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && cw.Len() == 0 {
		runtime.Gosched()
	}
	if cw.Len() == 0 {
		t.Fatal("no record")
	}
	cw.mu.Lock()
	r := cw.records[0]
	cw.mu.Unlock()
	found := false
	for _, f := range r.fields {
		if f.key == "key1" && f.value() == "val1" {
			found = true
		}
	}
	if !found {
		t.Error("SetBaseField key1 not found")
	}
}

// ===================== Package-level WithContext =====================

func Test_PackageLevel_WithContext(t *testing.T) {
	// Just verify no panic; WithContext needs a context extractor set
	l := WithContext(context.Background())
	_ = l
}

// ===================== KafkaWriter Init with mock =====================

func Test_KafkaWriter_Init(t *testing.T) {
	w := NewKafkaWriter(KafkaWriterOptions{
		Enable:         true,
		Level:          LevelFlagInfo,
		ProducerTopic:  "t",
		BufferSize:     1024,
		OverflowPolicy: "drop",
	})
	w.producerFactory = func() (kafka.Producer, error) {
		return newMockKafkaProducer(), nil
	}
	if err := w.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	w.Stop()
}

func Test_NetWriter_String(t *testing.T) {
	w := NewNetWriter(NetWriterOptions{Network: "tcp", Address: "localhost:9999", Level: "info"})
	_ = w.Metrics()
}

// ===================== Package-level SetBaseField / SetBaseField =====================

func Test_PackageLevel_SetBaseField(t *testing.T) {
	SetBaseField("pkg_key", "pkg_val")
	SetBaseField("pk1", 1)
	SetBaseField("pk2", "v")
}

// ===================== NetWriter String() =====================

func Test_NetWriter_StringMethod(t *testing.T) {
	w := NewNetWriter(NetWriterOptions{Network: "tcp", Address: "127.0.0.1:9999", Level: "info"})
	s := w.String()
	if !strings.Contains(s, "tcp") || !strings.Contains(s, "127.0.0.1") {
		t.Errorf("String() = %q", s)
	}
}

// ===================== SetLogWithConf (config file) =====================

func Test_SetLogWithConf(t *testing.T) {
	dir := t.TempDir()
	cfgPath := dir + "/log.json"
	cfgContent := []byte(`{"level":"INFO","format":"json","console_writer":{"enable":false}}`)
	if err := os.WriteFile(cfgPath, cfgContent, 0644); err != nil {
		t.Fatal(err)
	}
	old := loggerDefault.Swap(nil)
	if old != nil {
		old.Close()
	}
	if err := SetLogWithConf(cfgPath); err != nil {
		t.Fatalf("SetLogWithConf: %v", err)
	}
	Close()
}

// ===================== sonicMarshal direct =====================

func Test_SonicMarshal(t *testing.T) {
	b, err := sonicMarshal(map[string]int{"x": 1})
	if err != nil {
		t.Fatalf("sonicMarshal: %v", err)
	}
	if !strings.Contains(string(b), `"x":1`) {
		t.Errorf("sonicMarshal output: %s", b)
	}
}

// ===================== Package-level Panic (safe) =====================

func Test_PackageLevel_Panic_Safe(t *testing.T) {
	// Replace singleton with our own logger, capture the panic
	lg := newLoggerWithRecords(make(chan *Record, 4))
	lg.SetLevel(EMERGENCY)
	loggerDefault.Store(lg)                   // overwrite, don't save old — Panic kills the logger
	defer loggerDefault.Store((*Logger)(nil)) // force rebuild on next use
	func() {
		defer func() {
			r := recover()
			if r == nil {
				t.Error("expected panic")
			}
		}()
		Panic("pkg panic test")
	}()
}
