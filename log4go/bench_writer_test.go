package log4go

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func benchRecord() *Record {
	return &Record{level: INFO, time: "2026-06-25 00:00:00", file: "bench_test.go:1", msg: "benchmark writer message payload"}
}

// Benchmark_ConsoleWriter_Write measures console write throughput with stdout
// redirected to a pipe drained to io.Discard (no terminal blocking).
func Benchmark_ConsoleWriter_Write(b *testing.B) {
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		b.Fatal(err)
	}
	os.Stdout = w
	done := make(chan struct{})
	go func() { _, _ = io.Copy(io.Discard, r); close(done) }()
	defer func() {
		os.Stdout = orig
		_ = w.Close()
		<-done
	}()

	cw := NewConsoleWriterWithOptions(ConsoleWriterOptions{Level: LevelFlagInfo})
	rec := benchRecord()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := cw.Write(rec); err != nil {
			b.Fatal(err)
		}
	}
}

// Benchmark_ConsoleWriter_WriteColor measures console write with color enabled.
func Benchmark_ConsoleWriter_WriteColor(b *testing.B) {
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		b.Fatal(err)
	}
	os.Stdout = w
	done := make(chan struct{})
	go func() { _, _ = io.Copy(io.Discard, r); close(done) }()
	defer func() {
		os.Stdout = orig
		_ = w.Close()
		<-done
	}()

	cw := NewConsoleWriterWithOptions(ConsoleWriterOptions{Level: LevelFlagInfo, Color: true})
	rec := benchRecord()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cw.Write(rec)
	}
}

// Benchmark_FileWriter_Write measures file write throughput (bufio buffered).
func Benchmark_FileWriter_Write(b *testing.B) {
	f := NewFileWriterWithOptions(FileWriterOptions{
		Filename: filepath.Join(b.TempDir(), "bench-%Y%M%D.log"),
		Rotate:   true, Daily: true, MaxDays: 60,
	})
	if err := f.Init(); err != nil {
		b.Fatal(err)
	}
	defer func() { _ = f.Flush() }()
	rec := benchRecord()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := f.Write(rec); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	_ = f.Flush()
}
