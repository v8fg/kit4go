package loadbalance_test

import (
	"fmt"

	"github.com/v8fg/kit4go/loadbalance"
)

func ExampleNew() {
	// Plain round-robin over three endpoints (deterministic, no random).
	b := loadbalance.New(
		func(s string) string { return s },
		[]loadbalance.Entry[string]{
			{Value: "a:8080"},
			{Value: "b:8080"},
			{Value: "c:8080"},
		},
		loadbalance.WithStrategy[string](loadbalance.StrategyRoundRobin),
	)
	for range 4 {
		v, _ := b.Next()
		fmt.Println(v)
	}
	// Output:
	// a:8080
	// b:8080
	// c:8080
	// a:8080
}
