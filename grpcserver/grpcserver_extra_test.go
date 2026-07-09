package grpcserver_test

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/v8fg/kit4go/grpcserver"
)

// TestNew_ListenError covers the bind-failure path in New: an invalid address
// (port already taken / malformed) records listenErr and surfaces it from
// Start/Serve rather than the generic ErrNoListener.
func TestNew_ListenError(t *testing.T) {
	// Bind a real listener to occupy the port, then attempt New on the same
	// addr -> net.Listen fails -> listenErr recorded.
	occ, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer occ.Close()
	addr := occ.Addr().String()

	s := grpcserver.New(addr)
	require.NotNil(t, s)

	// Start must surface the real bind error (listenErr), not ErrNoListener.
	err = s.Start(context.Background())
	require.Error(t, err)
	require.NotErrorIs(t, err, grpcserver.ErrNoListener,
		"Start should surface the bind error, not ErrNoListener")

	// Serve must surface the same listenErr too.
	err = s.Serve()
	require.Error(t, err)
	require.NotErrorIs(t, err, grpcserver.ErrNoListener,
		"Serve should surface the bind error, not ErrNoListener")
}

// TestBind_Error covers the net.Listen error branch in Bind.
func TestBind_Error(t *testing.T) {
	occ, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer occ.Close()

	s := grpcserver.New("")
	err = s.Bind(occ.Addr().String())
	require.Error(t, err, "Bind on an occupied port must fail")
}

// TestStart_ServeError covers the errCh branch in Start: when the underlying
// gs.Serve returns an error before ctx is cancelled, Start must propagate it.
// We force Serve to fail immediately by closing the listener out from under it.
func TestStart_ServeError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close()) // close so Serve fails right away

	// Re-construct a Server whose listener is already closed. We use New with
	// the (now free) addr — but the port may be reclaimed; to make the test
	// deterministic we instead close after wiring via NewWithListener.
	ln2, err := net.Listen("tcp", addr)
	require.NoError(t, err)
	s := grpcserver.NewWithListener(ln2)
	require.NoError(t, ln2.Close()) // now Serve will return immediately with err

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = s.Start(ctx)
	require.Error(t, err, "Start should propagate gs.Serve's error")
}

// TestServe_ListenError covers the listenErr branch in Serve.
func TestServe_ListenError(t *testing.T) {
	occ, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer occ.Close()

	s := grpcserver.New(occ.Addr().String())
	err = s.Serve()
	require.Error(t, err)
	require.False(t, errors.Is(err, grpcserver.ErrNoListener))
}

// TestStart_DoubleCallReturnsError (R16) confirms the idempotency guard: a
// second Start on the same Server returns ErrAlreadyStarted immediately
// instead of blocking forever on the already-served listener.
//
// To make the test deterministic (the two Start calls must not race for the
// CAS), we first prove the first Start has genuinely entered Serve by dialing
// it, and only then invoke the second Start synchronously from the test
// goroutine.
func TestStart_DoubleCallReturnsError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()
	addr := ln.Addr().String()

	s := grpcserver.NewWithListener(ln)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	firstDone := make(chan error, 1)
	go func() { firstDone <- s.Start(ctx) }()

	// Prove the first Start has entered Serve by establishing a real client
	// connection. A successful dial guarantees the listener is being served
	// and therefore startGuard has been flipped by the first Start.
	cc, derr := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, derr)
	// Force the connection to actually attempt a handshake so the dial blocks
	// until the server is reachable, then close it.
	connCtx, connCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer connCancel()
	_ = cc.Invoke(connCtx, "/noop.Noop/None", &empty{}, &bytes{}) // expected to fail; we only needed the dial
	cc.Close()

	// Now the second Start must return immediately, not block.
	secondDone := make(chan error, 1)
	go func() { secondDone <- s.Start(ctx) }()
	select {
	case secondErr := <-secondDone:
		require.ErrorIs(t, secondErr, grpcserver.ErrAlreadyStarted,
			"second Start must return ErrAlreadyStarted, not block or serve")
	case <-time.After(5 * time.Second):
		t.Fatal("second Start did not return (would block forever without the guard)")
	}

	// Let the first Start finish cleanly.
	cancel()
	select {
	case firstErr := <-firstDone:
		// The first Start must NOT be the one rejected as a duplicate.
		require.False(t, errors.Is(firstErr, grpcserver.ErrAlreadyStarted),
			"first Start must not be mistaken for a duplicate: %v", firstErr)
	case <-time.After(5 * time.Second):
		t.Fatal("first Start did not return after cancel")
	}
}
