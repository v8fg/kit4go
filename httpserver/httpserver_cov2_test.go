package httpserver

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestStart_ListenAndServeError covers the `case err := <-errCh` branch of
// Start: when the underlying ListenAndServe fails immediately (here: binding to
// a port already in use), Start returns that error rather than waiting on
// ctx.Done().
func TestStart_ListenAndServeError(t *testing.T) {
	// Acquire a free port and keep it held so a second bind to the same address
	// fails with EADDRINUSE.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()
	inUse := ln.Addr().String()

	s := New(inUse, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	// Start should return the ListenAndServe error promptly (bind failure). Give
	// it a generous deadline so the test fails fast rather than hanging.
	done := make(chan error, 1)
	go func() { done <- s.Start(context.Background()) }()
	select {
	case err := <-done:
		require.Error(t, err, "Start must surface the ListenAndServe bind error")
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after ListenAndServe failure (deadlocked on errCh)")
	}
}

// TestStart_ShutdownError covers the Shutdown-error branch of Start
// (`if err := s.srv.Shutdown(...); err != nil { Close(); return err }`). We
// force Shutdown to miss its deadline by giving it a zero budget while a request
// is mid-flight (response delayed past the shutdown timeout).
func TestStart_ShutdownError(t *testing.T) {
	addr := freeAddr(t)
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sleep longer than the shutdown budget so Shutdown's context expires
		// while the connection is still active.
		time.Sleep(400 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})
	// near-zero shutdown budget → Shutdown will time out.
	s := New(addr, h, WithShutdownTimeout(time.Millisecond))

	ctx, cancel := context.WithCancel(context.Background())
	startDone := make(chan error, 1)
	go func() { startDone <- s.Start(ctx) }()
	time.Sleep(50 * time.Millisecond) // let it bind

	// Kick off a slow request that keeps a connection active through shutdown.
	go func() {
		c := &http.Client{Timeout: 2 * time.Second}
		resp, _ := c.Get("http://" + addr)
		if resp != nil {
			_ = resp.Body.Close()
		}
	}()
	time.Sleep(20 * time.Millisecond) // let the request land

	cancel() // triggers graceful shutdown with a tiny budget → deadline exceeded

	select {
	case err := <-startDone:
		// Shutdown timed out → Start returns the shutdown error and force-closes.
		require.Error(t, err, "Shutdown deadline exceeded must surface as an error")
	case <-time.After(3 * time.Second):
		t.Fatal("Start did not return after Shutdown timeout")
	}
}
