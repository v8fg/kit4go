# fsm

Generic, thread-safe finite state machine. Declarative rules (From→Event→Guard→Action→To), Send drives transitions, OnEnter/OnExit hooks, Listen observers, Can pre-check. Pure standard library.

## Usage

- `New(initial State, rules ...Rule) (*Machine, error)` — build with transition rules.
- `(*Machine).Send(event string, ctx any) error` — drive a transition.
- `(*Machine).Current() State` / `.Is(s State) bool` / `.Can(event, ctx) bool`.
- `(*Machine).OnEnter(state, fn)` / `.OnExit(state, fn)` / `.Listen(fn)`.

## Example

```go
m, _ := fsm.New("idle",
    fsm.Rule{From: "idle", Event: "submit", To: "pending"},
    fsm.Rule{From: "pending", Event: "pay", To: "paid",
        Guard: func(ctx any) bool { return ctx.(int) > 0 }},
)
m.Send("submit", nil)
m.Send("pay", 100)
m.Current() // → "paid"
```
