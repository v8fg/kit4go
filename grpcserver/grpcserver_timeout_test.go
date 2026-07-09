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
	// rpcReceived is signalled by the handler once the RPC has landed inside
	// grpc.Server (after decode, before it blocks). Waiting on it — instead of a
	// fixed sleep — proves the RPC is genuinely in-flight when GracefulStop
	// runs, removing the scheduling race where, under -race/CI load, the RPC
	// had not yet arrived so GracefulStop completed and Start returned nil.
	rpcReceived := make(chan struct{})

	srv := grpcserver.NewWithListener(ln,
		grpcserver.WithShutdownTimeout(30*time.Millisecond),
	)
	srv.GRPCServer().RegisterService(&grpc.ServiceDesc{
		ServiceName: "blk.Blk",
		HandlerType: (*blkServerIface)(nil),
		Methods: []grpc.MethodDesc{{
			MethodName: "Hang",
			Handler:    blkHandler(&blkServer{release: blockingUntil, received: rpcReceived}),
		}},
		Streams:  []grpc.StreamDesc{},
		Metadata: "blk.proto",
	}, &blkServer{release: blockingUntil, received: rpcReceived})

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

	// Wait until the RPC is actually in-flight inside grpc.Server (handler has
	// decoded it and is about to block). This is the deterministic replacement
	// for a fixed sleep: under -race/CI load the RPC may not have landed by the
	// old 100ms mark, in which case GracefulStop completes immediately and Start
	// returns nil, failing require.Error. Signalling after decode guarantees the
	// in-flight condition GracefulStop needs to actually block.
	select {
	case <-rpcReceived:
	case <-time.After(5 * time.Second):
		t.Fatal("blocking RPC never reached the server handler")
	}

	// Cancel triggers GracefulStop. The in-flight RPC holds it open past the
	// 30ms shutdownTO, so Start must fall through to the timeout branch.
	cancel()

	select {
	case err := <-done:
		require.Error(t, err, "Start should time out GracefulStop and return an error")
		require.ErrorIs(t, err, grpcserver.ErrShutdownTimeout,
			"timeout error must match the ErrShutdownTimeout sentinel")
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
	release  chan struct{}
	received chan struct{} // closed once the handler has decoded the RPC
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
// closed, simulating a long-running RPC. It signals `received` after decoding
// so callers can deterministically confirm the RPC is in-flight before relying
// on GracefulStop actually blocking.
func blkHandler(srvIface *blkServer) func(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	return func(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
		// Ignore decode errors; we only care about blocking.
		_ = dec(new(empty))
		// Signal receipt AFTER the RPC has landed inside grpc.Server but BEFORE
		// we block, so a waiting test knows GracefulStop will genuinely block.
		select {
		case <-srvIface.received: // already signalled (handler reused)
		default:
			close(srvIface.received)
		}
		select {
		case <-srvIface.release:
		case <-ctx.Done():
		}
		return new(empty), ctx.Err()
	}
}
