package httpserver_test

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/v8fg/kit4go/httpserver"
)

// statusMW wraps h and records the response status on the recorder-like writer.
func statusMW(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r)
	})
}

// ExampleNew shows the basic construction: an address, a handler, a middleware
// chain (outer-to-inner: the first middleware runs first on the request path),
// and the sensible timeout defaults. The returned Server is ready for Start;
// here we exercise the wrapped handler directly with httptest so the example
// needs no network and does not block.
func ExampleNew() {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok")
	})

	srv := httpserver.New(":8080", handler,
		httpserver.WithMiddleware(statusMW),
		httpserver.WithReadHeaderTimeout(5*time.Second),
		httpserver.WithWriteTimeout(10*time.Second),
		httpserver.WithShutdownTimeout(5*time.Second),
	)
	_ = srv

	fmt.Println("server configured")
	// Output:
	// server configured
}

// ExampleServer_HTTPServer demonstrates the middleware chain applied to the
// handler by routing a request through the underlying *http.Server.Handler.
// The first-added middleware runs outermost: it prints before delegating, then
// the base handler writes the body.
func ExampleServer_HTTPServer() {
	mw := func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Println("middleware: before handler")
			h.ServeHTTP(w, r)
		})
	}
	base := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "hello")
	})

	srv := httpserver.New(":0", base, httpserver.WithMiddleware(mw))

	rec := httptest.NewRecorder()
	srv.HTTPServer().Handler.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	fmt.Printf("body=%q", rec.Body.String())
	// Output:
	// middleware: before handler
	// body="hello\n"
}

// ExampleServer_Start exercises the serve-then-graceful-shutdown path with no
// external network: Start binds a real listener, we cancel the context a moment
// later, and Start returns after a clean (no in-flight requests) shutdown.
func ExampleServer_Start() {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "served")
	})
	srv := httpserver.New("127.0.0.1:0", handler,
		httpserver.WithShutdownTimeout(2*time.Second),
	)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond) // let Start bind and begin serving
		cancel()
	}()

	if err := srv.Start(ctx); err != nil {
		log.Printf("start: %v", err)
	}
	fmt.Println("stopped")
	// Output:
	// stopped
}
