package httpclient

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// bufPool reuses bytes.Buffer across drainBody calls to reduce per-request
// allocations. The buffer is Reset before each use and returned after.
var bufPool = sync.Pool{
	New: func() any { return new(bytes.Buffer) },
}

// respPool reuses Response structs. Callers that are done with a Response
// should call Response.Release() to return it to the pool; if they don't,
// the GC collects it normally (the pool is just an optimization).
var respPool = sync.Pool{
	New: func() any { return new(Response) },
}

// Response is the fully-materialised result of an HTTP call. The body is read
// into memory (and the underlying [http.Response.Body] closed) before this
// struct is returned, so callers do not need to worry about streaming or
// closing. For very large payloads prefer driving the [http.Client] directly.
type Response struct {
	// StatusCode is the HTTP status code of the final response (after any
	// redirect following). 0 if the request failed before a status was
	// received.
	StatusCode int

	// Header holds the response headers.
	Header http.Header

	// Body is the response payload. nil for responses with no body. It is a
	// snapshot taken at return time; mutating it does not affect the client.
	Body []byte

	// pooled indicates the Response came from respPool. Release() returns it.
	pooled bool
}

// Release returns the Response to the internal pool for reuse. Call this when
// you are done reading StatusCode/Header/Body to reduce GC pressure on hot
// paths. After Release, the Response and its fields must not be accessed.
// Calling Release on a Response not obtained from the pool is a no-op.
func (r *Response) Release() {
	if r == nil || !r.pooled {
		return
	}
	r.StatusCode = 0
	r.Header = nil
	r.Body = nil
	r.pooled = false
	respPool.Put(r)
}

// ClientMetrics is a point-in-time snapshot of the counters maintained by a
// [Client]. Values are gathered via atomic loads and may be slightly
// inconsistent with one another under concurrent load; that is acceptable for
// monitoring/observability use.
type ClientMetrics struct {
	// Total is the number of calls observed, regardless of outcome.
	Total uint64

	// Success is the number of calls whose final response had a 2xx status.
	Success uint64

	// Failed is the number of calls that did not end in a 2xx status: this
	// covers transport errors and non-2xx responses alike.
	Failed uint64

	// Retried is the total number of retry attempts made (not counting the
	// initial attempt). A call that required two retries contributes 2 here.
	Retried uint64
}

// ClientEvent is passed to the hook installed via [Client.SetOnEvent] for every
// notable outcome of a request lifecycle. It is the integration point for
// metrics push (Prometheus counters/histograms, tracing spans, etc.), mirroring
// the hook pattern used by the breaker package and log4go.
//
// Name is one of:
//   - "request": an attempt was sent (one per send, including retries).
//   - "retry":   an attempt failed and will be retried (fires before backoff).
//   - "success": the call completed with a 2xx final response.
//   - "failed":  the call completed without a 2xx response (transport error or
//     non-2xx status), or could not be sent at all.
//
// Attempt is the 0-indexed attempt number this event pertains to (0 = the
// initial send). StatusCode is the HTTP status of the attempt for "request"
// events where a response was received, and of the final response for
// "success"/"failed"; 0 when no response was obtained.
type ClientEvent struct {
	Name       string
	Method     string
	URL        string
	StatusCode int
	Attempt    int
}

// Client is a production-grade HTTP client wrapping [http.Client] with retry,
// per-request/per-dial timeouts, connection pooling and optional
// circuit-breaker integration. The zero value is not usable; construct one with
// [NewClient].
//
// All methods are safe for concurrent use by multiple goroutines.
type Client struct {
	httpCli *http.Client
	opts    ClientOptions

	// Counters are laid out as separate atomics rather than a single packed
	// struct so increments don't contend on the same cache line.
	total   atomic.Uint64
	success atomic.Uint64
	failed  atomic.Uint64
	retried atomic.Uint64

	// onEvent, when non-nil, is invoked for every notable request outcome
	// (request, retry, success, failed). Set via SetOnEvent and read with an
	// atomic load, so the default (nil) is zero-overhead on the hot path.
	onEvent atomic.Pointer[func(ClientEvent)]
}

// SetOnEvent installs a hook invoked for every notable request lifecycle event.
// fn receives a [ClientEvent] describing the attempt and its outcome. Pass nil
// to disable a previously-installed hook.
//
// The hook is intended for metrics/tracing and must be cheap and non-blocking:
// it fires synchronously on the goroutine issuing the request, inside Do/Do.
// Install it once at construction time (before traffic) for the cleanest
// ordering; SetOnEvent is nevertheless safe to call concurrently with in-flight
// requests.
func (c *Client) SetOnEvent(fn func(evt ClientEvent)) {
	if fn == nil {
		c.onEvent.Store(nil)
		return
	}
	f := fn // copy to heap
	c.onEvent.Store(&f)
}

// fireEvent is the single chokepoint for hook dispatch. When onEvent is nil
// (the default) the call collapses to a single nil compare, so the no-hook hot
// path is unaffected.
func (c *Client) fireEvent(name, method, url string, status, attempt int) {
	if p := c.onEvent.Load(); p != nil {
		(*p)(ClientEvent{
			Name:       name,
			Method:     method,
			URL:        url,
			StatusCode: status,
			Attempt:    attempt,
		})
	}
}

// NewClient constructs a [Client] from opts, filling zero fields with the
// package defaults. It builds a single shared [http.Transport] sized by the
// connection-pool options and wires the connect timeout into the dialer.
//
// The returned client is safe for concurrent use and ready to serve traffic.
func NewClient(opts ClientOptions) *Client {
	opts = opts.withDefaults()

	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: opts.ConnectTimeout,
		}).DialContext,
		MaxIdleConns:        opts.MaxIdleConns,
		MaxIdleConnsPerHost: opts.MaxIdlePerHost,
		IdleConnTimeout:     opts.IdleConnTimeout,
	}

	httpCli := &http.Client{
		Transport: transport,
		Timeout:   0, // per-request timeout is applied via context in Do
	}
	if !opts.FollowRedirect {
		httpCli.CheckRedirect = func(*http.Request, []*http.Request) error {
			// Use http.ErrUseLastResponse so the [http.Client] returns the most
			// recent response (the 3xx) to the caller instead of following.
			return http.ErrUseLastResponse
		}
	}

	return &Client{
		httpCli: httpCli,
		opts:    opts,
	}
}

// Do sends a request with the given method, URL, body and headers, applying
// retry, per-request timeout and (if configured) circuit-breaker integration.
//
// The body, if non-nil, is sent as the request body verbatim and re-read on
// each retry (a fresh [bytes.Reader] is constructed per attempt). headers is
// applied on top of the default header set; pass nil to skip.
//
// The returned [Response] always has its Body fully read and closed. On error
// the returned Response may be nil or may carry a partial StatusCode; callers
// branching on err should treat a non-nil err as a failure regardless of
// Response. Metrics (total/success/failed) are updated for every call.
//
// ctx, if it carries a deadline earlier than RequestTimeout, wins — the per-
// request timeout is only applied when the incoming ctx has none.
//
// RequestTimeout is applied here via context.WithTimeout so the cancel func is
// owned by Do and lives long enough for the response body to be fully read
// (and the underlying connection returned to the pool) before the context is
// torn down. DoWithRetry honours the deadline encoded on the request's context.
func (c *Client) Do(ctx context.Context, method, url string, body []byte, headers map[string]string) (*Response, error) {
	c.total.Add(1)

	if ctx == nil {
		ctx = context.Background()
	}
	// Apply the per-request timeout only when the caller's ctx lacks a
	// deadline; this lets callers impose tighter deadlines than the configured
	// RequestTimeout without being overridden. The cancel is deferred to the
	// end of Do (after drainBody) so the body stream stays alive.
	if _, hasDL := ctx.Deadline(); !hasDL && c.opts.RequestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.opts.RequestTimeout)
		defer cancel()
	}

	// Build the request up front so a malformed method/URL fails fast with a
	// real error rather than panicking inside DoWithRetry.
	req, bErr := c.buildRequest(ctx, method, url, body, headers)
	if bErr != nil {
		c.failed.Add(1)
		c.fireEvent("failed", method, url, 0, 0)
		return nil, fmt.Errorf("httpclient: build request: %w", bErr)
	}

	var (
		raw *http.Response
		err error
	)

	doFn := func(ctx context.Context) error {
		raw, err = c.DoWithRetry(ctx, req)
		return err
	}

	if c.opts.Breaker != nil {
		err = c.opts.Breaker.Execute(ctx, doFn)
	} else {
		err = doFn(ctx)
	}

	// If the round-trip never produced a usable response, count it as failed.
	if err != nil {
		c.failed.Add(1)
		// Drain/close raw body if one happens to be attached.
		if raw != nil && raw.Body != nil {
			_, _ = io.Copy(io.Discard, raw.Body)
			_ = raw.Body.Close()
		}
		c.fireEvent("failed", method, url, 0, 0)
		return nil, err
	}

	// Read the body fully into memory and close it so the connection can be
	// returned to the pool. This must happen before the deferred cancel above
	// fires, hence the body-read lives inside Do rather than after it returns.
	respBody, readErr := drainBody(raw)
	if readErr != nil {
		c.failed.Add(1)
		c.fireEvent("failed", method, url, raw.StatusCode, 0)
		return nil, fmt.Errorf("httpclient: read response body: %w", readErr)
	}

	resp := respPool.Get().(*Response)
	resp.StatusCode = raw.StatusCode
	resp.Header = raw.Header
	resp.Body = respBody
	resp.pooled = true

	if raw.StatusCode >= 200 && raw.StatusCode < 300 {
		c.success.Add(1)
		c.fireEvent("success", method, url, raw.StatusCode, 0)
	} else {
		c.failed.Add(1)
		c.fireEvent("failed", method, url, raw.StatusCode, 0)
	}
	return resp, nil
}

// Get issues a GET request. See [Client.Do] for semantics.
func (c *Client) Get(ctx context.Context, url string, headers map[string]string) (*Response, error) {
	return c.Do(ctx, http.MethodGet, url, nil, headers)
}

// Post issues a POST request with the given body. See [Client.Do] for
// semantics.
func (c *Client) Post(ctx context.Context, url string, body []byte, headers map[string]string) (*Response, error) {
	return c.Do(ctx, http.MethodPost, url, body, headers)
}

// Put issues a PUT request with the given body. See [Client.Do] for semantics.
func (c *Client) Put(ctx context.Context, url string, body []byte, headers map[string]string) (*Response, error) {
	return c.Do(ctx, http.MethodPut, url, body, headers)
}

// Delete issues a DELETE request. See [Client.Do] for semantics.
func (c *Client) Delete(ctx context.Context, url string, headers map[string]string) (*Response, error) {
	return c.Do(ctx, http.MethodDelete, url, nil, headers)
}

// DoWithRetry is the core round-trip method. It sends req via the underlying
// [http.Client], retrying transient failures (5xx responses and network errors)
// up to [ClientOptions.RetryMax] times with exponential backoff and jitter
// computed by [retryDelay].
//
// The per-request RequestTimeout is NOT applied here — [Client.Do] owns that so
// the cancel func outlives the response-body read. DoWithRetry simply honours
// whatever deadline the request's context carries.
//
// The body of req, if backed by an io.Reader, must be re-readable on each
// attempt — callers using [Client.Do] get this for free via buildRequest. The
// returned [http.Response] is handed to the caller unclosed and unread; it is
// the caller's responsibility to drain and close it (Client.Do does so). This
// holds even when every attempt was retryable: the last response's body is left
// open so the caller can surface its status and body.
//
// Metrics note: the retried counter is incremented here for each retry
// actually attempted.
func (c *Client) DoWithRetry(ctx context.Context, req *http.Request) (*http.Response, error) {
	var (
		resp    *http.Response
		err     error
		attempt int
	)
	// Capture method/URL once for event dispatch; req.WithContext returns a
	// shallow copy that preserves these, so reading them up front is safe.
	method := req.Method
	urlStr := req.URL.String()
	for attempt = 0; attempt <= c.opts.RetryMax; attempt++ {
		// Reset the request body before each attempt so retries can re-read it.
		if req.Body != nil && req.GetBody != nil {
			if newBody, gErr := req.GetBody(); gErr == nil {
				req.Body = newBody
			}
		}
			req = req.WithContext(ctx) // shallow copy, cheap

		resp, err = c.httpCli.Do(req)

		// Fire a "request" event for every attempt sent (success or fail).
		// StatusCode is 0 when no response was obtained.
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		c.fireEvent("request", method, urlStr, status, attempt)

		if !shouldRetry(resp, err) {
			// Either a non-retryable error or a final (success/4xx/3xx)
			// response — hand it back (body unread/unclosed).
			return resp, err
		}

		// Retryable. If this was the last allowed attempt, stop WITHOUT closing
		// the body so the caller (Do) can still drain it for status/body
		// surfacing. The body-close then happens in drainBody.
		if attempt == c.opts.RetryMax {
			break
		}

		// More attempts remain: drain and close this attempt's partial body so
		// the underlying connection can be returned to the pool, then back off.
		if resp != nil && resp.Body != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}

		c.retried.Add(1)
		// Fire a "retry" event before backoff so observers can attribute the
		// wait. The next attempt number will be attempt+1.
		c.fireEvent("retry", method, urlStr, status, attempt+1)

		// Honour ctx cancellation during the backoff sleep.
		delay := retryDelay(attempt, c.opts.RetryWaitMin, c.opts.RetryWaitMax)
		select {
		case <-ctx.Done():
			// ctx cancelled while waiting: surface the last transport error if
			// present, otherwise the ctx error.
			if err == nil {
				err = ctx.Err()
			}
			return resp, err
		case <-time.After(delay):
		}
	}
	return resp, err
}

// Metrics returns a point-in-time snapshot of the client's counters.
func (c *Client) Metrics() ClientMetrics {
	return ClientMetrics{
		Total:   c.total.Load(),
		Success: c.success.Load(),
		Failed:  c.failed.Load(),
		Retried: c.retried.Load(),
	}
}

// buildRequest constructs an [*http.Request] for the given method/URL/body/
// headers and attaches a GetBody hook so retry can re-read the body.
func (c *Client) buildRequest(ctx context.Context, method, url string, body []byte, headers map[string]string) (*http.Request, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		bodyCopy := body
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(bodyCopy)), nil
		}
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return req, nil
}

// drainBody reads b.Body fully into a []byte and closes it. Uses a pooled
// buffer to avoid allocating a new bytes.Buffer on every call.
func drainBody(b *http.Response) ([]byte, error) {
	if b == nil || b.Body == nil {
		return nil, nil
	}
	defer func() { _ = b.Body.Close() }()
	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufPool.Put(buf)
	_, err := buf.ReadFrom(b.Body)
	if err != nil {
		return nil, err
	}
	if buf.Len() == 0 {
		return nil, nil
	}
	// Append-copy so the returned slice does not alias the pooled buffer.
	out := make([]byte, buf.Len())
	copy(out, buf.Bytes())
	return out, nil
}
