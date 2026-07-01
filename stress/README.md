# stress: whole-kit load tests

A test-only package containing load and concurrency-safety tests that exercise
the client packages together. It is not an importable API; run it with the rest
of the build to catch regressions in throughput, allocation, and goroutine
safety.

## Tests

- `TestStress_AllClients` drives every client package at load.
- `TestStress_ConcurrentSafety` runs concurrent access looking for data races
  (run with `-race`).

## Run

```bash
go test -race -count=1 -run TestStress ./stress/...
go test -bench=. -benchmem ./stress/...
```
