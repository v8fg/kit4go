# Benchmarks

Hot-path primitive baselines. Order-of-magnitude reference; rerun locally for
your hardware.

Measured: darwin/arm64, Go 1.26, `-cpu=1`, `-benchtime=1s`, no `-race`.

## Run

```sh
go test -run='^$' -bench=. -benchtime=1s -cpu=1 ./<pkg>/...
```

## Rate control & resilience (0 allocs/op — P1 seams are allocation-neutral)

The `now`/`nowTime` clock seam (P1-b) and atomic state add no allocations; the
hot paths stay sub-150 ns single-threaded.

| Benchmark | ns/op | allocs/op |
|-----------|------|-----------|
| `limiter` TokenBucket Allow | ~86 | 0 |
| `limiter` TokenBucket Allow (parallel) | ~101 | 0 |
| `limiter` SlidingWindow Allow | ~100 | 0 |
| `limiter` TokenBucket Wait | ~109 | 0 |
| `breaker` Execute (success) | ~127 | 0 |
| `breaker` Execute (parallel) | ~116 | 0 |
| `breaker` State | ~0.8 | 0 |
| `breaker` Metrics | ~2.2 | 0 |
| `backpressure` TryAcquire+Release | ~7.3 | 0 |
| `backpressure` TryAcquire (contended) | ~7.3 | 0 |

## Logging

`log4go` has its own [PERFORMANCE.md](log4go/PERFORMANCE.md): `deliver` ≈ 1080
ns/op ≈ 923K QPS/core, a per-writer ns/op table, and multi-core scaling. The
P1-a `onEvent` atomicization (`atomic.Pointer[func]`, Load + nil-check) is
allocation-neutral and within benchmark noise.

## Other packages

Sketch / latency / auction baselines live in each package's `*_bench_test.go`:

```sh
go test -run='^$' -bench=. -benchtime=1s ./countmin/... ./hyperloglog/... ./latency/... ./auction/... ./shortlink/...
```
