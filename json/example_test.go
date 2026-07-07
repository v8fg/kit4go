package json_test

import (
	"fmt"

	"github.com/v8fg/kit4go/json"
)

// ExampleMarshal demonstrates the canonical marshal/unmarshal round-trip.
// The package is a drop-in, build-tag-selectable facade over encoding/json
// (default), goccy/go-json, json-iterator, or bytedance/sonic: swap the
// import from "encoding/json" to "github.com/v8fg/kit4go/json" and the same
// API works regardless of the compiled backend.
type user struct {
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
	Age   int    `json:"age"`
}

func ExampleMarshal() {
	u := user{Name: "xwi88", Email: "xwi88@example.com", Age: 18}

	out, err := json.Marshal(u)
	if err != nil {
		fmt.Println("marshal error:", err)
		return
	}
	fmt.Println(string(out))

	var got user
	if err := json.Unmarshal(out, &got); err != nil {
		fmt.Println("unmarshal error:", err)
		return
	}
	fmt.Println(got.Name, got.Email, got.Age)

	// Output:
	// {"name":"xwi88","email":"xwi88@example.com","age":18}
	// xwi88 xwi88@example.com 18
}

// ExampleBackend demonstrates runtime introspection of the compiled backend
// (selected at build time via tags: stdlib by default, go_json / jsoniter /
// sonic otherwise). MarshalIndent and Valid share the same backend and are
// shown here since the output is stable across all four.
func ExampleBackend() {
	fmt.Println("pkg:", json.PKG)
	fmt.Println("backend:", json.Backend())

	// MarshalIndent produces deterministic, backend-independent output.
	out, err := json.MarshalIndent(user{Name: "kit", Age: 1}, "", "  ")
	if err != nil {
		fmt.Println("marshal error:", err)
		return
	}
	fmt.Println(string(out))

	// Valid reports whether a byte slice is well-formed JSON.
	fmt.Println("valid:", json.Valid([]byte(`{"name":"kit","age":1}`)))
	fmt.Println("valid:", json.Valid([]byte("not-json")))

	// Output:
	// pkg: encoding/json
	// backend: stdlib
	// {
	//   "name": "kit",
	//   "age": 1
	// }
	// valid: true
	// valid: false
}
