package retry_test

import (
	"context"
	"testing"

	"github.com/v8fg/kit4go/retry"
)

// FuzzDoAttemptBoundary verifies Do's attempt-counting boundary: a fn that
// fails the first `succeedAfter` calls then succeeds must yield success within
// MaxAttempts when succeedAfter < MaxAttempts (Tries == succeedAfter+1), and a
// failure with Tries == MaxAttempts when the success never lands in time. This
// pins the off-by-one in the attempt loop (loop runs MaxAttempts times; the
// backoff is skipped on the last attempt). NoBackoff keeps the fuzz fast.
func FuzzDoAttemptBoundary(f *testing.F) {
	f.Add(0, 3) // succeeds immediately
	f.Add(2, 3) // succeeds on the 3rd try, within MaxAttempts=3
	f.Add(3, 3) // fails exactly MaxAttempts times -> failure
	f.Add(5, 3) // fails past MaxAttempts -> failure

	f.Fuzz(func(t *testing.T, succeedAfter, maxAttempts int) {
		if succeedAfter < 0 || succeedAfter > 50 || maxAttempts < 1 || maxAttempts > 50 {
			t.Skip("bounded for a fast fuzz")
		}
		calls := 0
		fn := func(context.Context) (int, error) {
			calls++
			if calls > succeedAfter {
				return calls, nil
			}
			return 0, errFuzzFail
		}
		// NoBackoff is the Config default, so no backoff option is needed.
		res := retry.Do(context.Background(), fn, retry.WithMaxAttempts(maxAttempts))

		switch {
		case succeedAfter < maxAttempts:
			// fn succeeds on attempt succeedAfter+1 (<= MaxAttempts).
			if res.Err != nil {
				t.Fatalf("expected success, got err=%v (succeedAfter=%d maxAttempts=%d)",
					res.Err, succeedAfter, maxAttempts)
			}
			if res.Tries != succeedAfter+1 {
				t.Fatalf("expected Tries=%d, got %d (succeedAfter=%d maxAttempts=%d)",
					succeedAfter+1, res.Tries, succeedAfter, maxAttempts)
			}
		default:
			// The success never lands within MaxAttempts attempts.
			if res.Err == nil {
				t.Fatalf("expected failure, got success (succeedAfter=%d maxAttempts=%d)",
					succeedAfter, maxAttempts)
			}
			if res.Tries != maxAttempts {
				t.Fatalf("expected Tries=%d, got %d (succeedAfter=%d maxAttempts=%d)",
					maxAttempts, res.Tries, succeedAfter, maxAttempts)
			}
		}
	})
}

var errFuzzFail = fuzzErr{}

type fuzzErr struct{}

func (fuzzErr) Error() string { return "fuzz: fail" }
