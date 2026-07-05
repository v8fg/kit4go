package clickhouse

// Metrics is a point-in-time snapshot of Client counters (all atomic loads).
type Metrics struct {
	Queries    uint64 // Query + QueryRow calls
	Execs      uint64 // Exec calls
	Batches    uint64 // PrepareBatch calls
	Errors     uint64 // any operation returning a non-nil error
	Pings      uint64 // Ping calls
	PingErrors uint64 // Ping calls that failed
}

// Event is fired after each operation when an OnEvent hook is installed.
type Event struct {
	Kind    string // KindQuery, KindExec, KindBatch, or KindPing
	Outcome string // OutcomeSuccess or OutcomeError
}

// Event kinds.
const (
	KindQuery = "query"
	KindExec  = "exec"
	KindBatch = "batch"
	KindPing  = "ping"
)

// Event outcomes.
const (
	OutcomeSuccess = "success"
	OutcomeError   = "error"
)

// SetOnEvent installs a hook fired after each operation (nil disables it).
// The hook runs on the calling goroutine; keep it cheap. When nil, the cost is
// a single atomic-pointer load per operation (effectively zero overhead).
func (c *Client) SetOnEvent(fn func(Event)) {
	if fn == nil {
		c.onEvent.Store(nil)
		return
	}
	c.onEvent.Store(&fn)
}

// Metrics returns a snapshot of the counters.
func (c *Client) Metrics() Metrics {
	return Metrics{
		Queries:    c.queries.Load(),
		Execs:      c.execs.Load(),
		Batches:    c.batches.Load(),
		Errors:     c.errors.Load(),
		Pings:      c.pings.Load(),
		PingErrors: c.pingErrors.Load(),
	}
}

func (c *Client) fireEvent(e Event) {
	if p := c.onEvent.Load(); p != nil {
		(*p)(e)
	}
}
