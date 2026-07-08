// Package shutdown orchestrates component lifecycle: start in dependency order,
// stop in reverse order, each step bounded by a timeout. Pure standard library.
//
// Register components with Add (a name, start/stop funcs, and the names they
// depend on). Start runs them in topological order (deps first); Stop runs the
// successful ones in reverse order, each with its own timeout, collecting every
// error. Run ties it together: Start, block until the context is done (or a
// signal arrives when WithSignal is set), then Stop.
//
// Cross-cutting use: every service that owns a consumer pool, a gRPC/HTTP
// server, a background worker, and a DB pool needs a single place to bring them
// up and — more importantly — tear them down gracefully on SIGTERM/SIGINT in the
// right order. This is that place.
package shutdown

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"time"
)

// ErrCycle is returned when the dependency graph has a cycle.
var ErrCycle = errors.New("shutdown: dependency cycle")

// ErrMissingDep is returned when a component depends on a name not registered.
var ErrMissingDep = errors.New("shutdown: missing dependency")

// ErrDuplicate is returned when Add registers a duplicate name.
var ErrDuplicate = errors.New("shutdown: duplicate component")

// ErrShutdown collects per-component stop errors so a failure in one component
// does not mask the others.
type ErrShutdown struct{ Errors []ComponentError }

// ComponentError pairs a component name with its stop error.
type ComponentError struct {
	Name string
	Err  error
}

func (e *ErrShutdown) Error() string {
	if len(e.Errors) == 1 {
		return fmt.Sprintf("shutdown: %s: %v", e.Errors[0].Name, e.Errors[0].Err)
	}
	msgs := make([]string, 0, len(e.Errors))
	for _, ce := range e.Errors {
		msgs = append(msgs, fmt.Sprintf("%s: %v", ce.Name, ce.Err))
	}
	return fmt.Sprintf("shutdown: %d errors: %v", len(e.Errors), msgs)
}

type component struct {
	name  string
	start func(context.Context) error
	stop  func(context.Context) error
	deps  []string
}

// Manager owns a set of components' start/stop ordering.
//
// Concurrency: safe for concurrent use. Add/Register/Start/Wait/Shutdown all
// acquire an internal sync.Mutex. Wait blocks the calling goroutine until the
// shutdown trigger fires; WaitSignal starts a signal-listener goroutine that
// releases every Waiter on the configured OS signal. Shutdown runs each
// component's stop function in reverse start order, collecting per-component
// errors into ErrShutdown.
type Manager struct {
	mu           sync.Mutex
	components   []*component
	byName       map[string]*component
	order        []string // resolved topological order (deps first), nil until resolved
	stopTimeout  time.Duration
	startTimeout time.Duration
	signals      []os.Signal
}

// Option configures a Manager.
type Option func(*Manager)

// WithStopTimeout sets the per-component stop budget (default 10s). A component
// whose Stop exceeds it is abandoned (its ctx cancels) but the shutdown of the
// rest continues.
func WithStopTimeout(d time.Duration) Option { return func(m *Manager) { m.stopTimeout = d } }

// WithStartTimeout sets the per-component start budget (default 30s).
func WithStartTimeout(d time.Duration) Option { return func(m *Manager) { m.startTimeout = d } }

// WithSignal enables signal-triggered shutdown: Run will cancel its context when
// any of sigs is received. With no args it defaults to SIGINT and (on unix)
// SIGTERM.
func WithSignal(sigs ...os.Signal) Option {
	return func(m *Manager) {
		if len(sigs) == 0 {
			def := []os.Signal{os.Interrupt}
			if unixSIGTERM != nil {
				def = append(def, unixSIGTERM)
			}
			m.signals = def
			return
		}
		m.signals = sigs
	}
}

// New builds a Manager.
func New(opts ...Option) *Manager {
	m := &Manager{
		byName:       make(map[string]*component),
		stopTimeout:  10 * time.Second,
		startTimeout: 30 * time.Second,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Add registers a component. start/stop may be nil (no-op). dependsOn names must
// be registered (or added later) before Start; missing names are reported then.
// Returns ErrDuplicate for a repeated name.
func (m *Manager) Add(name string, start, stop func(context.Context) error, dependsOn ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.byName == nil {
		m.byName = make(map[string]*component)
	}
	if _, ok := m.byName[name]; ok {
		return fmt.Errorf("%w: %s", ErrDuplicate, name)
	}
	c := &component{name: name, start: start, stop: stop, deps: dependsOn}
	m.components = append(m.components, c)
	m.byName[name] = c
	m.order = nil // invalidate resolved order
	return nil
}

// resolveLocked computes the topological order (deps first). Returns
// ErrMissingDep for unknown deps and ErrCycle for cyclic graphs.
func (m *Manager) resolveLocked() error {
	// Validate deps exist.
	for _, c := range m.components {
		for _, d := range c.deps {
			if _, ok := m.byName[d]; !ok {
				return fmt.Errorf("%w: %s depends on unknown %s", ErrMissingDep, c.name, d)
			}
		}
	}
	// DFS-based topological sort.
	state := make(map[string]int) // 0=unvisited, 1=on-stack, 2=done
	var order []string
	var visit func(c *component) error
	visit = func(c *component) error {
		switch state[c.name] {
		case 1:
			return fmt.Errorf("%w: involving %s", ErrCycle, c.name)
		case 2:
			return nil
		}
		state[c.name] = 1
		for _, d := range c.deps {
			if err := visit(m.byName[d]); err != nil {
				return err
			}
		}
		state[c.name] = 2
		order = append(order, c.name)
		return nil
	}
	// Visit in registration order for determinism.
	for _, c := range m.components {
		if err := visit(c); err != nil {
			return err
		}
	}
	m.order = order
	return nil
}

// Start runs every component's start hook in dependency order. If a start fails
// (returns an error or times out), already-started components are stopped in
// reverse order before the error is returned.
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	if err := m.resolveLocked(); err != nil {
		m.mu.Unlock()
		return err
	}
	// Snapshot (name, *component) under the lock so the map read does not race a
	// concurrent Add; the start calls run outside the lock as before.
	type startItem struct {
		name string
		c    *component
	}
	items := make([]startItem, len(m.order))
	for i, name := range m.order {
		items[i] = startItem{name: name, c: m.byName[name]}
	}
	m.mu.Unlock()

	started := make([]string, 0, len(items))
	for _, it := range items {
		if it.c == nil || it.c.start == nil {
			started = append(started, it.name)
			continue
		}
		sctx, cancel := context.WithTimeout(ctx, m.startTimeout)
		err := safeStart(it.c.start, sctx)
		cancel()
		if err != nil {
			// Roll back: stop what we started, in reverse. The rollback error is
			// secondary to the start failure, so it is intentionally ignored.
			_ = m.stopReverse(ctx, reverseStrings(started))
			return fmt.Errorf("shutdown: start %s: %w", it.name, err)
		}
		started = append(started, it.name)
		if ctx.Err() != nil {
			_ = m.stopReverse(ctx, reverseStrings(started))
			return ctx.Err()
		}
	}
	return nil
}

// Stop runs every registered component's stop hook in reverse dependency order.
// Each stop gets its own timeout; a failure or timeout in one component does not
// stop the rest. All errors are aggregated into an *ErrShutdown (nil if none).
func (m *Manager) Stop(ctx context.Context) error {
	m.mu.Lock()
	if m.order == nil {
		if err := m.resolveLocked(); err != nil {
			m.mu.Unlock()
			return err
		}
	}
	order := append([]string(nil), m.order...)
	m.mu.Unlock()
	return m.stopReverse(ctx, reverseStrings(order))
}

// safeStop runs a component's stop hook with panic recovery. A panic is turned
// into an error (aggregated into ErrShutdown) so one panicking stop does not
// abort the teardown of the remaining components — preserving the package's
// "a failure in one component does not mask the others" contract for panics.
func safeStop(stop func(context.Context) error, ctx context.Context) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("shutdown: stop panic recovered: %v", r)
		}
	}()
	return stop(ctx)
}

// safeStart runs a component's start hook with panic recovery. A panic is
// turned into an error so the caller (Start) treats it as a normal start
// failure — which triggers the reverse-order rollback of already-started
// components. This is the symmetric twin of safeStop: without it, a panicking
// start hook escapes Start before stopReverse runs, so already-started
// components are never stopped.
func safeStart(start func(context.Context) error, ctx context.Context) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("shutdown: start panic recovered: %v", r)
		}
	}()
	return start(ctx)
}

func (m *Manager) stopReverse(ctx context.Context, names []string) error {
	// Snapshot name -> *component under the lock (a concurrent Add mutates the
	// map); the stop calls run outside the lock so a slow stop doesn't block Add.
	type stopItem struct {
		name string
		c    *component
	}
	m.mu.Lock()
	items := make([]stopItem, 0, len(names))
	for _, name := range names {
		items = append(items, stopItem{name: name, c: m.byName[name]})
	}
	m.mu.Unlock()

	var errs []ComponentError
	for _, it := range items {
		if it.c == nil || it.c.stop == nil {
			continue
		}
		sctx, cancel := context.WithTimeout(ctx, m.stopTimeout)
		err := safeStop(it.c.stop, sctx)
		cancel()
		if err != nil {
			errs = append(errs, ComponentError{Name: it.name, Err: err})
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return &ErrShutdown{Errors: errs}
}

// Run starts the components, blocks until ctx is done (or a configured signal),
// then stops them. Returns the result of Stop (or the Start error). This is the
// typical entrypoint for a main function.
func (m *Manager) Run(ctx context.Context) error {
	if len(m.signals) > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithCancel(ctx)
		defer cancel()
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, m.signals...)
		defer signal.Stop(ch)
		go func() {
			select {
			case <-ch:
				cancel()
			case <-ctx.Done():
			}
		}()
	}
	if err := m.Start(ctx); err != nil {
		return err
	}
	<-ctx.Done()
	return m.Stop(context.Background())
}

// Components returns the registered component names in resolved start order.
// Resolve errors are returned (order is empty in that case).
func (m *Manager) Components() ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.order == nil {
		if err := m.resolveLocked(); err != nil {
			return nil, err
		}
	}
	return append([]string(nil), m.order...), nil
}

func reverseStrings(a []string) []string {
	out := make([]string, len(a))
	for i, s := range a {
		out[len(a)-1-i] = s
	}
	return out
}
