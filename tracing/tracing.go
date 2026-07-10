// Package tracing wraps OpenTelemetry Go SDK initialization: one-call setup of a
// TracerProvider with service name, sampler, and exporter — plus convenience span
// helpers. Own module so the otel dependency graph stays isolated.
//
// Ad-tech uses: trace each bid request end-to-end (ingress → enrichment →
// decision → log) across services. Pair with kit4go/metrics for a complete
// observability stack (traces + metrics).
package tracing

import (
	"context"
	"fmt"
	"io"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// Provider wraps a TracerProvider for clean lifecycle. (The exporter is owned
// by the SDK's TracerProvider — Shutdown/ForceFlush delegate to it — so the
// Provider does not hold a separate exporter reference.)
type Provider struct {
	tp *sdktrace.TracerProvider
}

// Option configures the Provider.
type Option func(*config)

type config struct {
	serviceName string
	sampler     sdktrace.Sampler
	exporter    sdktrace.SpanExporter
	stdout      bool
	stdoutW     io.Writer
}

// WithServiceName sets the service name attribute on all spans.
func WithServiceName(name string) Option { return func(c *config) { c.serviceName = name } }

// WithSampler sets the trace sampler (default AlwaysSample for dev; use
// ParentBased(TraceIDRatioBased(r)) in prod to control volume).
func WithSampler(s sdktrace.Sampler) Option { return func(c *config) { c.sampler = s } }

// WithExporter injects a custom span exporter (e.g. OTLP, or tracetest for tests).
func WithExporter(e sdktrace.SpanExporter) Option { return func(c *config) { c.exporter = e } }

// WithStdout enables a stdout exporter for development (writes to w, or os.Stderr
// if nil). Overridden by an explicit WithExporter.
func WithStdout(w io.Writer) Option {
	return func(c *config) {
		c.stdout = true
		if w != nil {
			c.stdoutW = w
		} else {
			c.stdoutW = os.Stderr
		}
	}
}

// New builds a Provider, sets it as the global TracerProvider, and returns it.
// Call Shutdown on graceful exit to flush pending spans.
func New(opts ...Option) (*Provider, error) {
	c := config{
		serviceName: "kit4go-app",
		sampler:     sdktrace.AlwaysSample(),
	}
	for _, opt := range opts {
		opt(&c)
	}

	var exporter sdktrace.SpanExporter
	if c.exporter != nil {
		exporter = c.exporter
	} else if c.stdout {
		w := c.stdoutW
		if w == nil {
			w = os.Stderr
		}
		e, err := stdouttrace.New(stdouttrace.WithWriter(w))
		if err != nil {
			return nil, fmt.Errorf("tracing: stdout exporter: %w", err)
		}
		exporter = e
	} else {
		// No exporter configured — use a no-op. Spans are recorded but go nowhere.
		exporter = noopExporter{}
	}

	// Merge service-name into the default resource. NewSchemaless avoids the
	// schema-URL conflict that NewWithAttributes can trigger against the SDK's
	// default resource.
	res, err := resource.Merge(
		resource.Default(),
		resource.NewSchemaless(semconv.ServiceName(c.serviceName)),
	)
	if err != nil {
		return nil, fmt.Errorf("tracing: resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(c.sampler),
	)
	otel.SetTracerProvider(tp)

	return &Provider{tp: tp}, nil
}

// Tracer returns a named tracer from the provider.
func (p *Provider) Tracer(name string) trace.Tracer { return p.tp.Tracer(name) }

// Shutdown flushes pending spans and closes the exporter.
func (p *Provider) Shutdown(ctx context.Context) error {
	return p.tp.Shutdown(ctx)
}

// ForceFlush exports all buffered spans immediately (for graceful shutdown).
func (p *Provider) ForceFlush(ctx context.Context) error {
	return p.tp.ForceFlush(ctx)
}

// SpanFromContext returns the active span from the context (or a no-op span).
func SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// StartSpan starts a span on the named tracer. Returns the new context (with the
// span attached) and the span. Always call span.End() when done (use defer).
func StartSpan(ctx context.Context, tracerName, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return otel.Tracer(tracerName).Start(ctx, spanName, opts...)
}

// noopExporter discards all spans.
type noopExporter struct{}

func (noopExporter) ExportSpans(_ context.Context, _ []sdktrace.ReadOnlySpan) error { return nil }
func (noopExporter) Shutdown(_ context.Context) error                               { return nil }
