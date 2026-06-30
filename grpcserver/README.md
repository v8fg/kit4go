# grpcserver

Wraps `google.golang.org/grpc` with interceptor chaining, configurable options,
and context-driven graceful shutdown. Own module so the grpc/protobuf dependency
graph stays isolated.

## API

```go
srv := grpcserver.New(":50051",
    grpcserver.WithUnaryInterceptor(recoveryInterceptor, loggingInterceptor),
    grpcserver.WithMaxRecvMessageSize(16*1024*1024),
    grpcserver.WithShutdownTimeout(10*time.Second),
)
srv.RegisterService(&my_pb.ServiceDesc, &myService{})
if err := srv.Start(ctx); err != nil { log.Fatal(err) }
```

| Symbol | Behavior |
|---|---|
| `New(addr, opts...)` | Build with TCP listener |
| `NewWithListener(l, opts...)` | Build with a pre-bound listener (bufconn/TLS) |
| `WithUnaryInterceptor(i)` / `WithStreamInterceptor(i)` | Append interceptors |
| `WithMaxRecvMessageSize(n)` | Max inbound message (default 4MB) |
| `WithShutdownTimeout(d)` | Graceful-stop budget (default 10s) |
| `WithGRPCOption(o)` | Pass-through raw grpc.ServerOption (keepalive, TLS) |
| `RegisterService(desc, impl)` | Register a gRPC service |
| `Start(ctx)` | Serve; gracefully stop on ctx.Done() |
| `Serve()` / `GracefulStop()` / `Stop()` | Standard lifecycle |
| `GRPCServer()` | Underlying *grpc.Server |
| `Bind(addr)` / `Addr()` | Deferred binding |

## Testing

80% statement coverage, `-race` clean, via `bufconn` (in-process gRPC, no real
listener). Covers new-with-addr, new-with-listener, start+serve+lifecycle
(graceful stop on ctx cancel), no-listener guard, deferred Bind, GracefulStop
idempotency, custom options (interceptors + message size), Serve directly,
and RegisterService.

```bash
go test -race -cover ./...
```
