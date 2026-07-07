// Package grpcserver wraps google.golang.org/grpc with middleware (interceptor)
// chaining, configurable options, and context-driven graceful shutdown.
//
// Ad-tech uses: internal RPC between bidder services (bid decision, user
// profile, budget). Pair with kit4go/grpcclient (the client side) for symmetric
// high-perf inter-service communication.
package grpcserver

import (
	"context"
	"errors"
	"net"
	"time"

	"google.golang.org/grpc"
)

// UnaryInterceptor is the standard gRPC unary server interceptor.
type UnaryInterceptor = grpc.UnaryServerInterceptor

// StreamInterceptor is the standard gRPC stream server interceptor.
type StreamInterceptor = grpc.StreamServerInterceptor

// Server wraps a grpc.Server with interceptors, options, and graceful shutdown.
type Server struct {
	gs         *grpc.Server
	listener   net.Listener
	listenErr  error // a New(addr) bind failure, surfaced from Start/Serve instead of the generic ErrNoListener
	unaryInts  []UnaryInterceptor
	streamInts []StreamInterceptor
	opts       []grpc.ServerOption
	shutdownTO time.Duration
	maxRecv    int
}

// Option configures the Server.
type Option func(*Server)

// WithUnaryInterceptor appends a unary interceptor (outer-to-inner order).
func WithUnaryInterceptor(i UnaryInterceptor) Option {
	return func(s *Server) { s.unaryInts = append(s.unaryInts, i) }
}

// WithStreamInterceptor appends a stream interceptor.
func WithStreamInterceptor(i StreamInterceptor) Option {
	return func(s *Server) { s.streamInts = append(s.streamInts, i) }
}

// WithMaxRecvMessageSize sets the max inbound message size in bytes (default 4MB).
func WithMaxRecvMessageSize(n int) Option { return func(s *Server) { s.maxRecv = n } }

// WithShutdownTimeout sets the graceful-stop budget (default 10s).
func WithShutdownTimeout(d time.Duration) Option { return func(s *Server) { s.shutdownTO = d } }

// WithGRPCOption passes a raw grpc.ServerOption (keepalive, TLS, etc.).
func WithGRPCOption(o grpc.ServerOption) Option {
	return func(s *Server) { s.opts = append(s.opts, o) }
}

// ErrNoListener is returned by Start/Serve when no listener is bound.
var ErrNoListener = errors.New("grpcserver: no listener")

// New builds a Server that will listen on addr. Pass "" to defer binding.
func New(addr string, opts ...Option) *Server {
	s := &Server{shutdownTO: 10 * time.Second, maxRecv: 4 * 1024 * 1024}
	for _, opt := range opts {
		opt(s)
	}
	s.build()
	if addr != "" {
		l, err := net.Listen("tcp", addr)
		if err != nil {
			s.listenErr = err // surfaced from Start/Serve so the caller sees the real bind failure
		} else {
			s.listener = l
		}
	}
	return s
}

// NewWithListener builds a Server with a pre-bound listener (bufconn for tests,
// TLS listener, etc.).
func NewWithListener(l net.Listener, opts ...Option) *Server {
	s := &Server{listener: l, shutdownTO: 10 * time.Second, maxRecv: 4 * 1024 * 1024}
	for _, opt := range opts {
		opt(s)
	}
	s.build()
	return s
}

func (s *Server) build() {
	opts := []grpc.ServerOption{grpc.MaxRecvMsgSize(s.maxRecv)}
	opts = append(opts, s.opts...)
	if len(s.unaryInts) > 0 {
		opts = append(opts, grpc.ChainUnaryInterceptor(s.unaryInts...))
	}
	if len(s.streamInts) > 0 {
		opts = append(opts, grpc.ChainStreamInterceptor(s.streamInts...))
	}
	s.gs = grpc.NewServer(opts...)
}

// RegisterService registers a gRPC service.
func (s *Server) RegisterService(sd *grpc.ServiceDesc, ss any) {
	s.gs.RegisterService(sd, ss)
}

// GRPCServer returns the underlying *grpc.Server.
func (s *Server) GRPCServer() *grpc.Server { return s.gs }

// Bind creates a TCP listener on addr.
func (s *Server) Bind(addr string) error {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	s.listener = l
	return nil
}

// Addr returns the bound address.
func (s *Server) Addr() string {
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

// Start serves and blocks until ctx is done, then gracefully stops within
// shutdownTO. Returns nil on clean stop, an error on timeout.
func (s *Server) Start(ctx context.Context) error {
	if s.listener == nil {
		if s.listenErr != nil {
			return s.listenErr
		}
		return ErrNoListener
	}
	errCh := make(chan error, 1)
	go func() { errCh <- s.gs.Serve(s.listener) }()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}
	done := make(chan struct{})
	go func() { s.gs.GracefulStop(); close(done) }()
	select {
	case <-done:
		return nil
	case <-time.After(s.shutdownTO):
		s.gs.Stop()
		return errors.New("grpcserver: graceful stop timed out")
	}
}

// Serve blocks (standard gRPC Serve, no graceful shutdown).
func (s *Server) Serve() error {
	if s.listener == nil {
		if s.listenErr != nil {
			return s.listenErr
		}
		return ErrNoListener
	}
	return s.gs.Serve(s.listener)
}

// GracefulStop stops accepting new RPCs and waits for in-flight to complete.
func (s *Server) GracefulStop() { s.gs.GracefulStop() }

// Stop immediately stops all RPCs.
func (s *Server) Stop() { s.gs.Stop() }
