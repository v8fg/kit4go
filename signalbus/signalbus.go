// Package signalbus implements a string-keyed synchronous pub/sub event bus
// (Django-style signal): Connect subscribes a handler to a named signal, Send
// dispatches an argument list to every handler registered for that name. Pure
// standard library.
//
// Dispatch model: Send runs SYNCHRONOUSLY on the caller's goroutine — each
// registered handler is invoked in registration order, one after another, in
// the same call stack that invoked Send. There are no goroutines and no
// channels: a Send returns only after every handler has returned. Handlers MUST
// be non-blocking (no network waits, no long locks); a slow handler stalls the
// caller and every handler after it.
//
// A panicking handler is recovered so it cannot abort the dispatch of the
// remaining handlers. Recovered panics are counted (Recovered) and optionally
// surfaced via a hook (SetPanicHook) — but never re-panicked, so a buggy
// handler cannot take down the process.
//
// Decoupling: the producer (Send) and the consumers (Connect) share only the
// signal name and the argument contract, never types. This is used in the local
// ad-tech stack to let a low-level package emit a signal ("creative cached",
// "bid lost") that higher-level packages subscribe to, without the low-level
// package importing them — breaking what would otherwise be a circular import.
package signalbus

import (
	"sync"
	"sync/atomic"
)

// Handler is a signal subscriber. It receives the variadic argument list passed
// to Send. Handlers must be non-blocking (see package doc).
type Handler func(args ...any)

// Bus is a string-keyed synchronous pub/sub event bus.
//
// Concurrency: safe for concurrent use. Connect/Send/Disconnect/Len all
// serialise map access via an internal sync.Mutex; Send copies the handler slice
// under the lock and dispatches OUTSIDE it, so a handler may re-entrantly call
// Connect/Disconnect/Send without deadlocking. The zero value is NOT usable —
// use New.
type Bus struct {
	mu        sync.Mutex
	subs      map[string][]entry
	nextID    uint64 // monotonically increasing handler id (precise disconnect)
	recovered atomic.Uint64
	hook      func(name string, handlerID uint64, r any) // optional, fired on a recovered handler panic
}

// entry pairs a Handler with a unique id so a disconnect func removes exactly
// one subscription, not every handler that happens to share the same func value.
type entry struct {
	id uint64
	h  Handler
}

// New builds an empty Bus.
func New() *Bus {
	return &Bus{subs: make(map[string][]entry)}
}

// Connect subscribes h to the named signal and returns a disconnect func. The
// returned func removes exactly that one subscription and is idempotent (safe
// to call multiple times, or never). Send invokes handlers in the order Connect
// was called. A nil h is ignored (returns a no-op disconnect).
func (b *Bus) Connect(name string, h Handler) (disconnect func()) {
	if h == nil {
		return func() {}
	}
	b.mu.Lock()
	b.nextID++
	id := b.nextID
	b.subs[name] = append(b.subs[name], entry{id: id, h: h})
	b.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() { b.remove(name, id) })
	}
}

// remove deletes the entry with the given id from name's slice, preserving the
// order of the remaining handlers. Caller-path: under b.mu (Connect's disconnect
// func) or via Disconnect.
func (b *Bus) remove(name string, id uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	subs := b.subs[name]
	for i, e := range subs {
		if e.id == id {
			// Compact in place to preserve registration order of the survivors.
			copy(subs[i:], subs[i+1:])
			subs[len(subs)-1] = entry{}
			b.subs[name] = subs[:len(subs)-1]
			if len(b.subs[name]) == 0 {
				delete(b.subs, name)
			}
			return
		}
	}
}

// Send dispatches args to every handler registered for name, in registration
// order, synchronously on the caller's goroutine. A signal with no subscribers
// is a no-op. A panicking handler is recovered (counted in Recovered, surfaced
// via the SetPanicHook if set) and the dispatch continues with the next handler.
//
// The handler slice is snapshotted under the lock and dispatched WITHOUT it, so
// a handler may re-entrantly Connect/Disconnect/Send on the same Bus without
// deadlocking. Handlers added or removed during the dispatch are not visible to
// the in-flight Send.
func (b *Bus) Send(name string, args ...any) {
	// Snapshot the handlers under the lock, dispatch outside it. This keeps the
	// critical section tiny and lets handlers re-enter the Bus (a handler that
	// Connects, Disconnects, or Sends will not self-deadlock).
	b.mu.Lock()
	subs := b.subs[name]
	snapshot := make([]entry, len(subs))
	copy(snapshot, subs)
	b.mu.Unlock()

	for _, e := range snapshot {
		b.invoke(name, e, args)
	}
}

// invoke calls one handler with panic recovery. A panic is counted and surfaced
// via the hook (if set) but never re-panicked, so one buggy handler cannot abort
// the dispatch or crash the process.
func (b *Bus) invoke(name string, e entry, args []any) {
	defer func() {
		if r := recover(); r != nil {
			b.recovered.Add(1)
			if b.hook != nil {
				b.hook(name, e.id, r)
			}
		}
	}()
	e.h(args...)
}

// Disconnect removes ALL handlers registered for name. It is a no-op if name has
// no subscribers.
func (b *Bus) Disconnect(name string) {
	b.mu.Lock()
	delete(b.subs, name)
	b.mu.Unlock()
}

// Len returns the number of handlers currently registered for name. Intended for
// tests and debug; do not use for dispatch decisions (the count can change
// between a Len call and a subsequent Send).
func (b *Bus) Len(name string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subs[name])
}

// SetPanicHook installs a hook fired after a handler panics and is recovered.
// The hook receives the signal name, the panicking handler's id (as assigned by
// Connect), and the recovered value. The hook runs on the Send caller's
// goroutine, so it must be non-blocking. Set to nil to disable.
func (b *Bus) SetPanicHook(hook func(name string, handlerID uint64, r any)) {
	b.mu.Lock()
	b.hook = hook
	b.mu.Unlock()
}

// Recovered returns the total number of handler panics recovered across all
// signals since the Bus was created.
func (b *Bus) Recovered() uint64 {
	return b.recovered.Load()
}
