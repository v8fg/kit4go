package grpcserver_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/v8fg/kit4go/grpcserver"
)

// TestStart_ShutdownTimeoutDeterministic reliably hits the GracefulStop
// timeout branch by keeping an in-flight RPC blocked past shutdownTO. A real
// client/server round-trip is used because GracefulStop only blocks while an
// RPC is actually in-flight inside grpc.Server.
func TestStart_ShutdownTimeoutDeterministic(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()

	// blockingUntil releases the handler only after we close it, keeping the
	// RPC in-flight and forcing GracefulStop to exceed shutdownTO.
	blockingUntil := make(chan struct{})

	srv := grpcserver.NewWithListener(ln,
		grpcserver.WithShutdownTimeout(30*time.Millisecond),
	)
	srv.GRPCServer().RegisterService(&grpc.ServiceDesc{
		ServiceName: "blk.Blk",
		HandlerType: (*blkServerIface)(nil),
		Methods: []grpc.MethodDesc{{
			MethodName: "Hang",
			Handler:    blkHandler(&blkServer{release: blockingUntil}),
		}},
		Streams:   []grpc.StreamDesc{},
		Metadata:  "blk.proto",
	}, &blkServer{release: blockingUntil})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Start(ctx) }()

	// Connect a client and fire the blocking RPC. We do not wait on its result.
	cc, derr := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, derr)
	defer cc.Close()
	go func() {
		// Make a raw unary call. The reply is ignored; we only need the RPC to
		// be in-flight when shutdown begins.
		out := new(bytes)
		_ = cc.Invoke(context.Background(), "/blk.Blk/Hang", &empty{}, out)
	}()

	// Wait long enough for the RPC to be in-flight inside the server.
	time.Sleep(100 * time.Millisecond)

	// Cancel triggers GracefulStop. The in-flight RPC holds it open past the
	// 30ms shutdownTO, so Start must fall through to the timeout branch.
	cancel()

	select {
	case err := <-done:
		require.Error(t, err, "Start should time out GracefulStop and return an error")
		require.Contains(t, err.Error(), "timed out")
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after cancel with in-flight RPC")
	}

	// Release the blocked handler so the goroutine exits cleanly.
	close(blockingUntil)
}

// Minimal hand-rolled protobuf-ish types so we don't need a generated stub.
// grpc's unary handler receives/returns []byte via the codec; raw Invoke with
// empty messages works because we never inspect payloads.

type blkServerIface any

type blkServer struct {
	release chan struct{}
}

type empty struct{}

func (e *empty) Reset()         {}
func (e *empty) String() string { return "" }
func (e *empty) ProtoMessage()  {}

type bytes struct{}

func (b *bytes) Reset()         {}
func (b *bytes) String() string { return "" }
func (b *bytes) ProtoMessage()  {}

// blkHandler returns a grpc method handler that blocks until `release` is
// closed, simulating a long-running RPC.
func blkHandler(srvIface *blkServer) func(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	return func(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
		// Ignore decode errors; we only care about blocking.
		_ = dec(new(empty))
		select {
		case <-srvIface.release:
		case <-ctx.Done():
		}
		return new(empty), ctx.Err()
	}
}
