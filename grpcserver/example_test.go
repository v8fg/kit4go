package grpcserver_test

import (
	"time"

	"github.com/v8fg/kit4go/grpcserver"
)

// ExampleNew demonstrates constructing a grpcserver.Server with a max-message
// cap and a graceful-shutdown budget. Register your generated service desc +
// implementation, then Start (blocks until the context cancels, then
// GracefulStop drains in-flight RPCs within WithShutdownTimeout).
//
// Non-output example: Start blocks serving and requires a registered service
// implementation + proto-generated ServiceDesc, so it is compiled (verifying the
// construction API) but not executed here.
func ExampleNew() {
	srv := grpcserver.New(":50051",
		grpcserver.WithMaxRecvMessageSize(16*1024*1024), // 16 MB
		grpcserver.WithShutdownTimeout(30*time.Second),  // 30s graceful drain
		// grpcserver.WithUnaryInterceptor(logInterceptor),
		// grpcserver.WithGRPCOption(grpc.KeepaliveParams(...)),
	)
	// srv.RegisterService(&pb.Greeter_ServiceDesc, &greeterImpl{})
	// if err := srv.Start(ctx); err != nil { log.Fatal(err) }

	// GRPCServer returns the underlying *grpc.Server for options the wrapper
	// does not expose (health checks, reflection, TLS reload, etc.).
	_ = srv.GRPCServer()
}
