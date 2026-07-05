package grpcserver_test

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

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
