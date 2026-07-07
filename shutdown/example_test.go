package shutdown_test

import (
	"context"
	"fmt"

	"github.com/v8fg/kit4go/shutdown"
)

// ExampleManager shows dependency-ordered start and reverse-ordered stop. Each
// component records its stop into a shared slice so we can observe the order.
// "server" depends on "db", so db starts first and stops last.
func ExampleManager() {
	var order []string
	mk := func(name string) (func(context.Context) error, func(context.Context) error) {
		start := func(context.Context) error { order = append(order, "start:"+name); return nil }
		stop := func(context.Context) error { order = append(order, "stop:"+name); return nil }
		return start, stop
	}

	m := shutdown.New()
	dbStart, dbStop := mk("db")
	srvStart, srvStop := mk("server")
	_ = m.Add("db", dbStart, dbStop)
	_ = m.Add("server", srvStart, srvStop, "db") // server depends on db

	ctx := context.Background()
	_ = m.Start(ctx)
	_ = m.Stop(ctx)

	fmt.Println(order)
	// Output: [start:db start:server stop:server stop:db]
}
