package errcode_test

import (
	"errors"
	"fmt"

	"github.com/v8fg/kit4go/errcode"
)

func ExampleNew() {
	// A NotFound error with a human-readable message and structured details.
	e := errcode.New(errcode.NotFound, "user not found").
		WithDetail("req-123")
	fmt.Println(e.Error())
	fmt.Println(errcode.CodeOf(e))

	// Two errors with the same code compare equal under errors.Is even when
	// their messages differ, enabling stable guards across call sites.
	other := errcode.New(errcode.NotFound, "someone else")
	fmt.Println(errors.Is(e, other))

	// output:
	// errcode: not_found: user not found
	// not_found
	// true
}

func ExampleCodeOf() {
	// nil is OK.
	fmt.Println(errcode.CodeOf(nil))

	// A plain error with no code classifies as Unknown.
	fmt.Println(errcode.CodeOf(errors.New("raw db error")))

	// An *Error yields its own code...
	err := errcode.New(errcode.DeadlineExceeded, "slow query")
	fmt.Println(errcode.CodeOf(err))

	// ...and that code survives wrapping by fmt.Errorf %w.
	wrapped := fmt.Errorf("handler failed: %w", err)
	fmt.Println(errcode.CodeOf(wrapped))

	// output:
	// ok
	// unknown
	// deadline_exceeded
	// deadline_exceeded
}
