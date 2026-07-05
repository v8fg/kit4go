package pipeline

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestPipeline_SetOnPanic(t *testing.T) {
	p := New(1, func(_ context.Context, _ int) (int, bool, error) { return 0, true, nil })
	var fired atomic.Bool
	p.SetOnPanic(func(any) { fired.Store(true) })
	if p.Recovered() != 0 {
		t.Fatal("initial recovered should be 0")
	}
}

func TestPipeline_Recovered(t *testing.T) {
	p := New(1, func(_ context.Context, _ int) (int, bool, error) { return 0, true, nil })
	if p.Recovered() != 0 {
		t.Fatal("initial recovered should be 0")
	}
}

func TestPipeline_StagePanicRecovered(t *testing.T) {
	var hookFired atomic.Bool
	p := New(1, func(_ context.Context, n int) (int, bool, error) {
		if n == 42 {
			panic("boom")
		}
		return n, true, nil
	})
	p.SetOnPanic(func(any) { hookFired.Store(true) })

	go func() {
		_ = p.Send(context.Background(), 42)
		_ = p.Send(context.Background(), 1)
		time.Sleep(50 * time.Millisecond)
		p.Close()
	}()

	count := 0
	for range p.Out() {
		count++
	}
	if count == 0 {
		t.Fatal("no items received — worker may have died from panic")
	}
	if p.Recovered() != 1 {
		t.Fatalf("Recovered = %d, want 1", p.Recovered())
	}
	if !hookFired.Load() {
		t.Fatal("onPanic hook not fired")
	}
}
