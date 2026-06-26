package tcpclient

import (
	"context"
	"sync"
	"testing"
	"time"
)

// latObserver is a minimal LatencyObserver that records every Observe call,
// used to verify the wiring fires once per public method with a real duration.
type latObserver struct {
	mu     sync.Mutex
	count  int
	total  time.Duration
	minDur time.Duration
}

func (f *latObserver) Observe(d time.Duration) {
	f.mu.Lock()
	f.count++
	f.total += d
	if f.minDur == 0 || d < f.minDur {
		f.minDur = d
	}
	f.mu.Unlock()
}

// TestClient_LatencyObserver_Fires confirms Send invokes the observer exactly
// once per call with a positive duration when ClientOptions.Latency is set.
// (SendReceive/SendReceiveLine share the identical wiring, so this one path
// covers all three.)
func TestClient_LatencyObserver_Fires(t *testing.T) {
	ln, _ := benchEchoListener(t)
	defer ln.Close()

	obs := &latObserver{}
	c := NewClient(ClientOptions{
		Network: "tcp",
		Address: ln.Addr().String(),
		Latency: obs,
	})
	defer c.Close()

	const n = 5
	for i := 0; i < n; i++ {
		if err := c.Send(context.Background(), []byte("x")); err != nil {
			t.Fatalf("send: %v", err)
		}
	}
	if obs.count != n {
		t.Fatalf("observe count=%d, want %d", obs.count, n)
	}
	if obs.minDur <= 0 {
		t.Errorf("min observed duration=%v, want > 0", obs.minDur)
	}
}
