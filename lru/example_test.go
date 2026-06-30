package lru_test

import (
	"fmt"
	"time"

	"github.com/v8fg/kit4go/lru"
)

func ExampleNew() {
	// A 3-entry cache of bidder configs.
	c := lru.New[string, string](lru.WithMaxSize[string, string](3))
	c.Set("ssp-a", "endpoint-a")
	c.Set("ssp-b", "endpoint-b")

	v, ok := c.Get("ssp-a")
	fmt.Println(ok, v)
	// Output: true endpoint-a
}

func ExampleWithTTL() {
	// Entries expire after the TTL (lazy on access).
	c := lru.New[string, int](lru.WithTTL[string, int](50 * time.Millisecond))
	c.Set("freq-cap:u42", 3)
	_, ok := c.Get("freq-cap:u42")
	fmt.Println("hit", ok)
	// Output: hit true
}

func ExampleWithOnEvicted() {
	dropped := make([]string, 0)
	c := lru.New[string, int](
		lru.WithMaxSize[string, int](2),
		lru.WithOnEvicted[string, int](func(k string, _ int) { dropped = append(dropped, k) }),
	)
	c.Set("a", 1)
	c.Set("b", 2)
	c.Set("c", 3) // evicts "a"
	fmt.Println(dropped)
	// Output: [a]
}
