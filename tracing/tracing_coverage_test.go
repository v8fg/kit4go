package tracing

import (
	"context"
	"os"
	"strings"
	"testing"
)

// Exercise the WithStdout + New branches without a live OTLP collector.

func TestNew_WithStdout(t *testing.T) {
	var buf strings.Builder
	p, err := New(
		WithServiceName("test-svc"),
		WithStdout(&buf),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if p == nil {
		t.Fatal("nil provider")
	}
	// Create a span to exercise the exporter.
	tracer := p.Tracer("test")
	_, span := tracer.Start(context.Background(), "test-span")
	span.End()
	// Flush.
	_ = p.ForceFlush(context.Background())
	_ = p.Shutdown(context.Background())
}

func TestNew_WithStdoutNilWriter(t *testing.T) {
	// nil writer should default to os.Stderr (the branch where c.stdoutW is nil
	// after WithStdout(nil)).
	p, err := New(WithStdout(nil))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_ = p.Shutdown(context.Background())
	_ = os.Stderr // referenced to ensure the default path compiles
}

func TestNew_WithExporter(t *testing.T) {
	// A nil exporter should still work (the explicit-exporter path).
	p, err := New(WithExporter(nil))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_ = p.Shutdown(context.Background())
}

func TestNew_DefaultOptions(t *testing.T) {
	p, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if p == nil {
		t.Fatal("nil provider")
	}
	_ = p.Shutdown(context.Background())
}
