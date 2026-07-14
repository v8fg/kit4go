package memoize_test

import (
	"fmt"

	"github.com/v8fg/kit4go/memoize"
)

func fib(n int) int {
	if n < 2 {
		return n
	}
	return fib(n-1) + fib(n-2)
}

// ExampleMemoize turns an O(2^n) recursive Fibonacci into an O(n) memoized one
// — the second call with the same argument is a cache hit.
func ExampleMemoize() {
	calls := 0
	// Wrap a pure function; the wrapper caches by argument.
	mfib := memoize.Memoize(func(n int) int {
		calls++
		return fib(n)
	})

	fmt.Println(mfib(30))
	fmt.Println(mfib(30)) // cached — calls does not increment
	fmt.Println("calls:", calls)
	// Output:
	// 832040
	// 832040
	// calls: 1
}

// ExampleMemoizeErr caches only successes; an error is not cached and retries.
func ExampleMemoizeErr() {
	parse := memoize.MemoizeErr(func(s string) (int, error) {
		if s == "" {
			return 0, fmt.Errorf("empty")
		}
		return len(s), nil
	})

	v, err := parse("hello")
	fmt.Println(v, err)

	_, err = parse("") // error, not cached
	fmt.Println("err:", err != nil)
	// Output:
	// 5 <nil>
	// err: true
}
