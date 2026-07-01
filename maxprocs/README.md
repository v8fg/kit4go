# maxprocs: right-size GOMAXPROCS

Right-sizes `GOMAXPROCS` to the container's CPU quota at startup. Call `Set()`
once near main; it logs the resolved value. No-op outside cgroup-limited
environments (bare metal / VMs keep the host default).

## Usage

- `Set()` adjusts `GOMAXPROCS` from the cgroup CPU quota and logs the change.

```go
import "github.com/v8fg/kit4go/maxprocs"

func main() {
    maxprocs.Set()
    // ...
}
```
