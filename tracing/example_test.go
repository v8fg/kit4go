package tracing_test

import (
	"context"

	"github.com/v8fg/kit4go/tracing"
)

// ExampleNew demonstrates constructing a tracing Provider (which installs the
// global TracerProvider), starting a span around a unit of work, and shutting
// down to flush pending spans. Use WithExporter(otlp/jaeger) in production;
// WithStdout is handy for local debugging.
//
// This is a non-output example: New sets a global provider, so it is compiled
// (verifying the API) but not executed here, to avoid interfering with the test
// binary's global tracing state.
func ExampleNew() {
	provider, err := tracing.New(
		tracing.WithServiceName("my-service"),
		// tracing.WithStdout(os.Stdout),       // local debug
		// tracing.WithExporter(jaegerExporter), // production
	)
	if err != nil {
		panic(err)
	}
	defer provider.Shutdown(context.Background())

	ctx, span := tracing.StartSpan(context.Background(), "my-service", "do-work")
	defer span.End()

	// Propagate ctx to downstream calls so child spans parent to this one.
	_ = tracing.SpanFromContext(ctx)
}
