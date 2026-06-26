package udpclient_test

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/v8fg/kit4go/udpclient"
)

// ExampleNewClient shows the basic construction of a client. A zero-valued
// ClientOptions (apart from Address, which is required) yields sensible
// production defaults: 5s read timeout, 2s write timeout, 4096-byte read buffer
// and up to 2 retries. Override only the fields you need.
func ExampleNewClient() {
	// A throwaway local server so the example is self-contained.
	laddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	conn, err := net.ListenUDP("udp", laddr)
	if err != nil {
		fmt.Println("listen error:", err)
		return
	}
	defer conn.Close()

	cli, err := udpclient.NewClient(udpclient.ClientOptions{
		Address:      conn.LocalAddr().String(),
		WriteTimeout: 500 * time.Millisecond,
		RetryMax:     2,
	})
	if err != nil {
		fmt.Println("new client error:", err)
		return
	}
	defer cli.Close()

	fmt.Println("client ready")

	// Output:
	// client ready
}

// ExampleClient_Send demonstrates fire-and-forget delivery: the client writes a
// datagram and returns without waiting for a reply. This is the typical
// statsd/syslog/telemetry pattern.
func ExampleClient_Send() {
	// Minimal local server that just drains incoming datagrams.
	laddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	conn, err := net.ListenUDP("udp", laddr)
	if err != nil {
		fmt.Println("listen error:", err)
		return
	}
	defer conn.Close()
	go func() {
		buf := make([]byte, 2048)
		for {
			if _, _, err := conn.ReadFromUDP(buf); err != nil {
				return
			}
		}
	}()

	cli, err := udpclient.NewClient(udpclient.ClientOptions{
		Address:      conn.LocalAddr().String(),
		WriteTimeout: 500 * time.Millisecond,
		RetryMax:     0,
	})
	if err != nil {
		fmt.Println("new client error:", err)
		return
	}
	defer cli.Close()

	if err := cli.Send(context.Background(), []byte("page.views:1|c")); err != nil {
		fmt.Println("send error:", err)
		return
	}
	fmt.Println("sent")

	// Output:
	// sent
}
