package etcd

// Metrics is a point-in-time snapshot of Client counters (all atomic loads).
type Metrics struct {
	Puts    uint64 // Put calls
	Gets    uint64 // Get calls
	Deletes uint64 // Delete calls
	Grants  uint64 // Grant calls
	Watches uint64 // Watch subscriptions started
	Errors  uint64 // any operation returning a non-nil error
}

// Event is fired after each operation when an OnEvent hook is installed.
type Event struct {
	Kind    string // KindPut, KindGet, KindDelete, KindGrant, KindKeepAlive, KindRevoke, KindWatch, KindStatus
	Outcome string // OutcomeSuccess or OutcomeError
}

// Event kinds.
const (
	KindPut       = "put"
	KindGet       = "get"
	KindDelete    = "delete"
	KindGrant     = "grant"
	KindKeepAlive = "keepalive"
	KindRevoke    = "revoke"
	KindWatch     = "watch"
	KindStatus    = "status"
)

// Event outcomes.
const (
	OutcomeSuccess = "success"
	OutcomeError   = "error"
)

// SetOnEvent installs a hook fired after each operation (nil disables it). The
// hook runs on the calling goroutine; keep it cheap and must not panic (a panic
// propagates to the caller — the wrapper does not recover). When nil, the cost is a
// single atomic-pointer load per operation (effectively zero overhead).
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
		Puts:    c.puts.Load(),
		Gets:    c.gets.Load(),
		Deletes: c.deletes.Load(),
		Grants:  c.grants.Load(),
		Watches: c.watches.Load(),
		Errors:  c.errors.Load(),
	}
}

func (c *Client) fireEvent(e Event) {
	if p := c.onEvent.Load(); p != nil {
		(*p)(e)
	}
}
