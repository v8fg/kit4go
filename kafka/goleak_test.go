package kafka

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain asserts no goroutine leaks across the suite (the producer drain
// goroutines and the partition-consumer pump must all exit on Close). sarama's
// own background goroutines are ignored by CurrentGoroutine() at setup so only
// leaks introduced by this package are reported.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m,
		// sarama spins up a handful of named background goroutines (e.g. the
		// mock producers' dispatchers) that outlive a single Close; ignore any
		// present at suite start so only this package's leaks surface.
		goleak.IgnoreCurrent(),
	)
}
