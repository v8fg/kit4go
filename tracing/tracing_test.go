package tracing_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/v8fg/kit4go/tracing"
)

func newTestProvider(t *testing.T, opts ...tracing.Option) (*tracing.Provider, *tracetest.InMemoryExporter) {
	t.Helper()
	exp := tracetest.NewInMemoryExporter()
	o := append([]tracing.Option{tracing.WithExporter(exp)}, opts...)
	p, err := tracing.New(o...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = p.Shutdown(context.Background()) })
	return p, exp
}

func TestNewWithDefaults(t *testing.T) {
	p, err := tracing.New()
	require.NoError(t, err)
	require.NotNil(t, p)
	require.NoError(t, p.Shutdown(context.Background()))
}

func TestNewWithServiceName(t *testing.T) {
	p, exp := newTestProvider(t, tracing.WithServiceName("bidder"))
	tr := p.Tracer("test")
	_, span := tr.Start(context.Background(), "bid")
	span.End()
	p.ForceFlush(context.Background())

	require.Len(t, exp.GetSpans(), 1)
	got := exp.GetSpans()
	require.Equal(t, "bid", got[0].Name)
	// Service name from resource.
	require.Contains(t, got[0].Resource.String(), "bidder")
}

func TestSpanCreationAndAttributes(t *testing.T) {
	p, exp := newTestProvider(t)
	tr := p.Tracer("kit4go/test")

	ctx, span := tr.Start(context.Background(), "parent")
	span.AddEvent("processing")
	_, child := tr.Start(ctx, "child")
	child.End()
	span.End()
	p.ForceFlush(context.Background())

	spans := exp.GetSpans()
	require.Len(t, spans, 2)
	// parent and child.
	names := map[string]bool{spans[0].Name: true, spans[1].Name: true}
	require.True(t, names["parent"])
	require.True(t, names["child"])
}

func TestStartSpanHelper(t *testing.T) {
	p, exp := newTestProvider(t)
	_ = p // provider sets global

	_, span := tracing.StartSpan(context.Background(), "kit4go/test", "helper-span")
	span.End()
	p.ForceFlush(context.Background())

	spans := exp.GetSpans()
	require.NotEmpty(t, spans)
	require.Equal(t, "helper-span", spans[0].Name)
}

func TestSpanFromContext(t *testing.T) {
	p, _ := newTestProvider(t)
	tr := p.Tracer("test")
	ctx, span := tr.Start(context.Background(), "active")
	require.True(t, span.IsRecording()) // active span is recording
	span.End()
	recovered := tracing.SpanFromContext(ctx)
	require.False(t, recovered.IsRecording()) // not recording after End
}

func TestCustomSampler(t *testing.T) {
	// NeverSample: no spans should be recorded.
	p, exp := newTestProvider(t, tracing.WithSampler(sdktrace.NeverSample()))
	tr := p.Tracer("test")
	_, span := tr.Start(context.Background(), "dropped")
	span.End()
	p.ForceFlush(context.Background())
	require.Empty(t, exp.GetSpans())
}

func TestShutdownIsIdempotent(t *testing.T) {
	p, err := tracing.New()
	require.NoError(t, err)
	require.NoError(t, p.Shutdown(context.Background()))
	require.NoError(t, p.Shutdown(context.Background()))
}

func TestNoExporterNoError(t *testing.T) {
	// No exporter configured -> noop exporter, no error.
	p, err := tracing.New(tracing.WithServiceName("test"))
	require.NoError(t, err)
	require.NotNil(t, p)
	tr := p.Tracer("test")
	_, span := tr.Start(context.Background(), "x")
	span.End()
	require.NoError(t, p.ForceFlush(context.Background()))
	require.NoError(t, p.Shutdown(context.Background()))
}

func TestStdoutExporter(t *testing.T) {
	// Stdout exporter writes to a buffer; verify it produces output.
	var buf bytes.Buffer
	p, err := tracing.New(tracing.WithStdout(&buf))
	require.NoError(t, err)
	tr := p.Tracer("test")
	_, span := tr.Start(context.Background(), "stdout-span")
	span.End()
	require.NoError(t, p.ForceFlush(context.Background()))
	require.NotEmpty(t, buf.String())
	require.Contains(t, buf.String(), "stdout-span")
	require.NoError(t, p.Shutdown(context.Background()))
}
