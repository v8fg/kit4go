package config_test

import (
	"fmt"
	"time"

	"github.com/v8fg/kit4go/config"
)

func Example_layered() {
	// Typically env (highest) overrides a JSON file, which overrides defaults.
	store := config.New(
		config.MapSource{"bidder.timeout": "1s"}, // would-be file source
		config.MapSource{"bidder.timeout": "500ms", "bidder.qps": "100000"},
	)
	fmt.Println(store.Duration("bidder.timeout", 0)) // first source that has the key wins
	fmt.Println(store.Int("bidder.qps", 0))          // only the 2nd source has it
	// Output:
	// 1s
	// 100000
}

func ExampleEnv() {
	// (In real code these come from the process environment.)
	// config.Env("app").Lookup("redis.addr") -> os.Getenv("APP_REDIS_ADDR")
	store := config.New(config.Env("app"))
	fmt.Println(store.String("redis.addr", "unset"))
	// Output: unset
}

func ExampleStore_Bool() {
	store := config.New(config.MapSource{"feature.fast_path": "on"})
	fmt.Println(store.Bool("feature.fast_path", false))
	// Output: true
}

func ExampleStore_Duration() {
	store := config.New(config.MapSource{"timeout": "250ms"})
	fmt.Println(store.Duration("timeout", time.Second))
	// Output: 250ms
}
