# mongo

Thin, option-configured wrapper around [`go.mongodb.org/mongo-driver`](https://pkg.go.dev/go.mongodb.org/mongo-driver) v1.17 (the official Go driver).

Targets the dominant local use — document **CRUD (Find/Insert/Update/Delete)** — and adds ergonomic construction (functional options + sane defaults), a fail-fast Ping at construction, a `Collection` wrapper that adds metrics + an event hook to every op, escape hatches to the underlying `*mongo.Collection` / `*mongo.Client`, and a graceful Disconnect. `CountDocuments`/`Aggregate`/`BulkWrite` are deliberately NOT wrapped (0 local usage — reach them via `Collection()`).

## Why

MongoDB is the default document store for ad-tech/finance services (creative metadata, user profiles, event logs). A thin wrapper with metrics + a fail-fast ping + consistent options removes boilerplate every service re-implements.

## Install

```
go get github.com/v8fg/kit4go/mongo
```

Isolated Go module — importing it pulls mongo-driver's dependency tree into your module, not the rest of kit4go.

## Quick start

```go
ctx := context.Background()
c, err := mongo.New(ctx, mongo.WithURI("mongodb://localhost:27017"), mongo.WithDatabase("ads"))
if err != nil { log.Fatal(err) }
defer c.Disconnect(ctx)

creatives := c.Collection("", "creatives") // "" -> WithDatabase default
creatives.InsertOne(ctx, bson.M{"name": "banner-1", "size": "300x250"})

cur, _ := creatives.Find(ctx, bson.M{"size": "300x250"})
defer cur.Close(ctx)
var results []bson.M
cur.All(ctx, &results)
```

## Two types

- **`Client`** owns the connection (Connect/Ping/Disconnect) and carries the shared metrics + event hook.
- **`Collection`** (from `client.Collection(db, coll)`) wraps `*mongo.Collection`; every op bumps the owning Client's metrics.

| Collection method | Notes |
|---|---|
| `InsertOne` / `InsertMany` | returns InsertOneResult{InsertedID} / InsertManyResult{InsertedIDs} |
| `Find` / `FindOne` | Find returns a `*mongo.Cursor` (caller closes); FindOne returns `*mongo.SingleResult` (error surfaces on Decode, not counted as error) |
| `UpdateOne` / `UpdateMany` | returns UpdateResult{MatchedCount, ModifiedCount, UpsertedID} |
| `DeleteOne` / `DeleteMany` | returns DeleteResult{DeletedCount} |

`Aggregate`, `CountDocuments`, `BulkWrite`, `FindOneAndUpdate` are reached via `Collection()` → `*mongo.Collection`.

## Construction

`New` connects and runs a `Ping(readpref.Primary)` (bounded by the context, 10s fallback) to **fail fast** on an unreachable server — `mongo.Connect` does not guarantee the server is live.

## Options

`WithURI` (required), `WithDatabase` (default db for `Collection("", ...)`), `WithConnectTimeout` (default 10s), `WithServerSelectionTimeout`, `WithMaxPoolSize`, `WithCredentials` (prefer credentials in the URI).

## Metrics & events

```go
c.SetOnEvent(func(e mongo.Event) { /* e.Kind, e.Outcome */ })
m := c.Metrics() // Inserts, Finds, Updates, Deletes, Errors (aggregated across collections)
```

## Mock seam

`*mongo.Client` and `*mongo.Collection` are concrete structs that satisfy local interface subsets (`clientAPI`, `collectionAPI`) by structural typing — the sole unit-test strategy (no miniredis-equivalent for MongoDB). `Wrap(*mongo.Client)` adopts an existing client.

## Testing

```
go test -short -race -cover ./...          # unit (mock), ~97% coverage
# integration (optional, needs a live MongoDB):
docker run -d -p 27017:27017 mongo:7
MONGO_URI=mongodb://127.0.0.1:27017 go test -run Integration -v ./mongo/
```
