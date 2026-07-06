# errcode

A unified, gRPC-aligned error-code primitive for propagating structured
failures across service boundaries. A `Code` mirrors the canonical gRPC status
codes (`OK`, `NotFound`, `InvalidArgument`, ...) so a caller can classify an
error without parsing a string. An `*Error` binds a `Code` to a message and
optional structured `Details`, and participates in the standard
`errors.Is` / `errors.As` / `errors.Unwrap` chain. Pure standard library.

## Why

When failures cross a process or module boundary, a sentinel `error` plus a
free-form message loses structure: the receiver has to string-match to recover
the category. `errcode` carries the category as a first-class `Code` while
keeping full fidelity with the `errors` package, so wrapping code never drops
information and guards stay stable regardless of the per-instance message.

Two `*Error` compare equal under `errors.Is` when their codes match, enabling
`errors.Is(err, errcode.New(errcode.NotFound, ""))` style guards against any
error constructed with that code.

## API

| Symbol | Behavior |
|---|---|
| `Code` | gRPC-aligned status code enum (`OK` ... `Unauthenticated`) |
| `New(code, msg) *Error` | Build an error with no cause |
| `Wrap(code, cause, msg) *Error` | Build an error that wraps `cause` (nil cause behaves like `New`) |
| `(*Error).WithDetail(d) *Error` | Append a structured detail; fluent, chain off `New`/`Wrap` |
| `(*Error).Error() / Unwrap() / Is(target)` | `error` interface + chain traversal |
| `CodeOf(err) Code` | First code in the unwrap chain; `OK` for nil, `Unknown` for a non-`*Error` |

## Example

```go
e := errcode.New(errcode.NotFound, "user not found").
    WithDetail("req-123")
fmt.Println(e.Error())
fmt.Println(errcode.CodeOf(e))

// Same code compares equal even with a different message.
other := errcode.New(errcode.NotFound, "someone else")
fmt.Println(errors.Is(e, other))

// Code survives wrapping by fmt.Errorf %w.
wrapped := fmt.Errorf("handler failed: %w",
    errcode.New(errcode.DeadlineExceeded, "slow query"))
fmt.Println(errcode.CodeOf(wrapped))

// output:
// errcode: not_found: user not found
// not_found
// true
// deadline_exceeded
```

## Testing

Pure standard library; no external services. `errors.Is`/`As`/`Unwrap`
semantics, `WithDetail` fluency, `CodeOf` over nil / plain errors / wrapped
chains, and `Code.String` for known and out-of-range values.

```bash
go test -race -cover ./errcode/...
```
