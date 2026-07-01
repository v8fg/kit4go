# pipeline

A generic stream-processing stage: N workers transform items (map + filter)
concurrently through a bounded channel pipeline, with graceful drain on Close.
Pure standard library.

## API

```go
p := pipeline.New(4, func(ctx context.Context, req *BidReq) (*BidResp, bool, error) {
    resp, err := evaluate(req)
    if err != nil { return nil, false, err }
    return resp, resp.Valid, nil
}, pipeline.WithInputBuffer[*BidReq, *BidResp](100))

go func() { for r := range p.Out() { send(r) } }()
for _, req := range requests { p.Send(ctx, req) }
p.Close()
```

| Symbol | Behavior |
|---|---|
| `New(workers, stage, opts...)` | Build with N workers |
| `WithInputBuffer(n)` / `WithOutputBuffer(n)` | Channel caps (default = workers) |
| `Send(ctx, item)` | Enqueue (backpressure + ctx-aware) |
| `Out() <-chan O` | Result stream |
| `Close()` | Graceful drain (process queued items, then close output). Idempotent |
| `Workers() int` | Worker count |

**Graceful drain**: Close closes the `done` channel, workers enter drain mode
(process remaining items non-blocking, then exit), Close waits for all workers,
then closes `Out`. No items are lost on graceful shutdown.

## Ad-tech uses

- **Bid evaluation pipeline**: validate → enrich → filter eligible → evaluate.
- **Log transformation**: raw → enriched → filtered → published (chain pipelines).
- **Creative processing**: fetch → resize → validate → store.

## Testing

95% coverage, `-race` clean. Covers transform+collect, filter, error-drop, type
transform, backpressure, concurrency limit, pipeline chaining, idempotent Close.
