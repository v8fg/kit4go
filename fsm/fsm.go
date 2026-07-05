// Package fsm is a generic, thread-safe finite state machine. Define states and
// transitions declaratively, then drive the machine by sending events. Each
// transition can carry a guard (predicate) and an action (callback). Pure
// standard library.
//
// Ad-tech / finance uses: order lifecycle (created→pending→paid→shipped→cancelled),
// ad-request pipeline (received→validated→matched→bid→won/lost), payment state
// tracking (init→authorized→captured→refunded), workflow engines.
package fsm

import (
	"errors"
	"fmt"
	"sync"
)

// State is a named state in the machine. Use string constants for clarity.
type State = string

// Rule defines a single transition: when in From and event Event arrives, if
// Guard passes, run Action then move to To.
type Rule struct {
	From   State
	Event  string
	Guard  func(ctx any) bool  // nil = always accept
	Action func(ctx any) error // nil = no-op
	To     State
}

var (
	// ErrNoTransition is returned by Send when no rule matches the current
	// state + event.
	ErrNoTransition = errors.New("fsm: no transition for state/event")
	// ErrGuardRejected is returned by Send when the matching rule's guard
	// returned false.
	ErrGuardRejected = errors.New("fsm: guard rejected the event")
	// ErrActionFailed wraps an error returned by a rule's action.
	ErrActionFailed = errors.New("fsm: action failed")
)

// EventListener is called after every successful transition.
type EventListener func(from, to State, event string, ctx any)

type ruleKey struct {
	from  State
	event string
}

// Machine is a thread-safe finite state machine. Create with New.
type Machine struct {
	mu        sync.RWMutex
	current   State
	rules     map[ruleKey]Rule
	onEnter   map[State]func(ctx any)
	onExit    map[State]func(ctx any)
	listeners []EventListener
}

// New builds a machine starting in initial, with the given transition rules.
// Returns an error if two rules share the same (From, Event) pair.
func New(initial State, rules ...Rule) (*Machine, error) {
	m := &Machine{
		current: initial,
		rules:   make(map[ruleKey]Rule, len(rules)),
		onEnter: make(map[State]func(any)),
		onExit:  make(map[State]func(any)),
	}
	for _, r := range rules {
		key := ruleKey{r.From, r.Event}
		if _, exists := m.rules[key]; exists {
			return nil, fmt.Errorf("fsm: duplicate rule for (%s, %s)", r.From, r.Event)
		}
		m.rules[key] = r
	}
	return m, nil
}

// Send processes an event. It finds the rule for (current, event), checks the
// guard, runs the action, and transitions to the target state. On enter/exit
// hooks and listeners fire after the transition completes.
//
// Returns:
//   - ErrNoTransition: no rule for (current, event).
//   - ErrGuardRejected: the guard returned false.
//   - ErrActionFailed (wrapping): the action returned an error.
func (m *Machine) Send(event string, ctx any) error {
	m.mu.Lock()
	rule, ok := m.rules[ruleKey{m.current, event}]
	if !ok {
		m.mu.Unlock()
		return ErrNoTransition
	}
	if rule.Guard != nil && !rule.Guard(ctx) {
		m.mu.Unlock()
		return ErrGuardRejected
	}
	from := m.current
	if rule.Action != nil {
		if err := rule.Action(ctx); err != nil {
			m.mu.Unlock()
			return fmt.Errorf("%w: %w", ErrActionFailed, err)
		}
	}
	m.current = rule.To
	onExit := m.onExit[from]
	onEnter := m.onEnter[rule.To]
	listeners := m.listeners
	m.mu.Unlock()

	// Fire hooks outside the lock so they can call back into the machine
	// (e.g., Current()) without deadlocking.
	if onExit != nil {
		onExit(ctx)
	}
	if onEnter != nil {
		onEnter(ctx)
	}
	for _, l := range listeners {
		l(from, rule.To, event, ctx)
	}
	return nil
}

// Current returns the current state. Safe for concurrent use.
func (m *Machine) Current() State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

// Can reports whether sending event would cause a transition from the current
// state (i.e., a rule exists and its guard would pass). Does not mutate state.
func (m *Machine) Can(event string, ctx any) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	rule, ok := m.rules[ruleKey{m.current, event}]
	if !ok {
		return false
	}
	if rule.Guard != nil {
		return rule.Guard(ctx)
	}
	return true
}

// Is reports whether the machine is in the given state.
func (m *Machine) Is(s State) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current == s
}

// OnEnter registers a callback fired when the machine enters state (after the
// action, before listeners). Safe to call at any time.
func (m *Machine) OnEnter(state State, fn func(ctx any)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onEnter[state] = fn
}

// OnExit registers a callback fired when the machine leaves state (after the
// action, before OnEnter of the target).
func (m *Machine) OnExit(state State, fn func(ctx any)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onExit[state] = fn
}

// Listen adds a listener fired after every successful transition. The listener
// receives (from, to, event, ctx). Listeners are called in registration order.
func (m *Machine) Listen(fn EventListener) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listeners = append(m.listeners, fn)
}

// AvailableEvents returns the events that have rules from the current state
// (regardless of guard status). Useful for debugging or rendering available
// actions.
func (m *Machine) AvailableEvents() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var events []string
	for key := range m.rules {
		if key.from == m.current {
			events = append(events, key.event)
		}
	}
	return events
}
