package fsm

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
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
