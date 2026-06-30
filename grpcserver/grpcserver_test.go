package grpcserver_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"

	"github.com/v8fg/kit4go/grpcserver"
)

const bufSize = 1024 * 1024

func TestNewWithAddr(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	_ = l.Close()

	s := grpcserver.New(addr)
	require.NotNil(t, s)
	require.NotEqual(t, "", s.Addr())
}

func TestNewWithListener(t *testing.T) {
	lis := bufconn.Listen(bufSize)
	s := grpcserver.NewWithListener(lis)
	require.NotNil(t, s)
	require.NotNil(t, s.GRPCServer())
}

func TestStartAndLifecycle(t *testing.T) {
	lis := bufconn.Listen(bufSize)
	s := grpcserver.NewWithListener(lis)
	s.GRPCServer().RegisterService(&grpc.ServiceDesc{
		ServiceName: "test.Echo",
		HandlerType: (*echoServerIface)(nil),
		Methods:     []grpc.MethodDesc{},
		Streams:     []grpc.StreamDesc{},
		Metadata:    "test.proto",
	}, &echoServer{})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Start(ctx) }()
	time.Sleep(100 * time.Millisecond)

	// Server is serving — cancel triggers GracefulStop.
	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after cancel")
	}
}

func TestStartNoListener(t *testing.T) {
	s := grpcserver.New("")
	err := s.Start(context.Background())
	require.ErrorIs(t, err, grpcserver.ErrNoListener)
}

func TestBind(t *testing.T) {
	s := grpcserver.New("")
	require.Equal(t, "", s.Addr())
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	_ = l.Close()
	require.NoError(t, s.Bind(addr))
	require.Equal(t, addr, s.Addr())
}

func TestGracefulStop(t *testing.T) {
	lis := bufconn.Listen(bufSize)
	s := grpcserver.NewWithListener(lis)
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = s.Start(ctx) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	time.Sleep(100 * time.Millisecond)
	require.NotPanics(t, func() { s.Stop() })
}

func TestCustomOptions(t *testing.T) {
	s := grpcserver.NewWithListener(bufconn.Listen(bufSize),
		grpcserver.WithMaxRecvMessageSize(8*1024*1024),
		grpcserver.WithShutdownTimeout(5*time.Second),
		grpcserver.WithUnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
			return h(ctx, req)
		}),
	)
	require.NotNil(t, s)
	require.NotNil(t, s.GRPCServer())
}

func TestServeDirectly(t *testing.T) {
	lis := bufconn.Listen(bufSize)
	s := grpcserver.NewWithListener(lis)
	go func() { _ = s.Serve() }()
	time.Sleep(50 * time.Millisecond)
	s.Stop()
	time.Sleep(50 * time.Millisecond)
}

func TestRegisterService(t *testing.T) {
	lis := bufconn.Listen(bufSize)
	s := grpcserver.NewWithListener(lis)
	require.NotPanics(t, func() {
		s.RegisterService(&grpc.ServiceDesc{
			ServiceName: "test.Dummy",
			HandlerType: (*echoServerIface)(nil),
			Methods:     []grpc.MethodDesc{},
			Streams:     []grpc.StreamDesc{},
			Metadata:    "test.proto",
		}, &echoServer{})
	})
}

type echoServerIface any
type echoServer struct{}
