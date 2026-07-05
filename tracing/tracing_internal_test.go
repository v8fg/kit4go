package tracing

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
)

// TestNew_StdoutNilInnerWriter covers the inner `w == nil -> os.Stderr` fallback
// in New (lines 80-82). The public WithStdout option always sets stdoutW (to the
// given writer or os.Stderr), so the only way to reach this branch is to set
// config.stdout=true while leaving stdoutW nil — which an external caller cannot
// do because config is unexported. We reach it via a raw Option in the package's
// own test build.
func TestNew_StdoutNilInnerWriter(t *testing.T) {
	p, err := New(
		WithServiceName("inner-nil-writer"),
		// Force stdout=true with stdoutW left nil: exercises the defensive
		// `w = os.Stderr` fallback inside New.
		func(c *config) {
			c.stdout = true
			c.stdoutW = nil
		},
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if p == nil {
		t.Fatal("nil provider")
	}
	// Emit a span so the stdout exporter actually writes something (proving the
	// fallback writer is wired and usable).
	tr := p.Tracer("test")
	_, span := tr.Start(context.Background(), "inner-nil-writer-span")
	span.End()
	if err := p.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush: %v", err)
	}
	if err := p.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

// TestNoopExporter exercises the noopExporter methods directly so they register
// as covered even if the SDK never calls them (e.g. Shutdown on a provider whose
// batcher was already shut down).
func TestNoopExporter(t *testing.T) {
	n := noopExporter{}
	if err := n.ExportSpans(context.Background(), nil); err != nil {
		t.Fatalf("ExportSpans: %v", err)
	}
	if err := n.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

// failingMeter embeds the noop meter and overrides instrument creators to
// always return an error. Used (with the OTEL_GO_X_OBSERVABILITY feature flag
// enabled) to force stdouttrace.New to fail — covering New's stdout-exporter
// error branch.
type failingMeter struct{ noop.Meter }

func (failingMeter) Int64UpDownCounter(string, ...metric.Int64UpDownCounterOption) (metric.Int64UpDownCounter, error) {
	return nil, errors.New("meter unavailable")
}

type failingMeterProvider struct{ noop.MeterProvider }

func (failingMeterProvider) Meter(string, ...metric.MeterOption) metric.Meter {
	return failingMeter{}
}

// TestNew_StdoutExporterError covers the stdouttrace.New error branch in New.
// We enable the OTel experimental self-observability feature (which makes
// stdouttrace.New consult the global MeterProvider) and install a meter provider
// whose instrument creation always fails. New must then wrap and return the
// exporter error instead of producing a Provider.
//
// Global state is restored via t.Setenv (env) and a deferred swap of the global
// MeterProvider so this test cannot leak into siblings.
func TestNew_StdoutExporterError(t *testing.T) {
	t.Setenv("OTEL_GO_X_OBSERVABILITY", "true")

	prev := otel.GetMeterProvider()
	otel.SetMeterProvider(failingMeterProvider{})
	t.Cleanup(func() { otel.SetMeterProvider(prev) })

	_, err := New(
		WithServiceName("stdout-err"),
		WithStdout(nil), // forces the stdout exporter construction path
	)
	if err == nil {
		t.Skip("stdouttrace.New did not error on this SDK build; cannot cover error branch")
	}
	if !strings.Contains(err.Error(), "stdout exporter") {
		t.Fatalf("expected wrapped 'stdout exporter' error, got %v", err)
	}
}
