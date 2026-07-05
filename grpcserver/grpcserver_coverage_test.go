package grpcserver_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/v8fg/kit4go/grpcserver"
	"google.golang.org/grpc"
)

func TestWithStreamInterceptor(t *testing.T) {
	si := func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		return handler(srv, ss)
	}
	s := grpcserver.New("",
		grpcserver.WithStreamInterceptor(si),
		grpcserver.WithGRPCOption(grpc.MaxRecvMsgSize(1024)),
	)
	if s == nil {
		t.Fatal("New returned nil")
	}
}

func TestServe_WithListener(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	s := grpcserver.NewWithListener(ln)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	go func() { _ = s.Start(ctx) }()
	time.Sleep(50 * time.Millisecond)
}

func TestGracefulStopMethod(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	s := grpcserver.NewWithListener(ln)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	go func() { _ = s.Start(ctx) }()
	time.Sleep(50 * time.Millisecond)
	s.GracefulStop()
}

func TestServe_NoListener(t *testing.T) {
	s := grpcserver.New("")
	if err := s.Serve(); err == nil {
		t.Fatal("Serve without listener should error")
	}
}

func TestServe_WithValidListener(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	s := grpcserver.NewWithListener(ln)
	// Serve blocks; run it and stop after a brief moment.
	go func() {
		time.Sleep(50 * time.Millisecond)
		s.Stop()
	}()
	_ = s.Serve()
}
