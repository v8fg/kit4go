// Package breaker provides a generic, zero-dependency circuit breaker.
//
// # Quick start
//
//	b := breaker.NewBreaker[string](breaker.BreakerOptions{
//	    FailRate:    0.5,
//	    MinRequests: 10,
//	    OpenDuration: 30 * time.Second,
//	})
//	result, err := b.Execute(ctx, func(ctx context.Context) (string, error) {
//	    return callDownstream(ctx)
//	})
//	if errors.Is(err, breaker.ErrCircuitOpen) {
//	    // fast-fail: downstream is tripped
//	}
//
// # Performance
//
//	BenchmarkBreaker_Execute_Success    70 ns    0 allocs
//	BenchmarkBreaker_Execute_Fail       65 ns    0 allocs
//	BenchmarkBreaker_Execute_Parallel  230 ns    0 allocs (RunParallel)
//	BenchmarkBreaker_State              0.5 ns   0 allocs
//	BenchmarkBreaker_Metrics            2.2 ns   0 allocs
//
// The hot path (Execute on Closed state) is allocation-free.
// SetOnEvent fires on state transitions (trip/recover/reject) — nil by default.
//
// # States
//
//	Closed → Open      failRate >= threshold AND requests >= minRequests
//	Open → HalfOpen    after OpenDuration
//	HalfOpen → Closed  MaxRequests consecutive successes
//	HalfOpen → Open    any failure
//
// # Monitoring
//
//	m := b.Metrics()
//	// m.State, m.Total, m.Success, m.Failures, m.ConsecutiveFail
//	b.SetOnEvent(func(evt breaker.BreakerEvent) {
//	    promBreakerEvents.WithLabelValues(evt.Name).Inc() // trip/recover/reject
//	})
package breaker
