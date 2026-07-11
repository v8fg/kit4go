package fsm

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	StateIdle      State = "idle"
	StatePending   State = "pending"
	StatePaid      State = "paid"
	StateShipped   State = "shipped"
	StateCancelled State = "cancelled"
)

const (
	EventSubmit = "submit"
	EventPay    = "pay"
	EventShip   = "ship"
	EventCancel = "cancel"
)

func orderRules() []Rule {
	return []Rule{
		{From: StateIdle, Event: EventSubmit, To: StatePending},
		{From: StatePending, Event: EventPay, To: StatePaid},
		{From: StatePending, Event: EventCancel, To: StateCancelled},
		{From: StatePaid, Event: EventShip, To: StateShipped},
	}
}

func mustNew(t *testing.T, initial State, rules ...Rule) *Machine {
	t.Helper()
	m, err := New(initial, rules...)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestMachine_BasicTransition(t *testing.T) {
	m := mustNew(t, StateIdle, orderRules()...)
	if !m.Is(StateIdle) {
		t.Fatalf("initial state = %s, want idle", m.Current())
	}
	if err := m.Send(EventSubmit, nil); err != nil {
		t.Fatal(err)
	}
	if !m.Is(StatePending) {
		t.Fatalf("after submit = %s, want pending", m.Current())
	}
	if err := m.Send(EventPay, nil); err != nil {
		t.Fatal(err)
	}
	if !m.Is(StatePaid) {
		t.Fatalf("after pay = %s, want paid", m.Current())
	}
}

func TestMachine_NoTransition(t *testing.T) {
	m := mustNew(t, StateIdle, orderRules()...)
	err := m.Send(EventPay, nil)
	if !errors.Is(err, ErrNoTransition) {
		t.Fatalf("expected ErrNoTransition, got %v", err)
	}
	if !m.Is(StateIdle) {
		t.Fatal("state changed on failed transition")
	}
}

func TestMachine_Guard(t *testing.T) {
	rules := orderRules()
	rules[1].Guard = func(ctx any) bool {
		amount, _ := ctx.(int)
		return amount > 0
	}
	m := mustNew(t, StateIdle, rules...)

	_ = m.Send(EventSubmit, nil)
	if err := m.Send(EventPay, 0); !errors.Is(err, ErrGuardRejected) {
		t.Fatalf("expected ErrGuardRejected for amount=0, got %v", err)
	}
	if !m.Is(StatePending) {
		t.Fatal("guard rejection should not change state")
	}
	if err := m.Send(EventPay, 100); err != nil {
		t.Fatalf("pay with amount=100: %v", err)
	}
	if !m.Is(StatePaid) {
		t.Fatalf("after pay = %s, want paid", m.Current())
	}
}

func TestMachine_Action(t *testing.T) {
	var actionRan atomic.Bool
	rules := orderRules()
	rules[1].Action = func(ctx any) error {
		actionRan.Store(true)
		return nil
	}
	m := mustNew(t, StateIdle, rules...)
	_ = m.Send(EventSubmit, nil)
	_ = m.Send(EventPay, nil)
	if !actionRan.Load() {
		t.Fatal("action did not run")
	}
}

func TestMachine_ActionError(t *testing.T) {
	actionErr := errors.New("payment gateway down")
	rules := orderRules()
	rules[1].Action = func(ctx any) error {
		return actionErr
	}
	m := mustNew(t, StateIdle, rules...)
	_ = m.Send(EventSubmit, nil)
	err := m.Send(EventPay, nil)
	if !errors.Is(err, ErrActionFailed) {
		t.Fatalf("expected ErrActionFailed, got %v", err)
	}
	if !m.Is(StatePending) {
		t.Fatal("state changed on action error")
	}
}

func TestMachine_OnEnterOnExit(t *testing.T) {
	var enterPaid atomic.Bool
	var exitPending atomic.Bool
	m := mustNew(t, StateIdle, orderRules()...)
	m.OnEnter(StatePaid, func(ctx any) { enterPaid.Store(true) })
	m.OnExit(StatePending, func(ctx any) { exitPending.Store(true) })

	_ = m.Send(EventSubmit, nil)
	_ = m.Send(EventPay, nil)
	if !exitPending.Load() {
		t.Fatal("OnExit(pending) did not fire")
	}
	if !enterPaid.Load() {
		t.Fatal("OnEnter(paid) did not fire")
	}
}

func TestMachine_Listener(t *testing.T) {
	var mu sync.Mutex
	var transitions []struct{ from, to, event string }
	m := mustNew(t, StateIdle, orderRules()...)
	m.Listen(func(from, to State, event string, ctx any) {
		mu.Lock()
		defer mu.Unlock()
		transitions = append(transitions, struct{ from, to, event string }{from, to, event})
	})

	_ = m.Send(EventSubmit, nil)
	_ = m.Send(EventPay, nil)
	_ = m.Send(EventShip, nil)

	mu.Lock()
	defer mu.Unlock()
	if len(transitions) != 3 {
		t.Fatalf("listener fired %d times, want 3", len(transitions))
	}
	if transitions[2].to != StateShipped {
		t.Fatalf("last transition to = %s, want shipped", transitions[2].to)
	}
}

func TestMachine_Can(t *testing.T) {
	rules := orderRules()
	rules[1].Guard = func(ctx any) bool {
		ok, _ := ctx.(bool)
		return ok
	}
	m := mustNew(t, StateIdle, rules...)

	// Rule exists for (idle, submit) and has no guard → Can must return true
	// regardless of ctx. This covers the nil-guard branch of Can.
	if !m.Can(EventSubmit, nil) {
		t.Fatal("Can(submit, nil) on a guardless rule should be true")
	}
	if !m.Can(EventSubmit, "anything") {
		t.Fatal("Can(submit, ...) on a guardless rule should be true for any ctx")
	}

	_ = m.Send(EventSubmit, nil)

	if !m.Can(EventPay, true) {
		t.Fatal("Can(pay, true) should be true")
	}
	if m.Can(EventPay, false) {
		t.Fatal("Can(pay, false) should be false (guard rejects)")
	}
	if m.Can(EventShip, nil) {
		t.Fatal("Can(ship) from pending should be false (no rule)")
	}
}

func TestMachine_ConcurrentSafe(t *testing.T) {
	m := mustNew(t, StateIdle, orderRules()...)
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			for range 100 {
				_ = m.Current()
				_ = m.Is(StateIdle)
			}
		})
	}
	wg.Go(func() {
		_ = m.Send(EventSubmit, nil)
	})
	wg.Wait()
}

func TestMachine_AvailableEvents(t *testing.T) {
	m := mustNew(t, StateIdle, orderRules()...)
	events := m.AvailableEvents()
	if len(events) != 1 || events[0] != EventSubmit {
		t.Fatalf("from idle, available = %v, want [submit]", events)
	}
	_ = m.Send(EventSubmit, nil)
	events = m.AvailableEvents()
	if len(events) != 2 {
		t.Fatalf("from pending, available = %v, want 2 events", events)
	}
}

func TestMachine_EmptyRules(t *testing.T) {
	m := mustNew(t, "start")
	if err := m.Send("anything", nil); !errors.Is(err, ErrNoTransition) {
		t.Fatalf("empty machine: %v", err)
	}
}

func TestMachine_DuplicateRule(t *testing.T) {
	_, err := New(StateIdle,
		Rule{From: StateIdle, Event: "x", To: StatePending},
		Rule{From: StateIdle, Event: "x", To: StatePaid},
	)
	if err == nil {
		t.Fatal("expected error for duplicate rule")
	}
}

// TestMachine_ActionCallingBackDoesNotDeadlock is a regression test for the R19
// P1 finding: an Action used to run while holding m.mu.Lock(), so an action that
// called back into the machine (here m.Current(), which takes RLock) self-
// deadlocked on sync.RWMutex (which is not reentrant). The fix runs the action
// outside the lock. On the old code this test hangs and the -timeout kills it.
func TestMachine_ActionCallingBackDoesNotDeadlock(t *testing.T) {
	var seenInAction atomic.Bool
	var stateFromInside State
	rules := orderRules()
	rules[1].Action = func(ctx any) error {
		// Calling back into the machine must not deadlock. Before the fix this
		// RLock blocks forever because the Send goroutine still holds Lock.
		stateFromInside = m_currentForTest(t, ctx)
		seenInAction.Store(true)
		return nil
	}
	m := mustNew(t, StateIdle, rules...)
	_ = m.Send(EventSubmit, nil)

	done := make(chan error, 1)
	go func() { done <- m.Send(EventPay, m) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Send returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Send deadlocked: action calling m.Current() blocked under the lock")
	}

	if !seenInAction.Load() {
		t.Fatal("action did not run")
	}
	// The state is committed only AFTER a successful action; during the action
	// the machine is still in `from`. The captured state reflects that.
	if stateFromInside != StatePending {
		t.Fatalf("state seen by action = %s, want %s (transition not yet committed)", stateFromInside, StatePending)
	}
	if !m.Is(StatePaid) {
		t.Fatalf("after action success, state = %s, want %s", m.Current(), StatePaid)
	}
}

// m_currentForTest calls Machine.Current on the *Machine carried in ctx, so the
// action exercises the re-entrancy path that used to deadlock. It lives here
// (not as a method) purely to thread the *testing.T for fatal diagnostics.
func m_currentForTest(t *testing.T, ctx any) State {
	t.Helper()
	m, ok := ctx.(*Machine)
	if !ok {
		t.Fatalf("ctx must be *Machine for this test, got %T", ctx)
	}
	return m.Current()
}

// TestMachine_ConcurrentSendSerialized guards the sendMu fix: two concurrent
// Sends from the same state must NOT both run their actions. The first
// acquires sendMu, runs its action, and commits (state moves off `from`); the
// second then sees the new state and returns ErrNoTransition. Before the fix
// both actions ran and the committed state clobbered — a hazard for
// side-effecting actions (e.g. concurrent pay+cancel both charging).
func TestMachine_ConcurrentSendSerialized(t *testing.T) {
	var ran atomic.Int64
	slow := func(any) error {
		ran.Add(1)
		time.Sleep(20 * time.Millisecond) // widen the window so both Sends are in flight
		return nil
	}
	m, err := New(StateIdle,
		Rule{From: StateIdle, Event: "e1", To: "s1", Action: slow},
		Rule{From: StateIdle, Event: "e2", To: "s2", Action: slow},
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	errs := make([]error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); errs[0] = m.Send("e1", nil) }()
	go func() { defer wg.Done(); errs[1] = m.Send("e2", nil) }()
	wg.Wait()

	if got := ran.Load(); got != 1 {
		t.Fatalf("actions ran = %d, want 1 (concurrent Sends must serialize: only the first transition's action runs)", got)
	}
	successes := 0
	for _, e := range errs {
		if e == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("successes = %d, want 1 (one transition commits, the other gets ErrNoTransition); errs=%v", successes, errs)
	}
}
