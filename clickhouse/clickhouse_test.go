package clickhouse_test

import (
	"context"
	"crypto/tls"
	"testing"
	"time"

	chdriver "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/stretchr/testify/require"

	"github.com/v8fg/kit4go/clickhouse"
)

func TestNew_NoAddrs(t *testing.T) {
	_, err := clickhouse.New(context.Background())
	require.ErrorIs(t, err, clickhouse.ErrNoAddrs)
}

func TestOptions_AllWith(t *testing.T) {
	var o clickhouse.Options
	for _, opt := range []clickhouse.Option{
		clickhouse.WithAddrs("h1:9000", "h2:9000"),
		clickhouse.WithProtocol(clickhouse.ProtocolHTTP),
		clickhouse.WithDatabase("db"),
		clickhouse.WithUsername("u"),
		clickhouse.WithPassword("p"),
		clickhouse.WithTLSConfig(&tls.Config{InsecureSkipVerify: true}),
		clickhouse.WithDialTimeout(2 * time.Second),
		clickhouse.WithMaxOpenConns(8),
		clickhouse.WithMaxIdleConns(4),
		clickhouse.WithConnMaxLifetime(time.Hour),
		clickhouse.WithSettings(chdriver.Settings{"max_execution_time": 60}),
		clickhouse.WithCompression(&chdriver.Compression{Method: chdriver.CompressionLZ4}),
		clickhouse.WithConnOpenStrategy(chdriver.ConnOpenRoundRobin),
		clickhouse.WithDebug(true),
	} {
		opt(&o)
	}
	require.Equal(t, []string{"h1:9000", "h2:9000"}, o.Addrs)
	require.Equal(t, clickhouse.ProtocolHTTP, o.Protocol)
	require.Equal(t, "db", o.Database)
	require.Equal(t, "u", o.Username)
	require.Equal(t, "p", o.Password)
	require.NotNil(t, o.TLSConfig)
	require.Equal(t, 2*time.Second, o.DialTimeout)
	require.Equal(t, 8, o.MaxOpenConns)
	require.Equal(t, 4, o.MaxIdleConns)
	require.Equal(t, time.Hour, o.ConnMaxLifetime)
	require.Equal(t, chdriver.Settings{"max_execution_time": 60}, o.Settings)
	require.Equal(t, &chdriver.Compression{Method: chdriver.CompressionLZ4}, o.Compression)
	require.Equal(t, chdriver.ConnOpenRoundRobin, o.ConnOpenStrategy)
	require.True(t, o.Debug)
}

// driverConnStub satisfies driver.Conn via an embedded nil interface — its
// methods are never called because Wrap does not own the conn (Close is no-op).
type driverConnStub struct{ driver.Conn }

func TestWrap_DoesNotOwn(t *testing.T) {
	c := clickhouse.Wrap(&driverConnStub{})
	require.NotNil(t, c.Conn(), "Wrap must expose the raw driver.Conn")
	require.NotPanics(t, func() { _ = c.Close() }, "Close on a wrapped client must be a no-op")
}
