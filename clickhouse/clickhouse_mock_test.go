package clickhouse

import (
	"context"
	"errors"
	"testing"

	ch "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/stretchr/testify/require"
)

// mockConn implements the local Conn interface (a subset of driver.Conn) so the
// Client can be unit-tested without a live ClickHouse. It records call counts
// so tests assert pass-through + counter behavior.
type mockConn struct {
	pingErr  error
	execErr  error
	queryErr error
	batchErr error

	pingCalls     int
	execCalls     int
	queryCalls    int
	queryRowCalls int
	batchCalls    int
	statsCalls    int
	closeCalls    int

	rows  driver.Rows
	row   driver.Row
	stats driver.Stats
}

func (m *mockConn) Ping(context.Context) error { m.pingCalls++; return m.pingErr }
func (m *mockConn) Exec(context.Context, string, ...any) error {
	m.execCalls++
	return m.execErr
}
func (m *mockConn) Query(context.Context, string, ...any) (driver.Rows, error) {
	m.queryCalls++
	if m.queryErr != nil {
		return nil, m.queryErr
	}
	return m.rows, nil
}
func (m *mockConn) QueryRow(context.Context, string, ...any) driver.Row {
	m.queryRowCalls++
	return m.row
}
func (m *mockConn) PrepareBatch(context.Context, string, ...driver.PrepareBatchOption) (driver.Batch, error) {
	m.batchCalls++
	if m.batchErr != nil {
		return nil, m.batchErr
	}
	return nil, nil
}
func (m *mockConn) Stats() driver.Stats { m.statsCalls++; return m.stats }
func (m *mockConn) Close() error        { m.closeCalls++; return nil }

// --- minimal Row/Rows stubs for the Query/QueryRow success paths ---

type mockRow struct{ err error }

func (r *mockRow) Err() error           { return r.err }
func (r *mockRow) Scan(...any) error    { return r.err }
func (r *mockRow) ScanStruct(any) error { return r.err }

type mockRows struct{}

func (mockRows) Next() bool                       { return false }
func (mockRows) Scan(...any) error                { return nil }
func (mockRows) ScanStruct(any) error             { return nil }
func (mockRows) ColumnTypes() []driver.ColumnType { return nil }
func (mockRows) Totals(...any) error              { return nil }
func (mockRows) Columns() []string                { return nil }
func (mockRows) Close() error                     { return nil }
func (mockRows) Err() error                       { return nil }
func (mockRows) HasData() bool                    { return false }

// --- Exec ---

func TestExec_Success(t *testing.T) {
	mc := &mockConn{}
	c := newWithConn(mc)
	require.NoError(t, c.Exec(context.Background(), "INSERT INTO t VALUES (1)"))
	require.Equal(t, 1, mc.execCalls)
	require.Equal(t, Metrics{Execs: 1}, c.Metrics())
}

func TestExec_Error(t *testing.T) {
	mc := &mockConn{execErr: errors.New("exec boom")}
	c := newWithConn(mc)
	err := c.Exec(context.Background(), "INSERT INTO t")
	require.Error(t, err)
	require.Equal(t, 1, mc.execCalls)
	require.Equal(t, Metrics{Execs: 1, Errors: 1}, c.Metrics())
}

// --- Query ---

func TestQuery_Success(t *testing.T) {
	want := mockRows{}
	mc := &mockConn{rows: want}
	c := newWithConn(mc)
	rows, err := c.Query(context.Background(), "SELECT 1")
	require.NoError(t, err)
	require.Equal(t, driver.Rows(want), rows)
	require.Equal(t, 1, mc.queryCalls)
	require.Equal(t, Metrics{Queries: 1}, c.Metrics())
}

func TestQuery_Error(t *testing.T) {
	mc := &mockConn{queryErr: errors.New("query boom")}
	c := newWithConn(mc)
	rows, err := c.Query(context.Background(), "SELECT 1")
	require.Error(t, err)
	require.Nil(t, rows)
	require.Equal(t, Metrics{Queries: 1, Errors: 1}, c.Metrics())
}

// --- QueryRow (no error path; error surfaces on Scan/Err) ---

func TestQueryRow(t *testing.T) {
	want := &mockRow{}
	mc := &mockConn{row: want}
	c := newWithConn(mc)
	row := c.QueryRow(context.Background(), "SELECT 1")
	require.Equal(t, 1, mc.queryRowCalls)
	require.Equal(t, driver.Row(want), row)
	require.Equal(t, Metrics{Queries: 1}, c.Metrics())
}

// --- PrepareBatch ---

func TestPrepareBatch_Success(t *testing.T) {
	mc := &mockConn{}
	c := newWithConn(mc)
	batch, err := c.PrepareBatch(context.Background(), "INSERT INTO t")
	require.NoError(t, err)
	require.Nil(t, batch) // mockConn returns nil batch on success
	require.Equal(t, 1, mc.batchCalls)
	require.Equal(t, Metrics{Batches: 1}, c.Metrics())
}

func TestPrepareBatch_Error(t *testing.T) {
	mc := &mockConn{batchErr: errors.New("batch boom")}
	c := newWithConn(mc)
	batch, err := c.PrepareBatch(context.Background(), "INSERT INTO t")
	require.Error(t, err)
	require.Nil(t, batch)
	require.Equal(t, Metrics{Batches: 1, Errors: 1}, c.Metrics())
}

// --- Ping ---

func TestPing_Success(t *testing.T) {
	mc := &mockConn{}
	c := newWithConn(mc)
	require.NoError(t, c.Ping(context.Background()))
	require.Equal(t, 1, mc.pingCalls)
	require.Equal(t, Metrics{Pings: 1}, c.Metrics())
}

func TestPing_Error(t *testing.T) {
	mc := &mockConn{pingErr: errors.New("ping boom")}
	c := newWithConn(mc)
	err := c.Ping(context.Background())
	require.Error(t, err)
	require.Equal(t, 1, mc.pingCalls)
	require.Equal(t, Metrics{Pings: 1, PingErrors: 1, Errors: 1}, c.Metrics())
}

// --- Stats / Conn / Close (lifecycle) ---

func TestStats_Passthrough(t *testing.T) {
	want := driver.Stats{Open: 3, Idle: 2, MaxOpenConns: 10, MaxIdleConns: 5}
	c := newWithConn(&mockConn{stats: want})
	require.Equal(t, want, c.Stats())
}

func TestConn_NilWhenMocked(t *testing.T) {
	require.Nil(t, newWithConn(&mockConn{}).Conn())
}

func TestClose_NoOpWhenNotOwned(t *testing.T) {
	mc := &mockConn{}
	c := newWithConn(mc)
	require.NoError(t, c.Close())
	require.Equal(t, 0, mc.closeCalls, "injected client must not close the mock")
}

// --- OnEvent ---

func TestSetOnEvent_Fires(t *testing.T) {
	mc := &mockConn{}
	c := newWithConn(mc)
	var got []Event
	c.SetOnEvent(func(e Event) { got = append(got, e) })

	require.NoError(t, c.Exec(context.Background(), "q")) // exec success
	require.NoError(t, c.Ping(context.Background()))      // ping success
	mc.execErr = errors.New("x")
	require.Error(t, c.Exec(context.Background(), "q2")) // exec error

	require.Equal(t, []Event{
		{Kind: KindExec, Outcome: OutcomeSuccess},
		{Kind: KindPing, Outcome: OutcomeSuccess},
		{Kind: KindExec, Outcome: OutcomeError},
	}, got)
}

func TestSetOnEvent_NilNoPanic(t *testing.T) {
	c := newWithConn(&mockConn{})
	c.SetOnEvent(nil)
	require.NotPanics(t, func() {
		_ = c.Exec(context.Background(), "q")
		_ = c.Ping(context.Background())
	})
}

// --- defaults + protocol mapping (internal) ---

func TestWithDefaults_DatabaseDefault(t *testing.T) {
	o := withDefaults(nil)
	require.Equal(t, "default", o.Database)
	require.Equal(t, ProtocolNative, o.Protocol) // zero value
}

func TestWithDefaults_KeepsExplicitDatabase(t *testing.T) {
	o := withDefaults([]Option{WithDatabase("mydb")})
	require.Equal(t, "mydb", o.Database)
}

func TestToDriverProtocol(t *testing.T) {
	require.Equal(t, ch.Native, toDriverProtocol(ProtocolNative))
	require.Equal(t, ch.HTTP, toDriverProtocol(ProtocolHTTP))
}

// --- newClient (New's real open/ping/close paths, open injected) ---

// fullMockConn satisfies the full driver.Conn via an embedded nil interface;
// only Ping/Close are reached on the newClient path. Lets tests cover New's
// real open/ping/close branches without a live ClickHouse.
type fullMockConn struct {
	driver.Conn
	pingErr               error
	pingCalls, closeCalls int
}

func (m *fullMockConn) Ping(context.Context) error { m.pingCalls++; return m.pingErr }
func (m *fullMockConn) Close() error               { m.closeCalls++; return nil }

func TestNewClient_Success(t *testing.T) {
	mc := &fullMockConn{}
	open := func(_ *ch.Options) (driver.Conn, error) { return mc, nil }
	c, err := newClient(context.Background(), []Option{WithAddrs("h:9000"), WithDatabase("db")}, open)
	require.NoError(t, err)
	require.Equal(t, 1, mc.pingCalls, "construction pings once")
	require.NotNil(t, c.Conn(), "real-opened client exposes the driver.Conn")
	require.Equal(t, []string{"h:9000"}, c.Options().Addrs)
	require.Equal(t, "db", c.Options().Database)
	require.NoError(t, c.Close()) // own=true -> closes the conn
	require.Equal(t, 1, mc.closeCalls)
}

func TestNewClient_OpenError(t *testing.T) {
	open := func(_ *ch.Options) (driver.Conn, error) { return nil, errors.New("open boom") }
	_, err := newClient(context.Background(), []Option{WithAddrs("h:9000")}, open)
	require.Error(t, err)
}

func TestNewClient_PingError_ClosesConn(t *testing.T) {
	mc := &fullMockConn{pingErr: errors.New("ping boom")}
	open := func(_ *ch.Options) (driver.Conn, error) { return mc, nil }
	_, err := newClient(context.Background(), []Option{WithAddrs("h:9000")}, open)
	require.Error(t, err)
	require.Equal(t, 1, mc.pingCalls)
	require.Equal(t, 1, mc.closeCalls, "failed Ping must close the conn")
}
