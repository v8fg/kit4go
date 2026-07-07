package trie_test

import (
	"fmt"

	"github.com/v8fg/kit4go/trie"
)

// ExampleTrie demonstrates exact lookup and longest-prefix match.
// Keys are segmented by "/"; LongestPrefix returns the value of the
// deepest configured key that prefixes the query.
func ExampleTrie() {
	t := trie.New[int]()
	t.Insert("/api", 1)
	t.Insert("/api/v1", 2)
	t.Insert("/api/v1/users", 3)

	v, key, ok := t.LongestPrefix("/api/v1/users/42")
	fmt.Println(key, v, ok)
	// Output: api/v1/users 3 true
}

// ExampleTrie_keysWithPrefix lists every configured key under a prefix in
// sorted order. Keys are stored without their surrounding "/" (segments are
// split on "/" and rejoined).
func ExampleTrie_keysWithPrefix() {
	t := trie.New[string]()
	t.Insert("/api/v1/users", "u")
	t.Insert("/api/v1/orgs", "o")
	t.Insert("/api/v2/items", "i")

	for _, k := range t.KeysWithPrefix("/api/v1") {
		fmt.Println(k)
	}
	// Output:
	// api/v1/orgs
	// api/v1/users
}
