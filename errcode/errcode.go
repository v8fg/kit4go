// Package errcode provides a unified, gRPC-aligned error-code primitive for
// propagating structured failures across service boundaries.
//
// A [Code] mirrors the gRPC canonical status codes (OK, NotFound,
// InvalidArgument, ...), so a non-HTTP/gRPC caller can still classify an
// error without parsing a string. An [*Error] binds a Code to a human-readable
// message and optional structured Details, and participates in the standard
// errors.Is / errors.As / errors.Unwrap chain so wrapping code keeps full
// fidelity.
//
// Two [*Error] compare equal under errors.Is when their codes match — this
// lets callers write errors.Is(err, errcode.NotFound) style guards against
// any error constructed with that code, regardless of the per-instance message.
//
// The package uses only the standard library.
package errcode

import (
	"errors"
	"fmt"
)

// Code is a gRPC-aligned canonical status code.
type Code int

// Canonical codes, ordered and numbered to match google.golang.org/grpc/status
// codes. They are the lingua franca for cross-service error classification.
const (
	OK                 Code = 0
	Canceled           Code = 1
	Unknown            Code = 2
	InvalidArgument    Code = 3
	DeadlineExceeded   Code = 4
	NotFound           Code = 5
	AlreadyExists      Code = 6
	PermissionDenied   Code = 7
	ResourceExhausted  Code = 8
	FailedPrecondition Code = 9
	Aborted            Code = 10
	OutOfRange         Code = 11
	Unimplemented      Code = 12
	Internal           Code = 13
	Unavailable        Code = 14
	DataLoss           Code = 15
	Unauthenticated    Code = 16
)

var codeNames = [...]string{
	OK:                 "ok",
	Canceled:           "canceled",
	Unknown:            "unknown",
	InvalidArgument:    "invalid_argument",
	DeadlineExceeded:   "deadline_exceeded",
	NotFound:           "not_found",
	AlreadyExists:      "already_exists",
	PermissionDenied:   "permission_denied",
	ResourceExhausted:  "resource_exhausted",
	FailedPrecondition: "failed_precondition",
	Aborted:            "aborted",
	OutOfRange:         "out_of_range",
	Unimplemented:      "unimplemented",
	Internal:           "internal",
	Unavailable:        "unavailable",
	DataLoss:           "data_loss",
	Unauthenticated:    "unauthenticated",
}

// String returns the human-readable name of the code.
// Unrecognized codes report "unknown" so a foreign value never panics.
func (c Code) String() string {
	if int(c) < 0 || int(c) >= len(codeNames) {
		return "unknown"
	}
	return codeNames[c]
}

// Error carries a canonical [Code], a message, and optional structured details.
// It implements the error interface and participates in errors.Is / errors.As
// / errors.Unwrap chains.
type Error struct {
	Code    Code
	Message string
	Details []any

	// cause is the wrapped error, if any. Kept unexported so callers build
	// instances via New / Wrap and read only the public fields.
	cause error
}

// Error returns a "errcode: <code>: <message>" form. When the Error was built
// with Wrap, the wrapped error's text is appended after the message so the
// root cause remains visible in a single line.
func (e *Error) Error() string {
	if e == nil {
		return "errcode: ok"
	}
	if e.cause != nil {
		return fmt.Sprintf("errcode: %s: %s: %s", e.Code, e.Message, e.cause)
	}
	return fmt.Sprintf("errcode: %s: %s", e.Code, e.Message)
}

// Unwrap returns the wrapped cause, or nil when the Error was built with New.
// It enables errors.Is and errors.As to traverse to the underlying error.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// Is reports whether the current error matches target. Two [*Error] are
// considered equal when their codes match, allowing
// errors.Is(err, errcode.New(errcode.NotFound, "")) style guards. For any
// other target type, standard identity comparison applies.
func (e *Error) Is(target error) bool {
	if e == nil {
		return target == nil
	}
	var t *Error
	if errors.As(target, &t) {
		if t == nil { // typed-nil *Error target: errors.Is(err, (*Error)(nil))
			return false
		}
		return e.Code == t.Code
	}
	return false
}

// WithDetail appends a structured detail to the Error and returns the same
// Error, enabling fluent construction: New(...).WithDetail(a).WithDetail(b).
// It must be called on a non-nil *Error, i.e. chained off New or Wrap.
func (e *Error) WithDetail(d any) *Error {
	e.Details = append(e.Details, d)
	return e
}

// New returns an [*Error] with the given code and message and no cause.
func New(code Code, msg string) *Error {
	return &Error{Code: code, Message: msg}
}

// Wrap returns an [*Error] with the given code and msg that wraps cause so
// errors.Is / errors.As can reach it. If cause is nil the Error behaves like
// one from New (Unwrap returns nil).
func Wrap(code Code, cause error, msg string) *Error {
	return &Error{Code: code, Message: msg, cause: cause}
}

// CodeOf walks the errors.Unwrap chain of err and returns the [Code] of the
// first [*Error] it meets. It returns OK when err is nil and Unknown for a
// non-nil error that carries no [*Error] in its chain.
func CodeOf(err error) Code {
	if err == nil {
		return OK
	}
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return Unknown
}
