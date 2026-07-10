package limiter

// Regression tests for the F4 fix: Wait() on a closed limiter must return
// promptly instead of busy-looping on sub-millisecond timers until the context
// expires.
//
// BEFORE THE FIX: every Wait implementation called Allow() (which returns false
// on a closed limiter) and then entered a poll loop armed with 1ms timers,
// spinning until ctx.Done(). The Limiter interface doc promised that
// Allow/Wait/TryAcquire are no-ops after Close, but Wait violated that — it
// blocked for the full context budget.
//
// AFTER THE FIX: each Wait checks `closed` first and returns at once — ctx.Err()
// if ctx is already done, else ErrLimiterClosed. These tests would FAIL on the
// old code (the elapsed assertion blows the budget; the error is
// context.DeadlineExceeded, not ErrLimiterClosed).

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestAllAlgorithms_WaitOnClosedReturnsPromptly is the headline regression: for
// every algorithm, Wait on a closed limiter with a long context returns
// essentially at once and reports ErrLimiterClosed. Under the old busy-loop code
// each case would block for the full 1s budget and return DeadlineExceeded.
func TestAllAlgorithms_WaitOnClosedReturnsPromptly(t *testing.T) {
	for name, l := range allAlgorithms(100, 10, time.Second) {
		t.Run(name, func(t *testing.T) {
			l.Close()
			// 1s budget: deliberately long so the old busy-loop path would make
			// the elapsed assertion below fail by a wide margin.
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			start := time.Now()
			err := l.Wait(ctx)
			elapsed := time.Since(start)

			if !errors.Is(err, ErrLimiterClosed) {
				t.Fatalf("%s: Wait on closed limiter err=%v want ErrLimiterClosed", name, err)
			}
			// Prompt return: 50ms is generous against scheduler/-race jitter yet
			// ~20x under the 1s budget the old code consumed.
			if elapsed > 50*time.Millisecond {
				t.Fatalf("%s: Wait on closed limiter took %v; expected prompt return (old code busy-looped ~1s)", name, elapsed)
			}
		})
	}
}

// TestAllAlgorithms_WaitOnClosedPrefersCtxErr asserts the precedence rule: when
// the limiter is closed AND ctx is already done, Wait surfaces ctx.Err() (the
// more informative condition) rather than the closed sentinel.
func TestAllAlgorithms_WaitOnClosedPrefersCtxErr(t *testing.T) {
	for name, l := range allAlgorithms(100, 10, time.Second) {
		t.Run(name, func(t *testing.T) {
			l.Close()
			ctx, cancel := context.WithCancel(context.Background())
			cancel() // ctx done must beat the closed sentinel
			err := l.Wait(ctx)
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("%s: Wait err=%v want context.Canceled (ctx done beats closed)", name, err)
			}
		})
	}
}

// TestTokenBucket_WaitClosedNoBusyLoop is the -race-friendly busy-loop guard.
// It runs Wait on a closed limiter with a live (uncancelled) context on a
// background goroutine and asserts the goroutine exits within a tight budget,
// proving Wait did not arm timers and spin. A busy-looping implementation would
// keep the goroutine alive until the 200ms context expired.
//
// We use the concrete type so the closed flag is observable directly and the
// test stays deterministic (no NewLimiter plumbing). The behaviour under test
// lives in tokenBucket.Wait, which is representative of all five impls (they
// share the identical guard added by the fix).
func TestTokenBucket_WaitClosedNoBusyLoop(t *testing.T) {
	tb := newTokenBucket(100, 5)
	tb.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- tb.Wait(ctx) }()

	select {
	case err := <-done:
		if !errors.Is(err, ErrLimiterClosed) {
			t.Fatalf("Wait err=%v want ErrLimiterClosed", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Wait on closed limiter did not return within 100ms — busy-looping (old behaviour)")
	}

	// Sanity: the context we handed Wait is still live, proving it returned of
	// its own accord (the closed short-circuit) rather than by ctx expiry.
	if ctx.Err() != nil {
		t.Fatalf("context unexpectedly done (%v) — Wait should have short-circuited on closed before ctx expired", ctx.Err())
	}
}

// TestAllAlgorithms_WaitClosedDuringBlock covers the Close-DURING-Wait race,
// which the entry-only check above cannot catch: Close arriving AFTER Wait has
// already entered its poll loop, blocked at capacity. The loop must re-check
// `closed` each iteration so a concurrent Close unblocks Wait within one poll
// instead of polling until ctx expires.
//
// Determinism: every algorithm is built with a FROZEN clock (a constant now),
// so after draining the single token no time advances — no token refills and no
// per-second window ever slides. Wait therefore blocks indefinitely for all five
// algorithms regardless of where the test lands on the wall-clock second, and
// Close is the only thing that can unblock it. (The poll sleep itself still uses
// real timers; only the token/window math sees the frozen clock.)
//
// BEFORE THIS FIX: each Wait's loop only consulted Allow() (false on close but
// indistinguishable from "at capacity"), so a blocked Wait ignored the Close and
// polled until ctx expired. This case FAILS the 500ms arm on the old code.
func TestAllAlgorithms_WaitClosedDuringBlock(t *testing.T) {
	frozen := func() time.Time { return time.Unix(1_000_000, 0) }
	cases := map[string]Limiter{
		"token_bucket":   func() Limiter { l := newTokenBucket(1, 1); l.now = frozen; return l }(),
		"leaky_bucket":   func() Limiter { l := newLeakyBucket(1, 1); l.now = frozen; return l }(),
		"gcra":           func() Limiter { l := newGCRA(1, 1); l.now = frozen; return l }(),
		"sliding_window": func() Limiter { l := newSlidingWindow(1, time.Second); l.now = frozen; return l }(),
		"fixed_window":   func() Limiter { l := newFixedWindow(1, time.Second); l.now = frozen; return l }(),
	}
	for name, l := range cases {
		t.Run(name, func(t *testing.T) {
			// Take the only token; with a frozen clock the next never arrives.
			if !l.Allow() {
				t.Fatalf("%s: drain Allow failed", name)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			done := make(chan error, 1)
			go func() { done <- l.Wait(ctx) }()

			// Let Wait enter the poll loop (it is blocked: the clock is frozen).
			time.Sleep(50 * time.Millisecond)
			l.Close() // close WHILE Wait is blocked at capacity

			select {
			case err := <-done:
				if !errors.Is(err, ErrLimiterClosed) {
					t.Fatalf("%s: Wait err=%v want ErrLimiterClosed", name, err)
				}
			case <-time.After(500 * time.Millisecond):
				t.Fatalf("%s: Wait did not return within 500ms of a Close issued while blocked — loop ignores concurrent Close", name)
			}
		})
	}
}

// TestTokenBucket_WaitClosedDoesNotSpinCounters is a complementary guard: it
// asserts Wait on a closed limiter never reaches the token-acquire path (Denied
// stays flat). The authoritative old-code-failure proofs are the two tests
// above; this one guards against a future regression where the closed guard is
// bypassed but Wait still returns quickly for some other reason.
func TestTokenBucket_WaitClosedDoesNotSpinCounters(t *testing.T) {
	tb := newTokenBucket(100, 5)
	tb.Close()

	before := tb.Metrics().Denied

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	_ = tb.Wait(ctx)

	after := tb.Metrics().Denied
	if after != before {
		t.Fatalf("Denied counter advanced %d -> %d on closed Wait; Wait must not call Allow (no busy-loop)", before, after)
	}
}
