package grpcclient_test

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/wrapperspb"

	grpcclient "github.com/v8fg/kit4go/grpcclient"
)

// ExampleNewMiddleware shows the basic construction of a middleware. A zero
// ClientOptions yields sensible production defaults (5s connect, 10s request
// timeout, up to 2 retries on Unavailable/DeadlineExceeded). Override only the
// fields you need, then pass the interceptors to grpc.Dial.
func ExampleNewMiddleware() {
	mw := grpcclient.NewMiddleware(grpcclient.ClientOptions{
		Target:         "localhost:50051",
		RequestTimeout: 5 * time.Second,
		RetryMax:       2,
		RetryWaitMin:   50 * time.Millisecond,
		RetryWaitMax:   time.Second,
	})
	_ = mw
	fmt.Println("middleware ready")

	// Output:
	// middleware ready
}

// ExampleNewMiddleware_dial wires the middleware's interceptors onto a real
// (here: would-be) *grpc.ClientConn. In production you would then build
// generated stubs from conn and call them as usual; the interceptors apply
// retry, timeout, metrics and breaker transparently.
func ExampleNewMiddleware_dial() {
	mw := grpcclient.NewMiddleware(grpcclient.ClientOptions{
		Target:         "localhost:50051",
		RequestTimeout: 5 * time.Second,
	})
	// We don't actually dial a live server in this example, so we construct the
	// dial options to show the wiring. grpc.Dial is non-blocking by default, so
	// this returns a conn whose connection comes up lazily on first RPC.
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(mw.UnaryClientInterceptor()),
		grpc.WithStreamInterceptor(mw.StreamClientInterceptor()),
	}
	_ = opts
	fmt.Println("dial options wired")

	// Output:
	// dial options wired
}

// ExampleDialConn uses the convenience constructor that dials with insecure
// credentials and the middleware's interceptors already attached. With an empty
// Target it surfaces a clear error rather than panicking.
func ExampleDialConn() {
	// A realistic call would pass a real Target and then use the returned conn
	// with generated stubs:
	//
	//   conn, err := grpcclient.DialConn(grpcclient.ClientOptions{
	//       Target: "localhost:50051",
	//   })
	//   if err != nil { return err }
	//   defer conn.Close()
	//   cli := pb.NewEchoerClient(conn)
	//   resp, err := cli.Echo(ctx, &pb.StringValue{Value: "hi"})
	//
	// Here we demonstrate the empty-Target guard, which is cheap to exercise
	// without a live server.
	_, err := grpcclient.DialConn(grpcclient.ClientOptions{Target: ""})
	fmt.Println("error:", err)

	// Output:
	// error: grpcclient: empty Target in ClientOptions
}

// ExampleMiddleware_Metrics shows how to read the middleware's atomic counters
// for monitoring. Because we have no live server here, we drive a single
// invocation through the interceptors via a minimal in-process round to show
// the shape of the returned snapshot. See grpcclient_test.go for full coverage
// with a bufconn server.
func ExampleMiddleware_Metrics() {
	srv := newEchoServer()
	dialer, shutdown := startTestServer(srv)
	defer shutdown()

	mw := grpcclient.NewMiddleware(grpcclient.ClientOptions{RetryMax: 0})
	conn := dialBufconn(fakeT{}, dialer, mw)
	defer conn.Close()

	for range 3 {
		if _, err := echoUnary(context.Background(), conn, wrapperspb.String("ping")); err != nil {
			fmt.Println("error:", err)
			return
		}
	}
	m := mw.Metrics()
	fmt.Printf("total=%d success=%d failed=%d retried=%d\n", m.Total, m.Success, m.Failed, m.Retried)

	// Output:
	// total=3 success=3 failed=0 retried=0
}

// fakeT is a no-op *testing.T stand-in so Example functions (which cannot
// receive a *testing.T) can reuse dialBufconn via the testingTB interface.
type fakeT struct{}

func (fakeT) Helper() {}
func (t fakeT) Fatalf(format string, args ...any) {
	fmt.Printf("fatal: "+format+"\n", args...)
}
