package shutdown

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// recorder captures the order of start/stop events across components.
type recorder struct {
	mu  sync.Mutex
	log []string
}

func (r *recorder) record(s string) { r.mu.Lock(); r.log = append(r.log, s); r.mu.Unlock() }
func (r *recorder) slice() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.log...)
}

func TestStartRespectsDependencies(t *testing.T) {
	var rec recorder
	m := New()
	// db has no deps; api depends on db; worker depends on api.
	m.Add("db", func(ctx context.Context) error { rec.record("start:db"); return nil }, func(ctx context.Context) error { rec.record("stop:db"); return nil })
	m.Add("api", func(ctx context.Context) error { rec.record("start:api"); return nil }, func(ctx context.Context) error { rec.record("stop:api"); return nil }, "db")
	m.Add("worker", func(ctx context.Context) error { rec.record("start:worker"); return nil }, func(ctx context.Context) error { rec.record("stop:worker"); return nil }, "api")

	require.NoError(t, m.Start(context.Background()))
	// Start order: deps first -> db, api, worker.
	require.Equal(t, []string{"start:db", "start:api", "start:worker"}, rec.slice())

	require.NoError(t, m.Stop(context.Background()))
	// Stop order: reverse -> worker, api, db.
	require.Equal(t,
		[]string{"start:db", "start:api", "start:worker", "stop:worker", "stop:api", "stop:db"},
		rec.slice())
}

func TestComponentsOrder(t *testing.T) {
	m := New()
	m.Add("a", nil, nil, "b")
	m.Add("b", nil, nil)
	order, err := m.Components()
	require.NoError(t, err)
	// b (no deps) before a (depends on b).
	require.Equal(t, []string{"b", "a"}, order)
}

func TestCycleDetected(t *testing.T) {
	m := New()
	m.Add("a", nil, nil, "b")
	m.Add("b", nil, nil, "a")
	err := m.Start(context.Background())
	require.ErrorIs(t, err, ErrCycle)
}

func TestMissingDependency(t *testing.T) {
	m := New()
	m.Add("a", nil, nil, "ghost")
	err := m.Start(context.Background())
	require.ErrorIs(t, err, ErrMissingDep)
}

func TestDuplicateComponent(t *testing.T) {
	m := New()
	require.NoError(t, m.Add("a", nil, nil))
	require.ErrorIs(t, m.Add("a", nil, nil), ErrDuplicate)
}

// A failing start rolls back already-started components (reverse stop).
func TestStartFailureRollsBack(t *testing.T) {
	var rec recorder
	m := New(WithStartTimeout(time.Second))
	m.Add("a", func(context.Context) error { rec.record("start:a"); return nil }, func(context.Context) error { rec.record("stop:a"); return nil })
	m.Add("b", func(context.Context) error { rec.record("start:b"); return errors.New("boom") }, nil, "a")

	err := m.Start(context.Background())
	require.Error(t, err)
	// a started, b failed -> a stopped during rollback.
	require.Equal(t, []string{"start:a", "start:b", "stop:a"}, rec.slice())
}

func TestStopAggregatesErrors(t *testing.T) {
	m := New(WithStopTimeout(time.Second))
	m.Add("a", nil, func(context.Context) error { return errors.New("a-down") })
	m.Add("b", nil, func(context.Context) error { return errors.New("b-down") }, "a") // depends so stops first
	err := m.Stop(context.Background())
	require.Error(t, err)
	var se *ErrShutdown
	require.ErrorAs(t, err, &se)
	require.Len(t, se.Errors, 2)
}

// A component whose Stop exceeds its timeout is abandoned; the rest still stop.
func TestStopTimeoutAbandonsSlowComponent(t *testing.T) {
	var rec recorder
	m := New(WithStopTimeout(20 * time.Millisecond))
	m.Add("slow", nil, func(ctx context.Context) error {
		select {
		case <-time.After(500 * time.Millisecond):
			rec.record("slow:done")
		case <-ctx.Done():
			rec.record("slow:abandoned")
		}
		return ctx.Err()
	})
	m.Add("fast", nil, func(context.Context) error { rec.record("fast:stop"); return nil }, "slow")

	err := m.Stop(context.Background()) // stops in reverse: fast first, then slow
	require.Error(t, err)
	// Both stopped (fast fully, slow abandoned by timeout); order is fast then slow.
	require.Contains(t, rec.slice(), "fast:stop")
	require.Contains(t, rec.slice(), "slow:abandoned")
}

// Run returns when the context is cancelled, then stops everything.
func TestRunCancelsAndStops(t *testing.T) {
	var rec recorder
	m := New()
	m.Add("svc", func(context.Context) error { rec.record("start"); return nil }, func(context.Context) error { rec.record("stop"); return nil })

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	require.NoError(t, m.Run(ctx))
	require.Equal(t, []string{"start", "stop"}, rec.slice())
}

// WithSignal installs a signal listener; sending the signal triggers Stop.
func TestRunWithSignal(t *testing.T) {
	var rec recorder
	// Use a real signal: SIGINT to the current process.
	m := New(WithSignal(os.Interrupt), WithStopTimeout(time.Second))
	m.Add("svc", func(context.Context) error { rec.record("start"); return nil }, func(context.Context) error { rec.record("stop"); return nil })

	done := make(chan error, 1)
	go func() { done <- m.Run(context.Background()) }()
	time.Sleep(30 * time.Millisecond) // let Start run
	require.NoError(t, sendSelfInterrupt())
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after signal")
	}
	require.Equal(t, []string{"start", "stop"}, rec.slice())
}

// sendSelfInterrupt sends SIGINT to the current process (unix only).
func sendSelfInterrupt() error {
	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		return err
	}
	return p.Signal(os.Interrupt)
}

func TestNilHooksAreNoOp(t *testing.T) {
	m := New()
	m.Add("a", nil, nil)
	require.NoError(t, m.Start(context.Background()))
	require.NoError(t, m.Stop(context.Background()))
}

func TestErrShutdownMessage(t *testing.T) {
	single := &ErrShutdown{Errors: []ComponentError{{Name: "db", Err: errors.New("x")}}}
	require.Contains(t, single.Error(), "db")
	multi := &ErrShutdown{Errors: []ComponentError{
		{Name: "db", Err: errors.New("x")},
		{Name: "api", Err: errors.New("y")},
	}}
	require.Contains(t, multi.Error(), "2 errors")
}

func TestWithSignalDefault(t *testing.T) {
	m := New(WithSignal())
	require.NotEmpty(t, m.signals)
	require.Contains(t, m.signals, os.Interrupt)
}

// Stop without a prior Start still resolves the graph and runs stop hooks.
func TestStopWithoutStartResolves(t *testing.T) {
	var rec recorder
	m := New(WithStopTimeout(time.Second))
	m.Add("a", nil, func(context.Context) error { rec.record("stop:a"); return nil })
	m.Add("b", nil, func(context.Context) error { rec.record("stop:b"); return nil }, "a")
	require.NoError(t, m.Stop(context.Background())) // resolves deps first
	// reverse order: b (depends on a) stops before a.
	require.Equal(t, []string{"stop:b", "stop:a"}, rec.slice())
}

func TestComponentsCycleError(t *testing.T) {
	m := New()
	m.Add("a", nil, nil, "b")
	m.Add("b", nil, nil, "a")
	_, err := m.Components()
	require.ErrorIs(t, err, ErrCycle)
}
