package elasticsearch

// Metrics is a point-in-time snapshot of Client counters (all atomic loads).
type Metrics struct {
	Indexes  uint64 // Index calls
	Searches uint64 // Search calls
	Gets     uint64 // Get calls
	Deletes  uint64 // Delete calls
	Errors   uint64 // any operation returning a transport error (err != nil)
}

// Event is fired after each operation when an OnEvent hook is installed.
type Event struct {
	Kind    string // KindIndex, KindSearch, KindGet, KindDelete
	Outcome string // OutcomeSuccess or OutcomeError
}

// Event kinds.
const (
	KindIndex  = "index"
	KindSearch = "search"
	KindGet    = "get"
	KindDelete = "delete"
)

// Event outcomes.
const (
	OutcomeSuccess = "success"
	OutcomeError   = "error"
)

// SetOnEvent installs a hook fired after each operation (nil disables it). The
// hook runs on the calling goroutine; keep it cheap. When nil, the cost is a
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
		Indexes:  c.indexes.Load(),
		Searches: c.searches.Load(),
		Gets:     c.gets.Load(),
		Deletes:  c.deletes.Load(),
		Errors:   c.errors.Load(),
	}
}

func (c *Client) fireEvent(e Event) {
	if p := c.onEvent.Load(); p != nil {
		(*p)(e)
	}
}
