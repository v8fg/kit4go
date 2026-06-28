//go:build !franzgo

package kafka

import (
	"errors"
	"testing"
	"time"
)

// Shared test helpers — no build tag, available to BOTH the sarama and franz-go
// backend test suites.

// waitUntil polls cond until it returns true or the deadline passes (fail on
// timeout). Backend drain/pump goroutines run async, so Metrics counters settle
// shortly after Send/Consume/Close.
func waitUntil(t *testing.T, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for: %s", what)
}

// errBoom is the shared sentinel error used across backend tests.
var errBoom = errors.New("boom")

// errorIs is a minimal errors.Is (unwrap chain) for test assertions.
func errorIs(err, target error) bool {
	for err != nil {
		if err == target {
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := err.(unwrapper)
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

// contains reports whether s contains v.
func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
