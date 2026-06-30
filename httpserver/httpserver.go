// Package httpserver wraps net/http with middleware chaining, configurable
// timeouts, and context-driven graceful shutdown — the one-call HTTP server
// every service needs. Pure standard library.
//
// The Server exposes a builder API: set the address, handler, middleware chain,
// and timeouts via functional options; Start blocks until the context is done,
// then shuts down gracefully (in-flight requests complete, new ones rejected).
//
// Ad-tech uses: the bid-request ingress server. Pair with shutdown (lifecycle),
// limiter (per-route rate limiting), metrics (request counters), and tracing
// (span per request) for a complete request-processing stack.
package httpserver

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// Middleware is the standard middleware signature: wrap a handler.
type Middleware func(http.Handler) http.Handler

// Server wraps an http.Server with middleware, timeouts, and graceful shutdown.
type Server struct {
	srv         *http.Server
	addr        string
	handler     http.Handler
	middlewares []Middleware
	shutdownTO  time.Duration
	readHeader  time.Duration
	readTO      time.Duration
	writeTO     time.Duration
	idleTO      time.Duration
}

// Option configures the Server.
type Option func(*Server)

// WithMiddleware appends middleware to the chain (outer-to-inner order: the first
// middleware runs first on the request path).
func WithMiddleware(mw ...Middleware) Option {
	return func(s *Server) { s.middlewares = append(s.middlewares, mw...) }
}

// WithReadHeaderTimeout sets the max time to read request headers (default 10s).
func WithReadHeaderTimeout(d time.Duration) Option { return func(s *Server) { s.readHeader = d } }

// WithReadTimeout sets the max time to read the entire request (default 30s).
func WithReadTimeout(d time.Duration) Option { return func(s *Server) { s.readTO = d } }

// WithWriteTimeout sets the max time to write the response (default 30s).
func WithWriteTimeout(d time.Duration) Option { return func(s *Server) { s.writeTO = d } }

// WithIdleTimeout sets the max idle time for keep-alive connections (default 120s).
func WithIdleTimeout(d time.Duration) Option { return func(s *Server) { s.idleTO = d } }

// WithShutdownTimeout sets the graceful-shutdown budget (default 10s).
func WithShutdownTimeout(d time.Duration) Option { return func(s *Server) { s.shutdownTO = d } }

// ErrAddrRequired is returned by Start when no address is set.
var ErrAddrRequired = errors.New("httpserver: address is required")

// New builds a Server bound to addr with the given handler and options.
func New(addr string, handler http.Handler, opts ...Option) *Server {
	s := &Server{
		addr:       addr,
		handler:    handler,
		readHeader: 10 * time.Second,
		readTO:     30 * time.Second,
		writeTO:    30 * time.Second,
		idleTO:     120 * time.Second,
		shutdownTO: 10 * time.Second,
	}
	for _, opt := range opts {
		opt(s)
	}
	// Apply middleware chain (reverse so first-added runs outermost).
	h := s.handler
	for i := len(s.middlewares) - 1; i >= 0; i-- {
		h = s.middlewares[i](h)
	}
	s.srv = &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: s.readHeader,
		ReadTimeout:       s.readTO,
		WriteTimeout:      s.writeTO,
		IdleTimeout:       s.idleTO,
	}
	return s
}

// HTTPServer returns the underlying *http.Server for advanced use (TLS, custom
// listeners, etc.).
func (s *Server) HTTPServer() *http.Server { return s.srv }

// Start binds, serves, and blocks until ctx is done, then shuts down gracefully.
// Returns the shutdown error (nil on clean exit).
func (s *Server) Start(ctx context.Context) error {
	if s.addr == "" {
		return ErrAddrRequired
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.srv.ListenAndServe()
	}()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), s.shutdownTO)
	defer cancel()
	return s.srv.Shutdown(shutdownCtx)
}

// ListenAndServe is the standard net/http entrypoint (blocks, no graceful
// shutdown integration — use Start for that).
func (s *Server) ListenAndServe() error { return s.srv.ListenAndServe() }

// Shutdown gracefully stops accepting new connections and waits for in-flight.
func (s *Server) Shutdown(ctx context.Context) error { return s.srv.Shutdown(ctx) }

// Close immediately drops all connections (use Shutdown for graceful).
func (s *Server) Close() error { return s.srv.Close() }

// Addr returns the configured address.
func (s *Server) Addr() string { return s.addr }
