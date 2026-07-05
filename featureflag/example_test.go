package featureflag_test

import (
	"fmt"

	"github.com/v8fg/kit4go/featureflag"
)

func ExampleNew() {
	flag := featureflag.New(
		featureflag.WithEnabled(true),
		featureflag.WithAllowlist("vip"),
	)
	fmt.Println(flag.Enabled("vip"))
	// Output: true
}
