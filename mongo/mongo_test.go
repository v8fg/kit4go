package mongo

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var errTest = errors.New("boom")

// shortCtx bounds dead-endpoint ping tests.
func shortCtx(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(context.Background(), 1500*time.Millisecond)
}

// --- New error paths ---

func TestNew_NoURI(t *testing.T) {
	_, err := newClient(context.Background(), nil, defaultConnector)
	require.ErrorIs(t, err, ErrNoURI)
}

func TestNew_ConnectError(t *testing.T) {
	conn := func(context.Context, ...*options.ClientOptions) (*mongo.Client, error) { return nil, errTest }
	_, err := newClient(context.Background(), []Option{WithURI("mongodb://x")}, conn)
	require.ErrorIs(t, err, errTest)
}

// Construction-error: a real Connect against a dead URI fails at the Ping (or
// Connect) — either way New returns nil + err.
func TestNew_ConstructionError(t *testing.T) {
	ctx, cancel := shortCtx(t)
	defer cancel()
	_, err := newClient(ctx, []Option{WithURI("mongodb://127.0.0.1:1")}, defaultConnector)
	require.Error(t, err)
}

func TestNew_DelegatesAndErrors(t *testing.T) {
	ctx, cancel := shortCtx(t)
	defer cancel()
	_, err := New(ctx, WithURI("mongodb://127.0.0.1:1"))
	require.Error(t, err)
}

// --- Collection ops (mock-backed) ---

func newMockClient() *Client { return &Client{} }

func TestInsertOne_SuccessAndError(t *testing.T) {
	cli := newMockClient()
	m := &mockAPI{}
	col := cli.newCollection(m, "db", "c")

	res, err := col.InsertOne(context.Background(), bson.M{"k": "v"})
	require.NoError(t, err)
	assert.Equal(t, "id", res.InsertedID)
	assert.Equal(t, uint64(1), cli.Metrics().Inserts)

	m.insertOneFn = func(context.Context, any, ...*options.InsertOneOptions) (*mongo.InsertOneResult, error) {
		return nil, errTest
	}
	_, err = col.InsertOne(context.Background(), bson.D{})
	require.ErrorIs(t, err, errTest)
	assert.Equal(t, uint64(1), cli.Metrics().Errors)
}

func TestInsertMany_SuccessAndError(t *testing.T) {
	cli := newMockClient()
	m := &mockAPI{}
	col := cli.newCollection(m, "db", "c")

	res, err := col.InsertMany(context.Background(), []any{bson.M{"k": 1}})
	require.NoError(t, err)
	assert.Len(t, res.InsertedIDs, 1)

	m.insertManyFn = func(context.Context, []any, ...*options.InsertManyOptions) (*mongo.InsertManyResult, error) {
		return nil, errTest
	}
	_, err = col.InsertMany(context.Background(), []any{bson.D{}})
	require.ErrorIs(t, err, errTest)
}

func TestFind_SuccessAndError(t *testing.T) {
	cli := newMockClient()
	m := &mockAPI{}
	col := cli.newCollection(m, "db", "c")

	cur, err := col.Find(context.Background(), bson.D{})
	require.NoError(t, err)
	require.NotNil(t, cur)
	assert.Equal(t, uint64(1), cli.Metrics().Finds)

	m.findFn = func(context.Context, any, ...*options.FindOptions) (*mongo.Cursor, error) {
		return nil, errTest
	}
	_, err = col.Find(context.Background(), bson.D{})
	require.ErrorIs(t, err, errTest)
}

func TestFindOne_DoesNotIncrementErrors(t *testing.T) {
	cli := newMockClient()
	col := cli.newCollection(&mockAPI{}, "db", "c")
	sr := col.FindOne(context.Background(), bson.D{})
	require.NotNil(t, sr)
	assert.Equal(t, uint64(1), cli.Metrics().Finds)
	assert.Equal(t, uint64(0), cli.Metrics().Errors) // error surfaces on Decode, not here
}

func TestUpdateOne_SuccessAndError(t *testing.T) {
	cli := newMockClient()
	m := &mockAPI{}
	col := cli.newCollection(m, "db", "c")

	res, err := col.UpdateOne(context.Background(), bson.D{}, bson.M{"$set": bson.M{"k": 1}})
	require.NoError(t, err)
	assert.Equal(t, int64(1), res.ModifiedCount)
	assert.Equal(t, uint64(1), cli.Metrics().Updates)

	m.updateOneFn = func(context.Context, any, any, ...*options.UpdateOptions) (*mongo.UpdateResult, error) {
		return nil, errTest
	}
	_, err = col.UpdateOne(context.Background(), bson.D{}, bson.D{})
	require.ErrorIs(t, err, errTest)
}

func TestUpdateMany_Success(t *testing.T) {
	cli := newMockClient()
	col := cli.newCollection(&mockAPI{}, "db", "c")
	res, err := col.UpdateMany(context.Background(), bson.D{}, bson.D{})
	require.NoError(t, err)
	assert.Equal(t, int64(2), res.ModifiedCount)
}

func TestUpdateMany_Error(t *testing.T) {
	cli := newMockClient()
	m := &mockAPI{updateManyFn: func(context.Context, any, any, ...*options.UpdateOptions) (*mongo.UpdateResult, error) {
		return nil, errTest
	}}
	col := cli.newCollection(m, "db", "c")
	_, err := col.UpdateMany(context.Background(), bson.D{}, bson.D{})
	require.ErrorIs(t, err, errTest)
	assert.Equal(t, uint64(1), cli.Metrics().Errors)
}

func TestDeleteOne_SuccessAndError(t *testing.T) {
	cli := newMockClient()
	m := &mockAPI{}
	col := cli.newCollection(m, "db", "c")
	res, err := col.DeleteOne(context.Background(), bson.D{})
	require.NoError(t, err)
	assert.Equal(t, int64(1), res.DeletedCount)
	assert.Equal(t, uint64(1), cli.Metrics().Deletes)

	m.deleteOneFn = func(context.Context, any, ...*options.DeleteOptions) (*mongo.DeleteResult, error) {
		return nil, errTest
	}
	_, err = col.DeleteOne(context.Background(), bson.D{})
	require.ErrorIs(t, err, errTest)
}

func TestDeleteMany_Success(t *testing.T) {
	cli := newMockClient()
	col := cli.newCollection(&mockAPI{}, "db", "c")
	res, err := col.DeleteMany(context.Background(), bson.D{})
	require.NoError(t, err)
	assert.Equal(t, int64(2), res.DeletedCount)
}

func TestDeleteMany_Error(t *testing.T) {
	cli := newMockClient()
	m := &mockAPI{deleteManyFn: func(context.Context, any, ...*options.DeleteOptions) (*mongo.DeleteResult, error) {
		return nil, errTest
	}}
	col := cli.newCollection(m, "db", "c")
	_, err := col.DeleteMany(context.Background(), bson.D{})
	require.ErrorIs(t, err, errTest)
	assert.Equal(t, uint64(1), cli.Metrics().Errors)
}

// --- Client.Collection from a real client + escape hatch + Options ---

func TestClient_CollectionFromRealClient(t *testing.T) {
	// A real client (dead endpoint is fine — Database/Collection are lazy struct
	// builders, no network). This covers Client.Collection + the dbName fallback
	// + the Collection() escape hatch (non-nil when backed by *mongo.Collection).
	raw, err := mongo.Connect(context.Background(), options.Client().ApplyURI("mongodb://127.0.0.1:1"))
	require.NoError(t, err)
	defer raw.Disconnect(context.Background())

	cli := Wrap(raw, WithDatabase("defaultdb"))
	// db == "" -> falls back to the Client's default database.
	col := cli.Collection("", "things")
	require.NotNil(t, col)
	assert.NotNil(t, col.Collection(), "escape hatch returns the real *mongo.Collection")
	_ = cli.Options() // covers Options()
}

func TestDisconnect_OwnedDisconnectsRaw(t *testing.T) {
	raw, err := mongo.Connect(context.Background(), options.Client().ApplyURI("mongodb://127.0.0.1:1"))
	require.NoError(t, err)
	cli := &Client{raw: raw, own: true} // white-box owning client
	assert.NoError(t, cli.Disconnect(context.Background()))
}

// --- Wrap / Disconnect / Client() / Collection() ---

func TestWrap_ClientAndDisconnect(t *testing.T) {
	raw, err := mongo.Connect(context.Background(), options.Client().ApplyURI("mongodb://127.0.0.1:1"))
	require.NoError(t, err)
	cli := Wrap(raw)
	assert.Equal(t, raw, cli.Client())
	assert.NoError(t, cli.Disconnect(context.Background())) // wrapped -> no-op
}

func TestClient_NilWhenNoRealClient(t *testing.T) {
	cli := newMockClient()
	assert.Nil(t, cli.Client())
}

func TestCollection_EscapeHatchNilWhenMock(t *testing.T) {
	cli := newMockClient()
	col := cli.newCollection(&mockAPI{}, "db", "c")
	assert.Nil(t, col.Collection()) // mock-backed -> nil
}

func TestDisconnect_NoOpWhenNotOwned(t *testing.T) {
	cli := newMockClient()
	assert.NoError(t, cli.Disconnect(context.Background()))
}

// --- OnEvent ---

func TestSetOnEvent_FiresOnSuccessAndError(t *testing.T) {
	cli := newMockClient()
	m := &mockAPI{}
	col := cli.newCollection(m, "db", "c")
	var got []Event
	cli.SetOnEvent(func(e Event) { got = append(got, e) })

	_, _ = col.InsertOne(context.Background(), bson.D{}) // success
	m.insertOneFn = func(context.Context, any, ...*options.InsertOneOptions) (*mongo.InsertOneResult, error) {
		return nil, errTest
	}
	_, _ = col.InsertOne(context.Background(), bson.D{}) // error

	require.Len(t, got, 2)
	assert.Equal(t, KindInsert, got[0].Kind)
	assert.Equal(t, OutcomeSuccess, got[0].Outcome)
	assert.Equal(t, OutcomeError, got[1].Outcome)
	cli.SetOnEvent(nil)
	assert.Nil(t, cli.onEvent.Load())
}

// --- With* options ---

func TestOptions_AllWith(t *testing.T) {
	o := withDefaults([]Option{
		WithURI("mongodb://h"),
		WithDatabase("db"),
		WithConnectTimeout(5 * time.Second),
		WithServerSelectionTimeout(5 * time.Second),
		WithMaxPoolSize(50),
		WithCredentials("u", "p", "admin"),
	})
	assert.Equal(t, "mongodb://h", o.URI)
	assert.Equal(t, "db", o.Database)
	assert.Equal(t, uint64(50), o.MaxPoolSize)
	assert.Equal(t, "u", o.Username)
}

func TestOptions_ConnectTimeoutDefaultsTo10s(t *testing.T) {
	assert.Equal(t, "10s", withDefaults(nil).ConnectTimeout.String())
}

func TestOptions_ToDriverMaps(t *testing.T) {
	o := withDefaults([]Option{
		WithURI("mongodb://h"),
		WithMaxPoolSize(7),
		WithConnectTimeout(3 * time.Second),
		WithServerSelectionTimeout(4 * time.Second),
		WithCredentials("u", "p", "admin"), // exercises the SetAuth branch
	})
	co := o.toDriver()
	require.NotNil(t, co)
}
