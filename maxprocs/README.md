# maxprocs: right-size GOMAXPROCS

Right-sizes `GOMAXPROCS` to the container's CPU quota. Call `Set` explicitly,
once near main. No-op outside cgroup-limited environments (bare metal / VMs
keep the host default). This package no longer mutates global state at import.

## Usage

- `Set(log)` adjusts `GOMAXPROCS` from the cgroup CPU quota. Pass `nil` to
  apply silently, or a `Logger` (`log.Printf`, a log4go bridge, etc.) to log
  the resolved value.

```go
import (
    "log"

    "github.com/v8fg/kit4go/maxprocs"
)

func main() {
    maxprocs.Set(nil)        // silent apply
    // or: maxprocs.Set(log.Printf)
    // ...
}
```
