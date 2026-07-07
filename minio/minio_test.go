package minio

import (
	"context"
	"errors"
	"io"
	"net/url"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errTest = errors.New("boom")

// --- New error paths ---

func TestNew_NoEndpoint(t *testing.T) {
	_, err := newClient(context.Background(), nil, defaultOpener)
	require.ErrorIs(t, err, ErrNoEndpoint)
}

func TestNew_OpenError(t *testing.T) {
	open := func(endpoint string, opts *minio.Options) (*minio.Client, error) { return nil, errTest }
	_, err := newClient(context.Background(), []Option{WithEndpoint("x")}, open)
	require.ErrorIs(t, err, errTest)
}

// Ping-error: opener returns a real *minio.Client aimed at a closed port so the
// ListBuckets ping fails fast (connection refused) — proves the fail-fast path
// returns nil + the underlying error without a half-built client.
func TestNew_PingError(t *testing.T) {
	open := func(endpoint string, opts *minio.Options) (*minio.Client, error) {
		return minio.New("127.0.0.1:1", opts) // port 1: connection refused
	}
	c, err := newClient(context.Background(), []Option{WithEndpoint("127.0.0.1:1"), WithSecure(false)}, open)
	require.Error(t, err)
	require.Nil(t, c)
}

// --- op happy + error paths (mock-injected) ---

func TestPutObject_SuccessAndBytes(t *testing.T) {
	m := &mockAPI{}
	c := newWithAPI(m)
	info, err := c.PutObject(context.Background(), "b", "o", io.Reader(nil), 0, minio.PutObjectOptions{})
	require.NoError(t, err)
	assert.Equal(t, int64(42), info.Size) // mock default
	mm := c.Metrics()
	assert.Equal(t, uint64(1), mm.Puts)
	assert.Equal(t, uint64(42), mm.BytesUploaded)
	assert.Equal(t, uint64(0), mm.Errors)
}

func TestPutObject_Error(t *testing.T) {
	m := &mockAPI{putFn: func(context.Context, string, string, io.Reader, int64, minio.PutObjectOptions) (minio.UploadInfo, error) {
		return minio.UploadInfo{}, errTest
	}}
	c := newWithAPI(m)
	_, err := c.PutObject(context.Background(), "b", "o", nil, 0, minio.PutObjectOptions{})
	require.ErrorIs(t, err, errTest)
	assert.Equal(t, uint64(1), c.Metrics().Errors)
	assert.Equal(t, uint64(0), c.Metrics().BytesUploaded) // not counted on error
}

func TestGetObject_SuccessAndError(t *testing.T) {
	m := &mockAPI{}
	c := newWithAPI(m)
	obj, err := c.GetObject(context.Background(), "b", "o", minio.GetObjectOptions{})
	require.NoError(t, err)
	require.NotNil(t, obj)
	assert.Equal(t, uint64(1), c.Metrics().Gets)

	m.getFn = func(context.Context, string, string, minio.GetObjectOptions) (*minio.Object, error) {
		return nil, errTest
	}
	_, err = c.GetObject(context.Background(), "b", "o", minio.GetObjectOptions{})
	require.ErrorIs(t, err, errTest)
	assert.Equal(t, uint64(1), c.Metrics().Errors)
}

func TestStatObject_SuccessAndError(t *testing.T) {
	m := &mockAPI{}
	c := newWithAPI(m)
	info, err := c.StatObject(context.Background(), "b", "o", minio.StatObjectOptions{})
	require.NoError(t, err)
	assert.Equal(t, "o", info.Key)
	assert.Equal(t, uint64(1), c.Metrics().Stats)

	m.statFn = func(context.Context, string, string, minio.StatObjectOptions) (minio.ObjectInfo, error) {
		return minio.ObjectInfo{}, errTest
	}
	_, err = c.StatObject(context.Background(), "b", "o", minio.StatObjectOptions{})
	require.ErrorIs(t, err, errTest)
	assert.Equal(t, uint64(1), c.Metrics().Errors)
}

func TestRemoveObject_SuccessAndError(t *testing.T) {
	m := &mockAPI{}
	c := newWithAPI(m)
	require.NoError(t, c.RemoveObject(context.Background(), "b", "o", minio.RemoveObjectOptions{}))
	assert.Equal(t, uint64(1), c.Metrics().Removes)

	m.removeFn = func(context.Context, string, string, minio.RemoveObjectOptions) error { return errTest }
	require.ErrorIs(t, c.RemoveObject(context.Background(), "b", "o", minio.RemoveObjectOptions{}), errTest)
	assert.Equal(t, uint64(1), c.Metrics().Errors)
}

func TestBucketExists_SuccessAndError(t *testing.T) {
	m := &mockAPI{}
	c := newWithAPI(m)
	ok, err := c.BucketExists(context.Background(), "b")
	require.NoError(t, err)
	assert.True(t, ok)

	m.bucketExistsF = func(context.Context, string) (bool, error) { return false, errTest }
	_, err = c.BucketExists(context.Background(), "b")
	require.ErrorIs(t, err, errTest)
}

func TestMakeBucket_SuccessAndError(t *testing.T) {
	m := &mockAPI{}
	c := newWithAPI(m)
	require.NoError(t, c.MakeBucket(context.Background(), "b", minio.MakeBucketOptions{}))
	m.makeBucketFn = func(context.Context, string, minio.MakeBucketOptions) error { return errTest }
	require.ErrorIs(t, c.MakeBucket(context.Background(), "b", minio.MakeBucketOptions{}), errTest)
}

func TestListObjects_Success(t *testing.T) {
	m := &mockAPI{}
	c := newWithAPI(m)
	out, err := c.ListObjects(context.Background(), "b", minio.ListObjectsOptions{})
	require.NoError(t, err)
	assert.Len(t, out, 2) // mock default emits a, b
	assert.Equal(t, "a", out[0].Key)
}

func TestListObjects_SurfacesEmbeddedError(t *testing.T) {
	m := &mockAPI{listObjectsFn: func(context.Context, string, minio.ListObjectsOptions) <-chan minio.ObjectInfo {
		ch := make(chan minio.ObjectInfo, 2)
		ch <- minio.ObjectInfo{Key: "good"}
		ch <- minio.ObjectInfo{Err: errTest}
		close(ch)
		return ch
	}}
	c := newWithAPI(m)
	out, err := c.ListObjects(context.Background(), "b", minio.ListObjectsOptions{})
	require.ErrorIs(t, err, errTest)
	assert.Len(t, out, 1) // the good item before the error
	assert.Equal(t, uint64(1), c.Metrics().Errors)
}

func TestPresignedGetObject_SuccessAndError(t *testing.T) {
	m := &mockAPI{}
	c := newWithAPI(m)
	s, err := c.PresignedGetObject(context.Background(), "b", "o", time.Hour, nil)
	require.NoError(t, err)
	assert.Contains(t, s, "/b/o")

	m.presignFn = func(context.Context, string, string, time.Duration, url.Values) (*url.URL, error) {
		return nil, errTest
	}
	_, err = c.PresignedGetObject(context.Background(), "b", "o", time.Hour, nil)
	require.ErrorIs(t, err, errTest)
}

// --- Wrap / Client() / Options() / HealthCheck ---

func TestWrap_ClientAndOptions(t *testing.T) {
	raw, err := minio.New("play.min.io", &minio.Options{Secure: true})
	require.NoError(t, err)
	c := Wrap(raw)
	assert.Equal(t, raw, c.Client()) // escape hatch returns the wrapped client
	assert.True(t, c.Options().Secure)
}

func TestClient_NilWhenMockInjected(t *testing.T) {
	c := newWithAPI(&mockAPI{})
	assert.Nil(t, c.Client())
}

func TestHealthCheck_NoPanicOnMock(t *testing.T) {
	c := newWithAPI(&mockAPI{})
	var (
		cancel context.CancelFunc
		err    error
	)
	assert.NotPanics(t, func() { cancel, err = c.HealthCheck(time.Second) }) // raw nil -> no-op
	require.NoError(t, err)
	assert.Nil(t, cancel)
}

func TestHealthCheck_OnRealClient(t *testing.T) {
	raw, err := minio.New("play.min.io", &minio.Options{})
	require.NoError(t, err)
	c := Wrap(raw)
	var cancel context.CancelFunc
	assert.NotPanics(t, func() { cancel, _ = c.HealthCheck(30 * time.Second) })
	if cancel != nil {
		cancel() // stop the background probe
	}
}

// --- OnEvent ---

func TestSetOnEvent_FiresOnSuccessAndError(t *testing.T) {
	m := &mockAPI{}
	c := newWithAPI(m)
	var got []Event
	c.SetOnEvent(func(e Event) { got = append(got, e) })

	_, _ = c.PutObject(context.Background(), "b", "o", nil, 0, minio.PutObjectOptions{}) // success
	m.putFn = func(context.Context, string, string, io.Reader, int64, minio.PutObjectOptions) (minio.UploadInfo, error) {
		return minio.UploadInfo{}, errTest
	}
	_, _ = c.PutObject(context.Background(), "b", "o", nil, 0, minio.PutObjectOptions{}) // error

	require.Len(t, got, 2)
	assert.Equal(t, KindPut, got[0].Kind)
	assert.Equal(t, OutcomeSuccess, got[0].Outcome)
	assert.Equal(t, OutcomeError, got[1].Outcome)

	c.SetOnEvent(nil) // disable
	assert.Nil(t, c.onEvent.Load())
}

// --- With* options land in resolved Options ---

func TestOptions_AllWith(t *testing.T) {
	o := withDefaults([]Option{
		WithEndpoint("ep"),
		WithCredentials("ak", "sk"),
		WithSecure(false),
		WithRegion("us-east-1"),
		WithBucketLookup(BucketLookupPath),
	})
	assert.Equal(t, "ep", o.Endpoint)
	assert.Equal(t, "ak", o.AccessKey)
	assert.Equal(t, "sk", o.SecretKey)
	assert.False(t, o.Secure) // explicitly disabled
	assert.Equal(t, "us-east-1", o.Region)
	assert.Equal(t, BucketLookupPath, o.BucketLookup)
}

func TestOptions_SecureDefaultsTrue(t *testing.T) {
	o := withDefaults(nil)
	assert.True(t, o.Secure) // HTTPS-by-default safety
}

func TestOptions_ToDriverMapsLookup(t *testing.T) {
	assert.Equal(t, minio.BucketLookupAuto, toDriverBucketLookup(BucketLookupAuto))
	assert.Equal(t, minio.BucketLookupDNS, toDriverBucketLookup(BucketLookupDNS))
	assert.Equal(t, minio.BucketLookupPath, toDriverBucketLookup(BucketLookupPath))
}
