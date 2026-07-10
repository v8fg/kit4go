package limiter

import (
	"context"
	"testing"
	"time"
)

func TestTokenBucket_WaitImmediateSuccess(t *testing.T) {
	l := NewLimiter(LimiterOptions{Algorithm: AlgorithmTokenBucket, Rate: 100, Burst: 10})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := l.Wait(ctx); err != nil {
		t.Fatalf("Wait should succeed immediately with burst: %v", err)
	}
}

func TestTokenBucket_WaitCtxCancelledWhileWaiting(t *testing.T) {
	l := NewLimiter(LimiterOptions{Algorithm: AlgorithmTokenBucket, Rate: 1, Burst: 1})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_ = l.Wait(context.Background())
	if err := l.Wait(ctx); err == nil {
		t.Fatal("Wait should return ctx.Err() when blocked")
	}
}

func TestTokenBucket_WaitSuccessAfterRefill(t *testing.T) {
	l := NewLimiter(LimiterOptions{Algorithm: AlgorithmTokenBucket, Rate: 1000, Burst: 1})
	_ = l.Wait(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := l.Wait(ctx); err != nil {
		t.Fatalf("Wait should succeed after refill: %v", err)
	}
}

func TestSlidingWindow_WaitImmediateSuccess(t *testing.T) {
	l := NewLimiter(LimiterOptions{Algorithm: AlgorithmSlidingWindow, Rate: 100, Burst: 10})
	if err := l.Wait(context.Background()); err != nil {
		t.Fatalf("Wait immediate: %v", err)
	}
}

func TestSlidingWindow_WaitCtxCancelled(t *testing.T) {
	// Frozen clock: the per-second window cannot slide, so a drained limiter
	// stays at capacity and Wait must time out. With the real clock this was
	// flaky — landing near a wall-second boundary slid the window inside the
	// 50ms budget and let Wait succeed.
	sw := newSlidingWindow(1, time.Second)
	sw.now = func() time.Time { return time.Unix(1_000_000, 0) }
	_ = sw.Wait(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := sw.Wait(ctx); err == nil {
		t.Fatal("should timeout")
	}
}

func TestFixedWindow_WaitImmediateSuccess(t *testing.T) {
	l := NewLimiter(LimiterOptions{Algorithm: AlgorithmFixedWindow, Rate: 100, Burst: 10})
	if err := l.Wait(context.Background()); err != nil {
		t.Fatalf("Wait immediate: %v", err)
	}
}

func TestFixedWindow_WaitCtxCancelled(t *testing.T) {
	// Frozen clock: the window cannot reset, so a drained limiter stays at
	// capacity and Wait must time out. With the real clock this was flaky —
	// landing near a wall-second boundary reset the window inside the 50ms
	// budget and let Wait succeed.
	fw := newFixedWindow(1, time.Second)
	fw.now = func() time.Time { return time.Unix(1_000_000, 0) }
	_ = fw.Wait(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := fw.Wait(ctx); err == nil {
		t.Fatal("should timeout")
	}
}

func TestFixedWindow_WaitSuccessAfterWindow(t *testing.T) {
	l := NewLimiter(LimiterOptions{Algorithm: AlgorithmFixedWindow, Rate: 1000, Burst: 1})
	_ = l.Wait(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := l.Wait(ctx); err != nil {
		t.Fatalf("Wait should succeed after window rollover: %v", err)
	}
}

func TestLeakyBucket_WaitImmediateSuccess(t *testing.T) {
	l := NewLimiter(LimiterOptions{Algorithm: AlgorithmLeakyBucket, Rate: 100, Burst: 10})
	if err := l.Wait(context.Background()); err != nil {
		t.Fatalf("Wait immediate: %v", err)
	}
}

func TestLeakyBucket_WaitCtxCancelled(t *testing.T) {
	l := NewLimiter(LimiterOptions{Algorithm: AlgorithmLeakyBucket, Rate: 1, Burst: 1})
	_ = l.Wait(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := l.Wait(ctx); err == nil {
		t.Fatal("should timeout")
	}
}

func TestLeakyBucket_WaitSuccessAfterDrain(t *testing.T) {
	l := NewLimiter(LimiterOptions{Algorithm: AlgorithmLeakyBucket, Rate: 1000, Burst: 1})
	_ = l.Wait(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := l.Wait(ctx); err != nil {
		t.Fatalf("Wait should succeed after drain: %v", err)
	}
}

func TestGCRA_WaitImmediateSuccess(t *testing.T) {
	l := NewLimiter(LimiterOptions{Algorithm: AlgorithmGCRA, Rate: 100, Burst: 10})
	if err := l.Wait(context.Background()); err != nil {
		t.Fatalf("Wait immediate: %v", err)
	}
}

func TestGCRA_WaitCtxCancelled(t *testing.T) {
	l := NewLimiter(LimiterOptions{Algorithm: AlgorithmGCRA, Rate: 1, Burst: 1})
	_ = l.Wait(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := l.Wait(ctx); err == nil {
		t.Fatal("should timeout")
	}
}

func TestGCRA_WaitSuccessAfterDelay(t *testing.T) {
	l := NewLimiter(LimiterOptions{Algorithm: AlgorithmGCRA, Rate: 1000, Burst: 1})
	_ = l.Wait(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := l.Wait(ctx); err != nil {
		t.Fatalf("Wait should succeed after delay: %v", err)
	}
}

func TestGCRA_WaitPreCancelledCtxAfterDrain(t *testing.T) {
	l := NewLimiter(LimiterOptions{Algorithm: AlgorithmGCRA, Rate: 1, Burst: 1})
	_ = l.Wait(context.Background()) // drain the burst token
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := l.Wait(ctx); err == nil {
		t.Fatal("pre-cancelled ctx should return ctx.Err() when blocked")
	}
}

func TestNewLimiter_ZeroRate(t *testing.T) {
	l := NewLimiter(LimiterOptions{Algorithm: AlgorithmTokenBucket, Rate: 0, Burst: 10})
	if l != nil {
		t.Fatal("zero rate should return nil")
	}
}
