package httpclient_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/v8fg/kit4go/httpclient"
)

// ExampleNewClient shows the basic construction of a client. A zero
// ClientOptions yields sensible production defaults (5s connect, 30s request,
// up to 3 retries, 100 pooled idle conns). Override only the fields you need.
func ExampleNewClient() {
	cli := httpclient.NewClient(httpclient.ClientOptions{
		RequestTimeout: 5 * time.Second,
		RetryMax:       2,
		RetryWaitMin:   50 * time.Millisecond,
		RetryWaitMax:   time.Second,
	})
	_ = cli
	fmt.Println("client ready")

	// Output:
	// client ready
}

// ExampleClient_Get demonstrates a simple GET against a real (test) server,
// reading the status code and body out of the returned Response.
func ExampleClient_Get() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "hello, world")
	}))
	defer srv.Close()

	cli := httpclient.NewClient(httpclient.ClientOptions{
		RequestTimeout: 2 * time.Second,
	})
	resp, err := cli.Get(context.Background(), srv.URL, map[string]string{
		"X-Trace-Id": "demo",
	})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("status=%d body=%q\n", resp.StatusCode, string(resp.Body))

	// Output:
	// status=200 body="hello, world"
}

// ExampleClient_Post sends a JSON-ish body to a test endpoint and prints the
// echoed response.
func ExampleClient_Post() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintf(w, "created: %s", r.Header.Get("Content-Type"))
	}))
	defer srv.Close()

	cli := httpclient.NewClient(httpclient.ClientOptions{RequestTimeout: 2 * time.Second})
	resp, err := cli.Post(
		context.Background(),
		srv.URL,
		[]byte(`{"name":"alice"}`),
		map[string]string{"Content-Type": "application/json"},
	)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("status=%d body=%q\n", resp.StatusCode, string(resp.Body))

	// Output:
	// status=201 body="created: application/json"
}

// ExampleClient_Metrics shows how to read the client's atomic counters for
// monitoring. After a few calls Metrics returns the accumulated totals.
func ExampleClient_Metrics() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cli := httpclient.NewClient(httpclient.ClientOptions{RetryMax: 0})
	for i := 0; i < 3; i++ {
		if _, err := cli.Get(context.Background(), srv.URL, nil); err != nil {
			fmt.Println("error:", err)
			return
		}
	}
	m := cli.Metrics()
	fmt.Printf("total=%d success=%d failed=%d retried=%d\n", m.Total, m.Success, m.Failed, m.Retried)

	// Output:
	// total=3 success=3 failed=0 retried=0
}
