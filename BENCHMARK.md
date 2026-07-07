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

## More primitives

Same hardware/toolchain as the Batch6 baselines above (darwin/arm64, Apple M5,
Go 1.26.2, `-cpu=1`, `-benchtime=1s`, `-benchmem`, no `-race`). Order-of-
magnitude reference; rerun locally for your hardware.

The hot-path primitives (`ringbuffer`, `reservoir`, `backpressure`, `semaphore`,
`retry` backoff-fn, `backoff` `Next`) are sub-100 ns and 0 allocs/op. The
finance, ID, and signing packages allocate per op by design (`big.Int`,
`crypto/rand`, HMAC).

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|-----------|
| `ringbuffer` TryPushTryPop | 19.34 | 0 | 0 |
| `ringbuffer` TryPushTryPop (parallel) | 19.33 | 0 | 0 |
| `ringbuffer` Len | 8.451 | 0 | 0 |
| `reservoir` OfferSteady | 9.712 | 0 | 0 |
| `reservoir` OfferFill | 1400 | 1152 | 6 |
| `reservoir` Sample | 141.0 | 1024 | 1 |
| `reservoir` Count | 8.462 | 0 | 0 |
| `semaphore` AcquireRelease | 65.97 | 0 | 0 |
| `semaphore` AcquireRelease (parallel) | 65.74 | 0 | 0 |
| `semaphore` TryAcquire | 34.96 | 0 | 0 |
| `backpressure` TryAcquireRejected | 3.154 | 0 | 0 |
| `backpressure` Current | 0.4530 | 0 | 0 |
| `lru` Get | 14.10 | 0 | 0 |
| `lru` Get (parallel) | 78.77 | 0 | 0 |
| `lru` Peek | 14.45 | 0 | 0 |
| `lru` Set | 15.46 | 0 | 0 |
| `lru` SetEvict | 198.9 | 100 | 3 |
| `cache` Get | 74.14 | 0 | 0 |
| `cache` GetMiss | 30.66 | 0 | 0 |
| `cache` Set | 21.79 | 0 | 0 |
| `cache` SetWithTTL | 84.82 | 0 | 0 |
| `cache` Has | 72.21 | 0 | 0 |
| `fanout` Publish | 52.29 | 0 | 0 |
| `fanout` PublishBlocking | 142.0 | 0 | 0 |
| `fanout` Subscribers | 6.671 | 0 | 0 |
| `batcher` Add | 137.9 | 8 | 0 |
| `batcher` Add (parallel) | 136.7 | 8 | 0 |
| `batcher` Flush | 532.2 | 112 | 1 |
| `retry` DoSuccess | 28.30 | 24 | 1 |
| `retry` DoRetryableFail | 48.34 | 24 | 1 |
| `retry` DoPermanentFail | 33.86 | 24 | 1 |
| `retry` ExponentialBackoff | 0.4512 | 0 | 0 |
| `backoff` Next/None | 9.551 | 0 | 0 |
| `backoff` Next/Full | 15.27 | 0 | 0 |
| `backoff` Next/Equal | 15.41 | 0 | 0 |
| `backoff` Next/Decorrelated | 15.56 | 0 | 0 |
| `backoff` NextParallel | 15.20 | 0 | 0 |
| `backoff` Reset | 8.668 | 0 | 0 |
| `backoff` WaitZero | 335.0 | 248 | 3 |
| `money` Add | 11.82 | 0 | 0 |
| `money` Sub | 11.81 | 0 | 0 |
| `money` Mul | 0.4522 | 0 | 0 |
| `money` Scale | 4.732 | 0 | 0 |
| `money` String | 230.7 | 56 | 4 |
| `money` Parse | 95.77 | 32 | 1 |
| `decimal` Add | 47.68 | 80 | 2 |
| `decimal` Sub | 41.81 | 40 | 2 |
| `decimal` MulDecimal | 30.91 | 48 | 1 |
| `decimal` Div | 43.88 | 40 | 2 |
| `decimal` Cmp | 5.281 | 0 | 0 |
| `decimal` Parse | 205.8 | 112 | 5 |
| `decimal` String | 116.9 | 32 | 4 |
| `shortlink` Next | 9.490 | 0 | 0 |
| `shortlink` Next (parallel) | 9.740 | 0 | 0 |
| `shortlink` EncodeBaseN | 11.33 | 0 | 0 |
| `shortlink` Decode | 1334 | 2344 | 3 |
| `shortlink` Generate | 1565 | 455 | 19 |
| `shortlink` Resolve | 11.14 | 0 | 0 |
| `uuid` NewV4 | 340.3 | 0 | 0 |
| `uuid` NewV5 | 151.9 | 168 | 4 |
| `uuid` NewKSUID | 423.8 | 0 | 0 |
| `uuid` NewKSUIDRandomWithTime | 414.3 | 0 | 0 |
| `uuid` NewXID | 63.88 | 0 | 0 |
| `uuid` NewXIDWithTime | 58.71 | 0 | 0 |
| `uuid` RequestID | 385.2 | 32 | 1 |
| `uuid` RequestIDCanonicalFormat | 385.4 | 48 | 1 |
| `otp` TOTPCode | 842.7 | 528 | 11 |
| `otp` TOTPCodeCustom | 808.3 | 528 | 11 |
| `otp` HOTPCode | 806.1 | 528 | 11 |
| `otp` VerifyTOTP | 866.8 | 528 | 11 |
| `signing` Sign | 996.9 | 992 | 16 |
| `signing` Verify | 1088 | 1040 | 17 |
| `signing` Canonical | 354.9 | 224 | 5 |
| `signing` Compute | 522.0 | 720 | 10 |

Notes:

- `backpressure` also has the two contention rows already listed under
  "Rate control & resilience" (`TryAcquire_Release` ~9.1 ns,
  `TryAcquire_Contended` ~9.8 ns); the table above adds the rejection and
  accessor paths.
- `shortlink` `Generate`/`Decode` allocate because they use `crypto/rand` (per-
  byte `big.Int` draw) and a fresh result slice; the sequential `Next`/
  `EncodeBaseN`/`Resolve` paths are 0-alloc.
- `otp` allocates 11/op because each code recomputes an HMAC over the
  time/counter block and base32-decodes the secret on every call (the
  `pquerna/otp` library path); memoize the secret decode to drop that on a hot
  path.
- `signing` `Sign`/`Verify` allocate ~1 KB/op: canonical-string build (sorted +
  query-escaped) plus HMAC-SHA256 + hex; `Verify` is one extra compare + the
  same recompute.

