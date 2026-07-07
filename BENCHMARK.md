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

## Data structures & algorithms

Batch6 baselines (darwin/arm64, Apple M5, Go 1.26, `-cpu=1`, `-benchtime=1s`,
no `-race`). Order-of-magnitude reference; rerun locally for your hardware.

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|-----------|
| `bloom` New | 15603 | 122960 | 2 |
| `bloom` Add | 162.9 | 64 | 1 |
| `bloom` AddString | 142.6 | 64 | 1 |
| `bloom` TestHit | 162.6 | 64 | 1 |
| `bloom` TestMiss | 121.9 | 64 | 1 |
| `bloom` TestAndAdd | 202.8 | 64 | 1 |
| `bloom` Indices | 107.5 | 64 | 1 |
| `countmin` Add | 145.2 | 0 | 0 |
| `countmin` Estimate | 263.6 | 0 | 0 |
| `hyperloglog` Add | 125.1 | 0 | 0 |
| `hyperloglog` Add (parallel) | 107.8 | 0 | 0 |
| `hyperloglog` AddString | 88.72 | 0 | 0 |
| `hyperloglog` Estimate | 1440356 | 0 | 0 |
| `topk` New | 15.02 | 0 | 0 |
| `topk` TouchAdmit | 2814 | 248 | 3 |
| `topk` TouchIncrement | 71.86 | 0 | 0 |
| `topk` TouchEvict | 1750 | 63 | 3 |
| `topk` TouchHighCardinality | 38.92 | 2 | 0 |
| `topk` TouchN | 22.24 | 0 | 0 |
| `topk` Top | 1676 | 336 | 4 |
| `topk` Count | 11.67 | 0 | 0 |
| `topk` TopK100 | 4129 | 2784 | 4 |
| `topk` TouchHighCardinalityK100 | 100.1 | 2 | 0 |
| `consistenthash` New/10nodes | 114.2 | 224 | 2 |
| `consistenthash` New/100nodes | 1014 | 1856 | 2 |
| `consistenthash` Add | 115302 | 97 | 1 |
| `consistenthash` Get/10nodes | 545.6 | 160 | 10 |
| `consistenthash` Get/50nodes | 4831 | 1120 | 50 |
| `consistenthash` Get/100nodes | 9182 | 2320 | 100 |
| `consistenthash` Get/500nodes | 24019 | 11920 | 500 |
| `consistenthash` GetN/10nodes | 588.8 | 448 | 12 |
| `consistenthash` GetN/100nodes | 4833 | 5056 | 102 |
| `consistenthash` GetN/500nodes | 23072 | 24256 | 502 |
| `consistenthash` Remove | 775.7 | 0 | 0 |
| `consistenthash` DefaultHash | 15.01 | 0 | 0 |
| `loadbalance` New | 378.1 | 216 | 7 |
| `loadbalance` NextSWRR | 24.74 | 0 | 0 |
| `loadbalance` NextRoundRobin | 17.07 | 0 | 0 |
| `loadbalance` NextRandom | 26.13 | 0 | 0 |
| `loadbalance` NextWeightedRandom | 40.93 | 0 | 0 |
| `loadbalance` Add | 163.7 | 104 | 3 |
| `loadbalance` Remove | 1118 | 216 | 7 |
| `loadbalance` RemoveHot | 476.7 | 0 | 0 |
| `auction` Resolve_Small | 382.3 | 296 | 5 |
| `auction` Resolve_Large | 30805 | 4264 | 5 |
| `fsm` Send | 47.04 | 0 | 0 |
| `fsm` Can | 29.57 | 0 | 0 |
| `errcode` New | 0.4187 | 0 | 0 |
| `errcode` Wrap | 0.4297 | 0 | 0 |
| `errcode` ErrorString | 156.6 | 48 | 2 |
| `errcode` ErrorStringWrapped | 284.8 | 64 | 2 |
| `errcode` ErrorsIs_SameCode | 73.19 | 8 | 1 |
| `errcode` ErrorsIs_DifferentCode | 69.47 | 8 | 1 |
| `errcode` ErrorsIs_ThroughWrapChain | 70.55 | 8 | 1 |
| `errcode` CodeOf_Nil | 0.4512 | 0 | 0 |
| `errcode` CodeOf_DirectError | 113.8 | 8 | 1 |
| `errcode` CodeOf_Wrapped | 124.9 | 8 | 1 |
| `errcode` CodeOf_PlainError | 84.53 | 8 | 1 |
| `errcode` WithDetail | 167.5 | 112 | 3 |
| `errcode` CodeString | 0.5644 | 0 | 0 |
| `errcode` CodeString_OutOfRange | 0.4963 | 0 | 0 |
| `objpool` New | 106.4 | 112 | 2 |
| `objpool` GetNoResetCold | 75.17 | 48 | 1 |
| `objpool` GetPutNoReset | 18.08 | 0 | 0 |
| `objpool` GetPutWithReset | 19.52 | 0 | 0 |
| `objpool` Stats | 1.564 | 0 | 0 |

## Other packages

Remaining baselines live in each package's `*_bench_test.go`:

```sh
go test -run='^$' -bench=. -benchtime=1s ./latency/... ./shortlink/...
```
