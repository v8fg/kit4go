package errcode

import (
	"errors"
	"testing"
)

// FuzzCodeOf asserts the round-trip invariant for [New] and [CodeOf]: for any
// code value, an *Error built with New(code, msg) must classify back to the
// same code via CodeOf. This guards the codeNames-independent code field
// plumbing and the errors.As traversal inside CodeOf. The fuzzer is free to
// explore the full int range of Code, including values outside the declared
// canonical set (0..16); for those, only the round trip — not String() — is
// asserted, so no branch panics.
func FuzzCodeOf(f *testing.F) {
	// Seeds: the boundary codes plus a few representative interior values.
	f.Add(int32(OK))              // 0
	f.Add(int32(Unauthenticated)) // 16
	f.Add(int32(NotFound))        // mid-range canonical
	f.Add(int32(Internal))        // mid-range canonical
	f.Add(int32(-1))              // negative out-of-range
	f.Add(int32(len(codeNames)))  // upper out-of-range (== 17)
	f.Add(int32(1 << 20))         // large positive
	f.Add(int32(-(1 << 20)))      // large negative

	f.Fuzz(func(t *testing.T, code int32) {
		c := Code(code)

		// New must not panic for any code value.
		err := New(c, "fuzz message")
		if err == nil {
			t.Fatal("New returned nil *Error")
		}
		if err.Code != c {
			t.Fatalf("err.Code = %v, want %v", err.Code, c)
		}

		// Round trip: CodeOf must recover the exact code from the bare *Error.
		if got := CodeOf(err); got != c {
			t.Fatalf("CodeOf(err) = %v, want %v", got, c)
		}

		// The code must also survive wrapping by a %w error: CodeOf walks the
		// chain via errors.As, so the same code must surface through the wrap.
		wrapped := wrapOnce(err)
		if got := CodeOf(wrapped); got != c {
			t.Fatalf("CodeOf(wrapped err) = %v, want %v", got, c)
		}
	})
}

// FuzzIsByCode asserts the same-code equality rule of (*Error).Is: any two
// *Error instances carrying the same Code compare equal under errors.Is,
// regardless of their messages. This is the property the package-level
// sentinels rely on, so fuzzing both code and both messages hardens it against
// message content. The negative arm is also covered: differing codes must NOT
// be equal.
func FuzzIsByCode(f *testing.F) {
	// Seeds: (code, msgA, msgB). Each seed builds two same-code instances with
	// differing messages, so equality is the property under test; the negative
	// arm inside the body derives a distinct code by XOR.
	f.Add(int32(NotFound), "user missing", "another user missing")
	f.Add(int32(Internal), "", "")
	f.Add(int32(PermissionDenied), "a", "b")
	f.Add(int32(-5), "out-of-range code", "still equal")
	f.Add(int32(OK), "ok-a", "ok-b")
	// Negative seed: different codes -> not equal.
	f.Add(int32(NotFound), "x", "y")
	f.Add(int32(NotFound), "", "")

	f.Fuzz(func(t *testing.T, codeA int32, msgA, msgB string) {
		a := New(Code(codeA), msgA)
		b := New(Code(codeA), msgB)

		// Same code -> errors.Is true both directions, irrespective of messages.
		if !errors.Is(a, b) {
			t.Fatalf("errors.Is(a, b) = false for code %v, want true (same code)", codeA)
		}
		if !errors.Is(b, a) {
			t.Fatalf("errors.Is(b, a) = false for code %v, want true (same code)", codeA)
		}

		// Same code also matches through a Wrap chain on either side.
		wrapped := wrapOnce(a)
		if !errors.Is(wrapped, b) {
			t.Fatalf("errors.Is(wrapped a, b) = false for code %v, want true", codeA)
		}
		if !errors.Is(b, wrapped) {
			t.Fatalf("errors.Is(b, wrapped a) = false for code %v, want true", codeA)
		}

		// Negative arm: a distinct code must not match. Flipping the low bit
		// (codeA^1) is guaranteed != codeA for every int32, so the negative
		// holds by construction without a special case.
		neg := New(Code(codeA)^1, msgB)
		if errors.Is(a, neg) {
			t.Fatalf("errors.Is(a, neg) = true for codes %v vs %v, want false",
				codeA, Code(codeA)^1)
		}
	})
}

// wrapOnce wraps err once with a %w verb, mirroring the typical
// fmt.Errorf("...: %w", err) pattern that CodeOf / errors.Is must traverse. It
// is kept in one place so both fuzzers exercise the same wrapping shape.
func wrapOnce(err error) error {
	return errWrapHelper{err: err}
}

// errWrapHelper is a minimal error-with-Unwrap type used only by the fuzz
// tests. It avoids pulling fmt into the hot fuzz body and gives a deterministic
// single-hop chain that errors.As / errors.Unwrap walk exactly.
type errWrapHelper struct{ err error }

func (e errWrapHelper) Error() string { return "fuzz wrap" }
func (e errWrapHelper) Unwrap() error { return e.err }
