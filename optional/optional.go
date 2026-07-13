// Package optional provides a generic Option[T] type — a value that may or may
// not be present — without the ambiguity of nil pointers. An Option is either
// Some(value) or None, and the caller must explicitly Unwrap to access the value.
//
// This eliminates the "is this nil pointer a valid zero or an absence?" question
// that plagues pointer-based optionality in Go.
//
// Pure standard library.
package optional

// Option represents an optional value of type T.
type Option[T any] struct {
	val T
	ok  bool
}

// Some wraps a value in an Option.
func Some[T any](v T) Option[T] { return Option[T]{val: v, ok: true} }

// None returns an empty Option.
func None[T any]() Option[T] { return Option[T]{} }

// FromPtr builds an Option from a pointer: nil → None, non-nil → Some(*p).
func FromPtr[T any](p *T) Option[T] {
	if p == nil {
		return None[T]()
	}
	return Some(*p)
}

// IsSome reports whether the Option holds a value.
func (o Option[T]) IsSome() bool { return o.ok }

// IsNone reports whether the Option is empty.
func (o Option[T]) IsNone() bool { return !o.ok }

// Get returns the value and a bool (true if present). The zero value of T is
// returned when the Option is None.
func (o Option[T]) Get() (T, bool) { return o.val, o.ok }

// Unwrap returns the value, panicking if the Option is None.
func (o Option[T]) Unwrap() T {
	if !o.ok {
		panic("optional: Unwrap on None")
	}
	return o.val
}

// UnwrapOr returns the value if present, or the given fallback.
func (o Option[T]) UnwrapOr(fallback T) T {
	if o.ok {
		return o.val
	}
	return fallback
}

// UnwrapOrElse returns the value if present, or calls fn to produce a fallback.
func (o Option[T]) UnwrapOrElse(fn func() T) T {
	if o.ok {
		return o.val
	}
	return fn()
}

// UnwrapOrZero returns the value if present, or the zero value of T.
func (o Option[T]) UnwrapOrZero() T {
	var zero T
	return o.UnwrapOr(zero)
}

// ToPtr returns a pointer to the value, or nil if None. Inverse of FromPtr.
func (o Option[T]) ToPtr() *T {
	if !o.ok {
		return nil
	}
	v := o.val
	return &v
}

// Map transforms the value with fn if present; None passes through.
func Map[T, U any](o Option[T], fn func(T) U) Option[U] {
	if !o.ok {
		return None[U]()
	}
	return Some(fn(o.val))
}

// MapOr transforms the value if present, or returns the fallback.
func MapOr[T, U any](o Option[T], fallback U, fn func(T) U) U {
	if !o.ok {
		return fallback
	}
	return fn(o.val)
}

// AndThen (flat-map) transforms the value with fn if present; None passes through.
func AndThen[T, U any](o Option[T], fn func(T) Option[U]) Option[U] {
	if !o.ok {
		return None[U]()
	}
	return fn(o.val)
}

// Equal compares two Options by their values (requires a comparison fn because
// T is `any`, not `comparable`).
func Equal[T any](a, b Option[T], eq func(T, T) bool) bool {
	if a.ok != b.ok {
		return false
	}
	if !a.ok {
		return true // both None
	}
	return eq(a.val, b.val)
}
