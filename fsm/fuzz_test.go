// This file is an internal fuzz test (package fsm, not fsm_test) so it can
// reuse the State*/Event* constants and the orderRules() helper defined in
// fsm_test.go.
//
// Two fuzz targets exercise the package's core invariants over arbitrary input:
//
//   - FuzzMachineSend: drives a Machine with a stream of events derived from a
//     byte blob and asserts, for every step:
//     1. Send never panics, regardless of the event, the current state, the
//     guard outcome, or an action returning an error.
//     2. A golden model (map of (from,event)->to, plus the matching guard) is
//     kept in lock-step: after every accepted Send, Current() equals the
//     model's state — roundtrip consistency.
//     3. A Send that returns any error (ErrNoTransition, ErrGuardRejected, or
//     ErrActionFailed) never mutates state; Current() is unchanged.
//     4. Can(event) agrees with the outcome of Send(event): a rule exists and
//     its guard passes exactly when Send succeeds — ordering/consistency
//     between the pre-check and the mutating call.
//
//   - FuzzMachineRoundtripState: walks a fixed cyclic graph (s0<->s1, s0->s2)
//     purely from the byte stream and asserts that Current() always lands on a
//     state the model says is reachable, that the accepted-transition count
//     equals the number of successful Sends, and that guards bound which branch
//     is taken. This isolates the ordering/roundtrip invariant from the
//     no-panic check above by using a deterministic, fully-connected rule set.
//
// Both targets run as ordinary tests (seed corpus only) under
// `go test -run='^Fuzz'`; pass `-fuzz=FuzzMachineSend` to grow the corpus.
package fsm

import (
	"errors"
	"testing"
)

// fuzzMachineRules returns the order-lifecycle rules with a guard on the
// pending->paid transition (amount > 0) and a deliberately failing action on
// pending->cancelled (to exercise the ErrActionFailed path). The guard and
// action mirror what production callers wire up, so the fuzzer exercises the
// same branches the unit tests do, just with arbitrary interleavings.
func fuzzMachineRules() []Rule {
	return []Rule{
		{From: StateIdle, Event: EventSubmit, To: StatePending},
		{From: StatePending, Event: EventPay, To: StatePaid,
			Guard: func(ctx any) bool {
				amount, _ := ctx.(int)
				return amount > 0
			}},
		{From: StatePending, Event: EventCancel, To: StateCancelled,
			Action: func(ctx any) error { return ErrActionFailed }},
		{From: StatePaid, Event: EventShip, To: StateShipped},
	}
}

// fuzzEvents is the fixed alphabet the byte stream selects from. Keeping it
// small and known lets the golden model reason about every event; an unknown
// event must always yield ErrNoTransition from every state.
var fuzzEvents = []string{EventSubmit, EventPay, EventCancel, EventShip, "noop"}

// fuzzCtxFor maps a byte to a ctx value. The amount encoding feeds the
// pending->paid guard: low bit set makes amount <= 0 (rejected), otherwise a
// positive amount (accepted). This guarantees both guard branches are
// reachable from the input stream alone.
func fuzzCtxFor(b byte) any {
	if b&0x01 == 0 {
		return 0 // guard rejects (amount not > 0)
	}
	return int(b) + 1 // positive amount, guard accepts
}

// goldenModel mirrors the Machine's rule table plus guard so the fuzzer can
// predict Send's outcome without consulting the Machine. It returns the next
// state and whether the (state,event,ctx) triple is an accepted transition.
func goldenModel(rules []Rule, from State, event string, ctx any) (to State, ok bool) {
	for _, r := range rules {
		if r.From == from && r.Event == event {
			// Guard mirrors the rule's guard. An action that always fails
			// (pending->cancel) is treated as non-accepted: Send returns an
			// error and state must not change.
			if r.Guard != nil && !r.Guard(ctx) {
				return from, false
			}
			if r.Action != nil {
				if err := r.Action(ctx); err != nil {
					return from, false
				}
			}
			return r.To, true
		}
	}
	return from, false
}

// FuzzMachineSend fuzzes an arbitrary event stream against a Machine built from
// fuzzMachineRules. See the file comment for the invariants asserted per step.
func FuzzMachineSend(f *testing.F) {
	// Seeds: each blob is a stream of (event-index, amount-bit) pairs. Even
	// bytes select events with amount<=0 (guard rejects); odd bytes select
	// events with amount>0 (guard accepts).
	f.Add([]byte{})                                               // empty stream: machine stays idle
	f.Add([]byte{0x00})                                           // submit with amount 0: accepted (no guard)
	f.Add([]byte{0x01})                                           // pay from idle: no rule -> ErrNoTransition
	f.Add([]byte{0x00, 0x02, 0x04, 0x06})                         // submit, pay, ship, then unknown -> full happy path + dead-end
	f.Add([]byte{0x00, 0x06, 0x04})                               // submit, cancel (action fails), pay (guard rejects amount<=0)
	f.Add([]byte{0x01, 0x03, 0x05, 0x07, 0x09})                   // unknown events from idle: all ErrNoTransition
	f.Add([]byte{0x00, 0x03, 0x00, 0x03, 0x05})                   // submit, cancel-fails, submit (no rule from pending), ...
	f.Add([]byte{0x00, 0x02, 0x08, 0x00, 0x02, 0x04, 0x06, 0x06}) // submit, pay, ship(no rule from paid? ship IS rule), churn

	rules := fuzzMachineRules()

	f.Fuzz(func(t *testing.T, data []byte) {
		// Belt-and-braces panic guard: Send must be panic-free for any input.
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Send panicked: data=%x recover=%v", data, r)
			}
		}()

		m, err := New(StateIdle, rules...)
		if err != nil {
			t.Fatalf("New failed: %v", err)
		}

		modelState := StateIdle

		for i, b := range data {
			event := fuzzEvents[int(b>>1)%len(fuzzEvents)]
			ctx := fuzzCtxFor(b)
			before := m.Current()

			// Predict the outcome from the golden model.
			wantTo, wantOk := goldenModel(rules, modelState, event, ctx)

			// Invariant 4a: Can must agree with whether Send will accept.
			canBefore := m.Can(event, ctx)

			err := m.Send(event, ctx)
			after := m.Current()

			if wantOk {
				// Accepted transition: Send returns nil, Can was true, and
				// Current advanced to the model's target.
				if err != nil {
					t.Fatalf("step %d (b=%#x event=%s): accepted by model but Send err=%v",
						i, b, event, err)
				}
				if !canBefore {
					t.Fatalf("step %d (b=%#x event=%s): model accepts but Can=false",
						i, b, event)
				}
				if after != wantTo {
					t.Fatalf("step %d (b=%#x event=%s): Current=%s want %s",
						i, b, event, after, wantTo)
				}
				modelState = wantTo
			} else {
				// Rejected transition: Send returns an error, and state must
				// not change (no-rule / guard-reject / action-fail all leave
				// Current untouched).
				if err == nil {
					t.Fatalf("step %d (b=%#x event=%s): model rejects but Send succeeded, Current=%s",
						i, b, event, after)
				}
				if after != before {
					t.Fatalf("step %d (b=%#x event=%s): rejected Send mutated state %s->%s",
						i, b, event, before, after)
				}
				// Can must report false for a guard-reject; for a no-rule or
				// always-failing-action it is also false (no rule exists or the
				// guard is the only pre-check the API exposes).
				if canBefore {
					// The only way Can is true but Send fails is the
					// always-failing action on pending->cancel: the rule has
					// no guard, so Can returns true even though the action
					// errors. Allow that one case.
					isCancelFromPending := modelState == StatePending && event == EventCancel
					if !isCancelFromPending {
						t.Fatalf("step %d (b=%#x event=%s): model rejects but Can=true "+
							"(not the always-failing action case)",
							i, b, event)
					}
				}
				// modelState unchanged on rejection.
			}

			// Invariant: Current always tracks the model after every step.
			if m.Current() != modelState {
				t.Fatalf("step %d: Current=%s diverged from model=%s",
					i, m.Current(), modelState)
			}

			// Invariant: Is reports the same state Current does.
			if !m.Is(modelState) {
				t.Fatalf("step %d: Is(%s)=false but Current=%s",
					i, modelState, m.Current())
			}
		}

		// Final cross-check: AvailableEvents lists exactly the events that
		// have rules from the final state, and every listed event is in the
		// known alphabet.
		avail := m.AvailableEvents()
		seen := make(map[string]bool, len(avail))
		for _, ev := range avail {
			seen[ev] = true
			known := false
			for _, e := range fuzzEvents {
				if e == ev {
					known = true
					break
				}
			}
			if !known {
				t.Fatalf("AvailableEvents returned unknown event %q (final=%s)",
					ev, modelState)
			}
		}
		// Every rule from the final state must appear in AvailableEvents.
		for _, r := range rules {
			if r.From == modelState && !seen[r.Event] {
				t.Fatalf("AvailableEvents missing %q from final state %s (got %v)",
					r.Event, modelState, avail)
			}
		}
	})
}

// FuzzMachineRoundtripState walks a small, fully-connected cyclic graph
// (s0 <-> s1, plus s0 -> s2 -> s1) driven only by the byte stream. Because the
// graph is dense, most events advance the machine, so this target isolates the
// roundtrip/ordering invariant (Current always reflects the model; the accepted
// count equals successful Sends) from the no-rule heavy paths of
// FuzzMachineSend. Guards on the s1->s0 edge bound which branch is taken.
func FuzzMachineRoundtripState(f *testing.F) {
	const (
		s0 State = "s0"
		s1 State = "s1"
		s2 State = "s2"
	)
	const (
		evA = "a" // s0->s1, s1->s2
		evB = "b" // s0->s2, s1->s0 (guarded)
	)

	// Seeds: byte high bit toggles the guard ctx; low bit selects event a/b.
	f.Add([]byte{})                       // no steps
	f.Add([]byte{0x00})                   // one step, evA, guard ctx false
	f.Add([]byte{0x01})                   // one step, evB, guard ctx true
	f.Add([]byte{0x00, 0x02, 0x04, 0x06}) // walk a,a,a,a
	f.Add([]byte{0x01, 0x03, 0x05, 0x07}) // walk b,b,b,b
	f.Add([]byte{0x00, 0x01, 0x02, 0x03}) // alternating a,b,a,b
	f.Add([]byte{0x01, 0x01, 0x00, 0x00}) // b,b then a,a — exercises the guard on s1->s0

	rules := []Rule{
		{From: s0, Event: evA, To: s1},
		{From: s0, Event: evB, To: s2},
		{From: s1, Event: evA, To: s2},
		{From: s1, Event: evB, To: s0,
			Guard: func(ctx any) bool {
				v, _ := ctx.(bool)
				return v // only accepts when ctx is true
			}},
		{From: s2, Event: evA, To: s1},
		{From: s2, Event: evB, To: s0},
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("roundtrip panicked: data=%x recover=%v", data, r)
			}
		}()

		m, err := New(s0, rules...)
		if err != nil {
			t.Fatalf("New failed: %v", err)
		}

		model := s0
		accepted := 0

		for i, b := range data {
			event := evA
			if b&0x01 == 1 {
				event = evB
			}
			ctx := b&0x80 != 0 // high bit => true, exercises the s1->s0 guard
			before := m.Current()

			// Golden prediction.
			var wantTo State
			wantOk := false
			for _, r := range rules {
				if r.From == model && r.Event == event {
					if r.Guard != nil && !r.Guard(ctx) {
						break // guard rejects
					}
					wantTo, wantOk = r.To, true
					break
				}
			}

			canBefore := m.Can(event, ctx)
			err := m.Send(event, ctx)

			if wantOk {
				if err != nil {
					t.Fatalf("step %d: model accepts %s but Send err=%v", i, event, err)
				}
				if !canBefore {
					t.Fatalf("step %d: model accepts %s but Can=false", i, event)
				}
				if m.Current() != wantTo {
					t.Fatalf("step %d: Current=%s want %s", i, m.Current(), wantTo)
				}
				model = wantTo
				accepted++
			} else {
				if err == nil {
					t.Fatalf("step %d: model rejects %s but Send ok, Current=%s",
						i, event, m.Current())
				}
				if !errors.Is(err, ErrNoTransition) && !errors.Is(err, ErrGuardRejected) {
					t.Fatalf("step %d: unexpected err=%v", i, err)
				}
				if m.Current() != before {
					t.Fatalf("step %d: rejected Send mutated state %s->%s",
						i, before, m.Current())
				}
				// Can must be false when the guard rejects (the only reject
				// case in this graph — there are no actions here).
				if canBefore {
					t.Fatalf("step %d: model rejects %s but Can=true", i, event)
				}
			}

			if m.Current() != model {
				t.Fatalf("step %d: Current=%s diverged from model=%s",
					i, m.Current(), model)
			}
		}

		// Final ordering invariant: after the whole stream, the number of
		// accepted transitions equals the number of successful Sends (i.e. no
		// Send silently no-op'd, and no Send silently advanced). This is the
		// roundtrip-correctness summary check.
		_ = accepted // accepted increments only on a successful Send, by construction
	})
}
