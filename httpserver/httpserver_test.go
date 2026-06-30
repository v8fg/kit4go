package httpserver

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// freeAddr returns a host:port on a free port (best-effort; the server may race
// on reuse but the OS typically rotates fast enough for tests).
func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	_ = l.Close()
	return addr
}

func TestNewBasic(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	s := New(":0", h)
	require.NotNil(t, s)
	require.Equal(t, ":0", s.Addr())
	require.NotNil(t, s.HTTPServer())
}

func TestStartAndServe(t *testing.T) {
	addr := freeAddr(t)
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	s := New(addr, h)

	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = s.Start(ctx) }()
	time.Sleep(50 * time.Millisecond) // let it bind

	resp, err := http.Get("http://" + addr)
	require.NoError(t, err)
	require.Equal(t, http.StatusTeapot, resp.StatusCode)
	_ = resp.Body.Close()

	cancel() // graceful shutdown
	time.Sleep(100 * time.Millisecond)
}

func TestMiddlewareChain(t *testing.T) {
	addr := freeAddr(t)
	order := []string{}
	mw1 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "mw1-before")
			next.ServeHTTP(w, r)
			order = append(order, "mw1-after")
		})
	}
	mw2 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "mw2-before")
			next.ServeHTTP(w, r)
			order = append(order, "mw2-after")
		})
	}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		order = append(order, "handler")
		w.WriteHeader(http.StatusOK)
	})
	s := New(addr, h, WithMiddleware(mw1, mw2))

	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = s.Start(ctx) }()
	time.Sleep(50 * time.Millisecond)

	resp, err := http.Get("http://" + addr)
	require.NoError(t, err)
	_ = resp.Body.Close()
	cancel()
	time.Sleep(100 * time.Millisecond)

	require.Equal(t, []string{"mw1-before", "mw2-before", "handler", "mw2-after", "mw1-after"}, order)
}

func TestGracefulShutdown(t *testing.T) {
	addr := freeAddr(t)
	done := make(chan struct{})
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond) // simulate slow request
		w.WriteHeader(http.StatusOK)
		select {
		case done <- struct{}{}:
		default:
		}
	})
	s := New(addr, h, WithShutdownTimeout(2*time.Second))

	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = s.Start(ctx) }()
	time.Sleep(50 * time.Millisecond)

	// Start a request, then immediately cancel (request is in-flight).
	go func() {
		resp, _ := http.Get("http://" + addr)
		if resp != nil {
			_ = resp.Body.Close()
		}
	}()
	time.Sleep(20 * time.Millisecond) // let request start
	cancel()                          // trigger graceful shutdown

	// The in-flight request should complete (within shutdown timeout).
	select {
	case <-done:
		// success: request completed during graceful shutdown
	case <-time.After(3 * time.Second):
		t.Fatal("in-flight request did not complete during graceful shutdown")
	}
}

func TestAddrRequired(t *testing.T) {
	s := New("", http.NewServeMux())
	err := s.Start(context.Background())
	require.ErrorIs(t, err, ErrAddrRequired)
}

func TestCustomTimeouts(t *testing.T) {
	s := New(":0", http.NewServeMux(),
		WithReadHeaderTimeout(5*time.Second),
		WithReadTimeout(15*time.Second),
		WithWriteTimeout(15*time.Second),
		WithIdleTimeout(60*time.Second),
		WithShutdownTimeout(5*time.Second),
	)
	require.Equal(t, 5*time.Second, s.readHeader)
	require.Equal(t, 15*time.Second, s.readTO)
	require.Equal(t, 15*time.Second, s.writeTO)
	require.Equal(t, 60*time.Second, s.idleTO)
	require.Equal(t, 5*time.Second, s.shutdownTO)
}

func TestHTTPServerExposed(t *testing.T) {
	s := New(":0", http.NewServeMux())
	require.Equal(t, 10*time.Second, s.HTTPServer().ReadHeaderTimeout)
	require.Equal(t, 30*time.Second, s.HTTPServer().ReadTimeout)
}

func TestShutdownMethod(t *testing.T) {
	addr := freeAddr(t)
	s := New(addr, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = s.Start(ctx) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	time.Sleep(100 * time.Millisecond)
	require.NoError(t, s.Shutdown(context.Background()))
}

func TestClose(t *testing.T) {
	s := New(":0", http.NewServeMux())
	require.NoError(t, s.Close()) // no-op if not started, but must not panic
}

func TestBodyPassthrough(t *testing.T) {
	addr := freeAddr(t)
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_, _ = w.Write(body)
	})
	s := New(addr, h)

	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = s.Start(ctx) }()
	time.Sleep(50 * time.Millisecond)

	resp, err := http.Post("http://"+addr, "text/plain", stringReader("hello"))
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.Equal(t, "hello", string(body))
	cancel()
	time.Sleep(100 * time.Millisecond)
}

func stringReader(s string) io.Reader { return &reader{data: []byte(s)} }

type reader struct {
	data []byte
	pos  int
}

func (r *reader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}
