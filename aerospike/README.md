# aerospike

Thin, option-configured wrapper around [`github.com/aerospike/aerospike-client-go/v8`](https://pkg.go.dev/github.com/aerospike/aerospike-client-go/v8).

Targets high-throughput KV (ad-tech session/profile/audience stores) and adds ergonomic construction (functional options + sane defaults), an eager connection (NewClientWithPolicy connects + pings), pass-through Put/Get/Delete/BatchGet, lightweight metrics + an event hook, an escape hatch to `*as.Client`, and a graceful Close. Query/Scan/Operate/UDF are reached via `Client()` (not wrapped).

## Why

Aerospike is the classic high-throughput KV for ad-tech (per-user session, frequency capping, audience-segment stores). A thin wrapper with metrics + consistent options removes per-service boilerplate.

## Install

```
go get github.com/v8fg/kit4go/aerospike
```

Isolated Go module.

## Quick start

```go
c, err := aerospike.New("localhost", 3000)
if err != nil { log.Fatal(err) }
defer c.Close()

key, _ := as.NewKey("profiles", "user", "u-42")
c.Put(nil, key, as.BinMap{"segment": "auto", "freq": 3}) // nil policy = defaults

rec, _ := c.Get(nil, key) // no binNames -> all bins
// rec.Bins is map[string]any
```

## Operations

| Method | Notes |
|---|---|
| `Put(policy, key, binMap)` | binMap is `as.BinMap` (map[string]any); nil policy = defaults |
| `Get(policy, key, binNames...)` | returns `*as.Record` (`.Bins` map); empty binNames = all |
| `Delete(policy, key)` | returns whether a record existed |
| `BatchGet(policy, keys, binNames...)` | multi-key, one round-trip |

No `context.Context` (aerospike uses policies for timeouts, not context). Query/Scan/Operate/UDF/CreateIndex via `Client()` → `*as.Client`.

## Errors

aerospike methods return `as.Error` (an interface embedding `error` with `ResultCode()` etc.). The wrapper returns it as the builtin `error` — still usable with `errors.Is`/`errors.As`, and you can type-assert to `as.AerospikeError` for `ResultCode`-based branching.

## Construction

`New` calls `NewClientWithPolicy`, which connects + pings the first node eagerly — a misconfigured host/credentials surface at construction, not the first op. Port defaults to 3000; timeout to 5s.

## Options

`WithHost` (required), `WithPort` (default 3000), `WithTimeout` (default 5s), `WithCredentials` (user/pass for a security-enabled cluster), `WithClusterName`, `WithNamespace` (documentation only — the namespace is carried by each `*as.Key`).

## Metrics & events

```go
c.SetOnEvent(func(e aerospike.Event) { /* e.Kind, e.Outcome */ })
m := c.Metrics() // Puts, Gets (incl. BatchGet), Deletes, Errors
```

## Mock seam

`*as.Client` satisfies a local `asAPI` interface subset directly (methods return `as.Error`). `as.Error` has unexported methods so it can't be constructed outside the package — the mock's error path returns a real `as.Error` sentinel obtained from a public aerospike function. `Wrap(*as.Client)` adopts an existing client.

## Testing

```
go test -short -race -cover ./...          # unit (mock), ~97% coverage
# integration (optional, needs a live cluster):
docker run -d -p 3000:3000 -p 3001:3001 -p 3002:3002 aerospike/aerospike-server
AEROSPIKE_HOST=127.0.0.1 go test -run Integration -v ./aerospike/
```
