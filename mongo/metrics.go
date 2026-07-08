package mongo

// Metrics is a point-in-time snapshot of Client counters (all atomic loads),
// aggregated across all Collections opened from the Client.
type Metrics struct {
	Inserts uint64 // InsertOne + InsertMany calls
	Finds   uint64 // Find + FindOne calls
	Updates uint64 // UpdateOne + UpdateMany calls
	Deletes uint64 // DeleteOne + DeleteMany calls
	Errors  uint64 // any operation returning a non-nil error (FindOne excluded)
}

// Event is fired after each Collection operation when an OnEvent hook is
// installed.
type Event struct {
	Kind    string // KindInsert, KindFind, KindUpdate, KindDelete
	Outcome string // OutcomeSuccess or OutcomeError
}

// Event kinds.
const (
	KindInsert = "insert"
	KindFind   = "find"
	KindUpdate = "update"
	KindDelete = "delete"
)

// Event outcomes.
const (
	OutcomeSuccess = "success"
	OutcomeError   = "error"
)

// SetOnEvent installs a hook fired after each Collection operation (nil disables
// it). The hook runs on the calling goroutine; keep it cheap. When nil, the cost
// is a single atomic-pointer load per operation (effectively zero overhead).
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
		Inserts: c.inserts.Load(),
		Finds:   c.finds.Load(),
		Updates: c.updates.Load(),
		Deletes: c.deletes.Load(),
		Errors:  c.errors.Load(),
	}
}

func (c *Client) fireEvent(e Event) {
	if p := c.onEvent.Load(); p != nil {
		(*p)(e)
	}
}
