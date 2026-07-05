# backpressure

Non-blocking load-shed gate. When in-flight work exceeds a threshold, `TryAcquire` returns false (reject, don't queue). CAS-based, `Rejected()` counter (L5 observable). Distinct from semaphore (which blocks). Pure standard library.

## Usage

- `New(maxConcurrent int32) *Gate`
- `(*Gate).TryAcquire() bool` — returns false at capacity (shed the work).
- `(*Gate).Release() bool` — decrement; false if nothing in flight (no panic).
- `(*Gate).Current() / .Rejected() / .IsOverloaded() / .Max() / .SetMax()`.

## Example

```go
gate := backpressure.New(100)
if !gate.TryAcquire() {
    http.Error(w, "busy", 503) // shed
    return
}
defer gate.Release()
handleRequest(r)
```
