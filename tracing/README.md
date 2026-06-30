# tracing

One-call OpenTelemetry SDK setup: TracerProvider with service name, sampler, and
exporter — plus convenience span helpers. Own module so the otel dependency graph
stays isolated.

## Why

Setting up otel correctly involves resource detection, sampler choice, exporter
wiring, and global registration — boilerplate that's easy to get subtly wrong.
This package does it in one `New` call with functional options, and exposes
`StartSpan` / `SpanFromContext` helpers for clean call sites. Pair with
`kit4go/metrics` for a complete observability stack (traces + metrics).

## API

```go
tp, _ := tracing.New(
    tracing.WithServiceName("bidder"),
    tracing.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(0.1))),
    tracing.WithStdout(os.Stderr),              // dev; or WithExporter(otlpExporter)
)
defer tp.Shutdown(ctx)

ctx, span := tracing.StartSpan(ctx, "kit4go/bid", "bid-decision")
defer span.End()
```

| Symbol | Behavior |
|---|---|
| `New(opts...) (*Provider, error)` | Init + set global TracerProvider |
| `WithServiceName(name)` | Service name on all spans (resource attribute) |
| `WithSampler(s)` | Trace sampler (default AlwaysSample) |
| `WithExporter(e)` | Inject an exporter (OTLP, tracetest) |
| `WithStdout(w)` | Dev stdout exporter (os.Stderr if nil) |
| `Provider.Tracer(name)` | Named tracer |
| `Provider.Shutdown(ctx)` | Flush + close (idempotent) |
| `Provider.ForceFlush(ctx)` | Export buffered spans now |
| `StartSpan(ctx, tracer, name, opts...)` | Convenience span starter |
| `SpanFromContext(ctx)` | Active span (or no-op) |

## Testing

90% statement coverage, `-race` clean. Uses an in-memory exporter
(`tracetest.InMemoryExporter`) to verify span creation, service-name resource
attributes, parent/child span chains, the StartSpan helper, SpanFromContext
recording state, custom sampler (NeverSample drops all), stdout exporter output,
idempotent Shutdown, and the no-exporter noop path.

```bash
go test -race -cover ./...
```
