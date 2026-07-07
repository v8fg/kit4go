package minio

// Metrics is a point-in-time snapshot of Client counters (all atomic loads).
type Metrics struct {
	Puts          uint64 // PutObject calls
	Gets          uint64 // GetObject calls
	Stats         uint64 // StatObject calls
	Removes       uint64 // RemoveObject calls
	Errors        uint64 // any operation returning a non-nil error
	BytesUploaded uint64 // sum of PutObject UploadInfo.Size
}

// Event is fired after each operation when an OnEvent hook is installed.
type Event struct {
	Kind    string // KindPut, KindGet, KindStat, KindRemove, KindBucket, KindList, KindPresign
	Outcome string // OutcomeSuccess or OutcomeError
}

// Event kinds.
const (
	KindPut     = "put"
	KindGet     = "get"
	KindStat    = "stat"
	KindRemove  = "remove"
	KindBucket  = "bucket"  // BucketExists / MakeBucket
	KindList    = "list"    // ListObjects
	KindPresign = "presign" // PresignedGetObject
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
		Puts:          c.puts.Load(),
		Gets:          c.gets.Load(),
		Stats:         c.stats.Load(),
		Removes:       c.removes.Load(),
		Errors:        c.errors.Load(),
		BytesUploaded: c.bytesUploaded.Load(),
	}
}

func (c *Client) fireEvent(e Event) {
	if p := c.onEvent.Load(); p != nil {
		(*p)(e)
	}
}
