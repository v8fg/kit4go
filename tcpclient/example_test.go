package tcpclient_test

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/v8fg/kit4go/tcpclient"
)

// startEcho starts a localhost TCP listener that, for each accepted
// connection, reads a single chunk, echoes it back, and closes the connection.
// The close is what lets the client's SendReceive (which reads until EOF)
// return, so the example produces deterministic output. It returns the
// listener's "host:port" address.
func startEcho() string {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 4*1024)
				n, err := c.Read(buf)
				if err != nil || n == 0 {
					return
				}
				_, _ = c.Write(buf[:n])
			}(conn)
		}
	}()
	return ln.Addr().String()
}

// ExampleNewClient shows the basic construction of a client. A zero
// ClientOptions yields sensible production defaults (5s connect, 10s read,
// pool of 10, up to 2 retries); override only the fields you need. Address is
// the one field you almost always set explicitly.
func ExampleNewClient() {
	addr := startEcho()
	cli := tcpclient.NewClient(tcpclient.ClientOptions{
		Network:  "tcp",
		Address:  addr,
		PoolSize: 4,
		RetryMax: 1,
	})
	defer cli.Close()
	fmt.Println("client ready")

	// Output:
	// client ready
}

// ExampleClient_Send writes a payload to the server. Send does not read a
// response, so it suits fire-and-forget or request-without-reply protocols.
func ExampleClient_Send() {
	addr := startEcho()
	cli := tcpclient.NewClient(tcpclient.ClientOptions{
		Address:      addr,
		WriteTimeout: time.Second,
	})
	defer cli.Close()

	if err := cli.Send(context.Background(), []byte("HELO\n")); err != nil {
		fmt.Println("send error:", err)
		return
	}
	fmt.Println("sent")

	// Output:
	// sent
}

// ExampleClient_SendReceive writes a payload and reads the full response back.
// For an echo server the reply equals the request.
func ExampleClient_SendReceive() {
	addr := startEcho()
	cli := tcpclient.NewClient(tcpclient.ClientOptions{
		Address:      addr,
		ReadTimeout:  time.Second,
		WriteTimeout: time.Second,
		RetryMax:     1,
	})
	defer cli.Close()

	reply, err := cli.SendReceive(context.Background(), []byte("PING"))
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("reply=%q\n", string(reply))

	// Output:
	// reply="PING"
}
