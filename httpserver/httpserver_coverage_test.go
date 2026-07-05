package httpserver_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/v8fg/kit4go/httpserver"
)

func TestStart_GracefulShutdown(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
	s := httpserver.New(":0", h)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
}

func TestStart_AdrRequired(t *testing.T) {
	s := httpserver.New("", http.NotFoundHandler())
	if err := s.Start(context.Background()); err != httpserver.ErrAddrRequired {
		t.Fatalf("empty addr: %v", err)
	}
}

func TestListenAndServe(t *testing.T) {
	s := httpserver.New(":0", http.NotFoundHandler())
	go func() {
		_ = s.ListenAndServe()
	}()
	time.Sleep(100 * time.Millisecond)
	s.Close()
}

func TestClose_Idempotent(t *testing.T) {
	s := httpserver.New(":0", http.NotFoundHandler())
	s.Close()
	s.Close()
}
