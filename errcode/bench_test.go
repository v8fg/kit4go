package errcode

import (
	"errors"
	"fmt"
	"testing"
)

// The benchmarks below cover the hot paths of the package:
//
//   - New / Wrap construction (allocation cost of an *Error)
//   - Error() string formatting (plain and cause-bearing)
//   - errors.Is same-code matching (the sentinel guard idiom)
//   - errors.Is traversal through a Wrap chain
//   - CodeOf classification (nil fast path, *Error direct, wrapped chain)
//   - WithDetail fluent chaining
//   - Code.String() table lookup
//
// Each benchmark reports ns/op + B/op + allocs/op so allocation regressions in
// the construction and formatting paths are visible at a glance.

// BenchmarkNew measures the cost of building a fresh *Error via New. This is
// the most frequent allocation in the package and the baseline for Wrap.
func BenchmarkNew(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		_ = New(NotFound, "user missing")
	}
}

// BenchmarkWrap measures the cost of building an *Error that carries a cause.
// It differs from New only by the extra field assignment, so any divergence
// signals an unexpected allocation change.
func BenchmarkWrap(b *testing.B) {
	cause := errors.New("root cause")
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = Wrap(Internal, cause, "db down")
	}
}

// BenchmarkErrorString measures Error() on a plain (no cause) *Error. The
// single fmt.Sprintf is the only allocation source; this guards formatting
// regressions on the common path.
func BenchmarkErrorString(b *testing.B) {
	e := New(NotFound, "user missing")
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = e.Error()
	}
}

// BenchmarkErrorStringWrapped measures Error() on a cause-bearing *Error. The
// extra "%s: %s" segment exercises the wrapped formatting branch.
func BenchmarkErrorStringWrapped(b *testing.B) {
	e := Wrap(Internal, errors.New("root cause"), "db down")
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = e.Error()
	}
}

// BenchmarkErrorsIs_SameCode measures the sentinel-guard idiom: matching a
// fresh instance against a package-level sentinel by code equality. This is
// the hottest comparison path in cross-service error classification.
func BenchmarkErrorsIs_SameCode(b *testing.B) {
	err := New(NotFound, "user missing")
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if !errors.Is(err, ErrNotFound) {
			b.Fatal("errors.Is = false, want true")
		}
	}
}

// BenchmarkErrorsIs_DifferentCode measures the negative arm of the sentinel
// guard: a non-matching code short-circuits to false.
func BenchmarkErrorsIs_DifferentCode(b *testing.B) {
	err := New(Internal, "boom")
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if errors.Is(err, ErrNotFound) {
			b.Fatal("errors.Is = true, want false")
		}
	}
}

// BenchmarkErrorsIs_ThroughWrapChain measures errors.Is when the target code
// sits behind a fmt.Errorf %w wrapper, exercising the Unwrap traversal.
func BenchmarkErrorsIs_ThroughWrapChain(b *testing.B) {
	inner := New(PermissionDenied, "no access")
	wrapped := fmt.Errorf("handler: %w", inner)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if !errors.Is(wrapped, ErrPermissionDenied) {
			b.Fatal("errors.Is = false, want true")
		}
	}
}

// BenchmarkCodeOf_Nil measures the fast path of CodeOf: a nil error returns OK
// without any allocation.
func BenchmarkCodeOf_Nil(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		if got := CodeOf(nil); got != OK {
			b.Fatalf("CodeOf(nil) = %v, want OK", got)
		}
	}
}

// BenchmarkCodeOf_DirectError measures CodeOf on a bare *Error, where
// errors.As matches on the first hop.
func BenchmarkCodeOf_DirectError(b *testing.B) {
	err := New(AlreadyExists, "dup")
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if got := CodeOf(err); got != AlreadyExists {
			b.Fatalf("CodeOf = %v, want AlreadyExists", got)
		}
	}
}

// BenchmarkCodeOf_Wrapped measures CodeOf when the *Error sits behind a
// fmt.Errorf %w wrapper, so errors.As must walk the chain.
func BenchmarkCodeOf_Wrapped(b *testing.B) {
	err := fmt.Errorf("outer: %w", Wrap(ResourceExhausted, nil, "quota"))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if got := CodeOf(err); got != ResourceExhausted {
			b.Fatalf("CodeOf = %v, want ResourceExhausted", got)
		}
	}
}

// BenchmarkCodeOf_PlainError measures CodeOf on a non-*Error error: errors.As
// fails to find an *Error and the function returns Unknown.
func BenchmarkCodeOf_PlainError(b *testing.B) {
	err := errors.New("untyped")
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if got := CodeOf(err); got != Unknown {
			b.Fatalf("CodeOf = %v, want Unknown", got)
		}
	}
}

// BenchmarkWithDetail measures the fluent builder chain appending three
// details. The append growth of the Details slice is the only allocation.
func BenchmarkWithDetail(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		_ = New(DeadlineExceeded, "slow").
			WithDetail("request-id").
			WithDetail(42).
			WithDetail(struct{ K int }{K: 7})
	}
}

// BenchmarkCodeString measures the codeNames table lookup, the cheapest
// operation in the package; it establishes a floor for the rest.
func BenchmarkCodeString(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		if s := NotFound.String(); s == "" {
			b.Fatal("empty code name")
		}
	}
}

// BenchmarkCodeString_OutOfRange measures the out-of-range fallback branch of
// String(), which returns "unknown" without indexing the table.
func BenchmarkCodeString_OutOfRange(b *testing.B) {
	c := Code(-1)
	b.ReportAllocs()
	for range b.N {
		if s := c.String(); s == "" {
			b.Fatal("empty code name")
		}
	}
}
