package aerospike

import (
	"errors"
	"testing"
	"time"

	as "github.com/aerospike/aerospike-client-go/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errTest is for the opener-injection path (opener returns builtin error).
var errTest = errors.New("boom")

func mustKey(t *testing.T, ns, set, pk string) *as.Key {
	t.Helper()
	k, err := as.NewKey(ns, set, pk)
	require.NoError(t, err)
	return k
}

// --- New error paths ---

func TestNew_NoHost(t *testing.T) {
	_, err := newClient("", 0, nil, defaultOpener)
	require.ErrorIs(t, err, ErrNoHost)
}

func TestNew_OpenError(t *testing.T) {
	open := func(*as.ClientPolicy, string, int) (*as.Client, error) { return nil, errTest }
	_, err := newClient("h", 3000, nil, open)
	require.ErrorIs(t, err, errTest)
}

// Construction-error: NewClientWithPolicy against a closed port fails within
// the policy timeout.
func TestNew_ConstructionError(t *testing.T) {
	_, err := newClient("127.0.0.1", 1, []Option{WithTimeout(1500 * time.Millisecond)}, defaultOpener)
	require.Error(t, err)
}

func TestNew_DelegatesAndErrors(t *testing.T) {
	_, err := New("127.0.0.1", 1, WithTimeout(1500*time.Millisecond))
	require.Error(t, err)
}

// --- ops ---

func TestPut_SuccessAndError(t *testing.T) {
	cli := newWithAPI(&mockAPI{})
	key := mustKey(t, "ns", "s", "pk")

	require.NoError(t, cli.Put(nil, key, as.BinMap{"a": 1}))
	assert.Equal(t, uint64(1), cli.Metrics().Puts)

	m := &mockAPI{putFn: func(*as.WritePolicy, *as.Key, as.BinMap) as.Error { return asErrSentinel }}
	cli2 := newWithAPI(m)
	err := cli2.Put(nil, key, as.BinMap{})
	require.ErrorIs(t, err, asErrSentinel)
	assert.Equal(t, uint64(1), cli2.Metrics().Errors)
}

func TestGet_SuccessAndError(t *testing.T) {
	cli := newWithAPI(&mockAPI{})
	key := mustKey(t, "ns", "s", "pk")

	rec, err := cli.Get(nil, key)
	require.NoError(t, err)
	require.NotNil(t, rec)
	assert.Equal(t, uint64(1), cli.Metrics().Gets)

	m := &mockAPI{getFn: func(*as.BasePolicy, *as.Key, ...string) (*as.Record, as.Error) {
		return nil, asErrSentinel
	}}
	cli2 := newWithAPI(m)
	_, err = cli2.Get(nil, key)
	require.ErrorIs(t, err, asErrSentinel)
	assert.Equal(t, uint64(1), cli2.Metrics().Errors)
}

func TestDelete_SuccessAndError(t *testing.T) {
	cli := newWithAPI(&mockAPI{})
	key := mustKey(t, "ns", "s", "pk")

	existed, err := cli.Delete(nil, key)
	require.NoError(t, err)
	assert.True(t, existed)
	assert.Equal(t, uint64(1), cli.Metrics().Deletes)

	m := &mockAPI{deleteFn: func(*as.WritePolicy, *as.Key) (bool, as.Error) { return false, asErrSentinel }}
	ex, err := newWithAPI(m).Delete(nil, key)
	require.ErrorIs(t, err, asErrSentinel)
	assert.False(t, ex)
}

func TestBatchGet_SuccessAndError(t *testing.T) {
	cli := newWithAPI(&mockAPI{})
	key := mustKey(t, "ns", "s", "pk")

	recs, err := cli.BatchGet(nil, []*as.Key{key})
	require.NoError(t, err)
	assert.NotNil(t, recs)
	assert.Equal(t, uint64(1), cli.Metrics().Gets) // BatchGet counts under Gets

	m := &mockAPI{batchGetFn: func(*as.BatchPolicy, []*as.Key, ...string) ([]*as.Record, as.Error) {
		return nil, asErrSentinel
	}}
	_, err = newWithAPI(m).BatchGet(nil, []*as.Key{key})
	require.ErrorIs(t, err, asErrSentinel)
}

// --- Wrap / Close / Client() / Options ---

func TestClient_NilWhenMockInjected(t *testing.T) {
	assert.Nil(t, newWithAPI(&mockAPI{}).Client())
}

func TestClose_NoOpWhenMockInjected(t *testing.T) {
	m := &mockAPI{}
	cli := newWithAPI(m)
	cli.Close() // own=false -> no-op, must NOT call api.Close
	assert.False(t, m.closeCalled)
}

func TestClose_OwnedCallsAPIClose(t *testing.T) {
	m := &mockAPI{}
	cli := &Client{api: m, own: true} // white-box owning client over the mock
	cli.Close()
	assert.True(t, m.closeCalled)
}

func TestOptions_ReturnsResolved(t *testing.T) {
	cli := newWithAPI(&mockAPI{})
	o := cli.Options()
	assert.Equal(t, 3000, o.Port) // default
}

// --- OnEvent ---

func TestSetOnEvent_FiresOnSuccessAndError(t *testing.T) {
	m := &mockAPI{}
	cli := newWithAPI(m)
	key := mustKey(t, "ns", "s", "pk")
	var got []Event
	cli.SetOnEvent(func(e Event) { got = append(got, e) })

	require.NoError(t, cli.Put(nil, key, as.BinMap{})) // success
	m.putFn = func(*as.WritePolicy, *as.Key, as.BinMap) as.Error { return asErrSentinel }
	require.Error(t, cli.Put(nil, key, as.BinMap{})) // error

	require.Len(t, got, 2)
	assert.Equal(t, KindPut, got[0].Kind)
	assert.Equal(t, OutcomeSuccess, got[0].Outcome)
	assert.Equal(t, OutcomeError, got[1].Outcome)
	cli.SetOnEvent(nil)
	assert.Nil(t, cli.onEvent.Load())
}

// --- With* options ---

func TestOptions_AllWith(t *testing.T) {
	o := withDefaults([]Option{
		WithHost("h"),
		WithPort(4000),
		WithTimeout(7 * time.Second),
		WithCredentials("u", "p"),
		WithClusterName("c"),
		WithNamespace("ns"),
	})
	assert.Equal(t, "h", o.Host)
	assert.Equal(t, 4000, o.Port)
	assert.Equal(t, "u", o.UserName)
	assert.Equal(t, "c", o.ClusterName)
}

func TestOptions_DefaultsPortAndTimeout(t *testing.T) {
	o := withDefaults(nil)
	assert.Equal(t, 3000, o.Port)
	assert.Equal(t, "5s", o.Timeout.String())
}

func TestOptions_ToClientPolicyMaps(t *testing.T) {
	o := withDefaults([]Option{
		WithHost("h"),
		WithCredentials("u", "p"),
		WithClusterName("c"),
		WithTimeout(3 * time.Second),
	})
	cp := o.toClientPolicy()
	require.NotNil(t, cp)
	assert.Equal(t, "u", cp.User)
	assert.Equal(t, "c", cp.ClusterName)
	assert.Equal(t, 3*time.Second, cp.Timeout)
}
