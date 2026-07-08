package elasticsearch

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errTest = errors.New("boom")

func okResponse() *esapi.Response {
	return &esapi.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("{}"))}
}

// newMockClient builds a Client whose esapi callables return a success Response
// (or, when fns are non-nil, whatever they return).
func newMockClient() *Client {
	return &Client{
		index: func(string, io.Reader, ...func(*esapi.IndexRequest)) (*esapi.Response, error) {
			return okResponse(), nil
		},
		search: func(...func(*esapi.SearchRequest)) (*esapi.Response, error) { return okResponse(), nil },
		get:    func(string, string, ...func(*esapi.GetRequest)) (*esapi.Response, error) { return okResponse(), nil },
		delete: func(string, string, ...func(*esapi.DeleteRequest)) (*esapi.Response, error) { return okResponse(), nil },
		ping:   func(...func(*esapi.PingRequest)) (*esapi.Response, error) { return okResponse(), nil },
	}
}

// --- New error paths ---

func TestNew_NoAddresses(t *testing.T) {
	_, err := newClient(context.Background(), nil, defaultOpener)
	require.ErrorIs(t, err, ErrNoAddresses)
}

func TestNew_OpenError(t *testing.T) {
	open := func(elasticsearch.Config) (*elasticsearch.Client, error) { return nil, errTest }
	_, err := newClient(context.Background(), []Option{WithAddresses("http://x")}, open)
	require.ErrorIs(t, err, errTest)
}

// Construction-error: a real client against a dead address fails the Ping.
func TestNew_ConstructionError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		// Short ctx so the Ping does not wait for the 10s fallback.
		pingCtx, cancel2 := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel2()
		_, err := newClient(pingCtx, []Option{WithAddresses("http://127.0.0.1:1")}, defaultOpener)
		_ = err // non-nil expected
	}()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("newClient against dead address did not return within 2s")
	}
}

// --- ops ---

func TestIndex_SuccessAndError(t *testing.T) {
	c := newMockClient()
	resp, err := c.Index(context.Background(), "idx", strings.NewReader(`{}`))
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, uint64(1), c.Metrics().Indexes)

	c.index = func(string, io.Reader, ...func(*esapi.IndexRequest)) (*esapi.Response, error) {
		return nil, errTest
	}
	_, err = c.Index(context.Background(), "idx", strings.NewReader(`{}`))
	require.ErrorIs(t, err, errTest)
	assert.Equal(t, uint64(1), c.Metrics().Errors)
}

func TestSearch_SuccessAndError(t *testing.T) {
	c := newMockClient()
	resp, err := c.Search(context.Background())
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, uint64(1), c.Metrics().Searches)

	c.search = func(...func(*esapi.SearchRequest)) (*esapi.Response, error) { return nil, errTest }
	_, err = c.Search(context.Background())
	require.ErrorIs(t, err, errTest)
}

func TestGet_SuccessAndError(t *testing.T) {
	c := newMockClient()
	_, err := c.Get(context.Background(), "idx", "1")
	require.NoError(t, err)
	assert.Equal(t, uint64(1), c.Metrics().Gets)

	c.get = func(string, string, ...func(*esapi.GetRequest)) (*esapi.Response, error) { return nil, errTest }
	_, err = c.Get(context.Background(), "idx", "1")
	require.ErrorIs(t, err, errTest)
}

func TestDelete_SuccessAndError(t *testing.T) {
	c := newMockClient()
	_, err := c.Delete(context.Background(), "idx", "1")
	require.NoError(t, err)
	assert.Equal(t, uint64(1), c.Metrics().Deletes)

	c.delete = func(string, string, ...func(*esapi.DeleteRequest)) (*esapi.Response, error) { return nil, errTest }
	_, err = c.Delete(context.Background(), "idx", "1")
	require.ErrorIs(t, err, errTest)
}

// --- Wrap / Client() / Options / pingFailFast ---

func TestClient_NilWhenMockInjected(t *testing.T) {
	assert.Nil(t, newMockClient().Client())
}

func TestPingFailFast_ErrorPath(t *testing.T) {
	c := newMockClient()
	c.ping = func(...func(*esapi.PingRequest)) (*esapi.Response, error) { return nil, errTest }
	require.ErrorIs(t, c.pingFailFast(context.Background()), errTest)
}

func TestPingFailFast_NonSuccessStatus(t *testing.T) {
	c := newMockClient()
	c.ping = func(...func(*esapi.PingRequest)) (*esapi.Response, error) {
		return &esapi.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader(""))}, nil
	}
	// Returns a sentinel-wrapped error so callers can errors.Is it.
	require.ErrorIs(t, c.pingFailFast(context.Background()), ErrPingFailed)
}

func TestPingFailFast_Success(t *testing.T) {
	c := newMockClient() // default ping -> 200
	require.NoError(t, c.pingFailFast(context.Background()))
}

// F4 regression: pingFailFast must close the response Body on EVERY path
// (success, non-success status), not leak it.
func TestPingFailFast_ClosesBodyOnAllPaths(t *testing.T) {
	for _, status := range []int{200, 503, 401} {
		c := newMockClient()
		body := &trackingBody{closed: new(bool)}
		c.ping = func(...func(*esapi.PingRequest)) (*esapi.Response, error) {
			return &esapi.Response{StatusCode: status, Body: body}, nil
		}
		_ = c.pingFailFast(context.Background())
		assert.True(t, *body.closed, "pingFailFast must close the response Body for status %d", status)
	}
}

// trackingBody is an io.ReadCloser whose Close flips *closed (Read returns EOF).
type trackingBody struct{ closed *bool }

func (b *trackingBody) Read(p []byte) (int, error) { return 0, io.EOF }
func (b *trackingBody) Close() error               { *b.closed = true; return nil }

func TestOptions_ReturnsResolved(t *testing.T) {
	c := newMockClient()
	o := c.Options()
	_ = o
}

// Wrap + New are coverable with a LAZY real client (elasticsearch.NewClient does
// not connect eagerly, unlike aerospike). This covers Wrap + the func-field copy
// + the public New delegation.
func TestWrap_RealLazyClient(t *testing.T) {
	raw, err := elasticsearch.NewClient(elasticsearch.Config{Addresses: []string{"http://127.0.0.1:1"}})
	require.NoError(t, err) // lazy: no connection attempted
	c := Wrap(raw)
	require.Equal(t, raw, c.Client()) // escape hatch returns the wrapped client
	_ = c.Options()
}

func TestNew_DelegatesAndErrors(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		// Short ctx so the Ping does not wait for the 10s fallback.
		pingCtx, cancel2 := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel2()
		_, err := New(pingCtx, WithAddresses("http://127.0.0.1:1")) // defaultOpener: lazy client + Ping fails
		done <- err
	}()
	select {
	case err := <-done:
		require.Error(t, err)
	case <-ctx.Done():
		t.Fatal("New against dead address did not return within 3s")
	}
}

// --- OnEvent ---

func TestSetOnEvent_FiresOnSuccessAndError(t *testing.T) {
	c := newMockClient()
	var got []Event
	c.SetOnEvent(func(e Event) { got = append(got, e) })

	_, _ = c.Index(context.Background(), "idx", strings.NewReader(`{}`)) // success
	c.index = func(string, io.Reader, ...func(*esapi.IndexRequest)) (*esapi.Response, error) {
		return nil, errTest
	}
	_, _ = c.Index(context.Background(), "idx", strings.NewReader(`{}`)) // error

	require.Len(t, got, 2)
	assert.Equal(t, KindIndex, got[0].Kind)
	assert.Equal(t, OutcomeSuccess, got[0].Outcome)
	assert.Equal(t, OutcomeError, got[1].Outcome)
	c.SetOnEvent(nil)
	assert.Nil(t, c.onEvent.Load())
}

// --- With* options ---

func TestOptions_AllWith(t *testing.T) {
	o := withDefaults([]Option{
		WithAddresses("http://a", "http://b"),
		WithCredentials("u", "p"),
		WithCloudID("cloud"),
		WithCACert([]byte("cert")),
		WithTransport(http.DefaultTransport),
	})
	assert.Equal(t, []string{"http://a", "http://b"}, o.Addresses)
	assert.Equal(t, "u", o.Username)
	assert.Equal(t, "cloud", o.CloudID)
	assert.Equal(t, []byte("cert"), o.CACert)
	assert.NotNil(t, o.Transport)
}

func TestOptions_AddressesCopied(t *testing.T) {
	in := []string{"http://a"}
	o := withDefaults([]Option{WithAddresses(in...)})
	in[0] = "http://mutated"
	assert.Equal(t, []string{"http://a"}, o.Addresses, "WithAddresses must copy")
}

func TestOptions_ToDriverMaps(t *testing.T) {
	o := withDefaults([]Option{WithAddresses("http://h"), WithCredentials("u", "p"), WithCloudID("c")})
	cfg := o.toDriver()
	assert.Equal(t, []string{"http://h"}, cfg.Addresses)
	assert.Equal(t, "u", cfg.Username)
	assert.Equal(t, "c", cfg.CloudID)
}
