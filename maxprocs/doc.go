// Package maxprocs right-sizes GOMAXPROCS to the container's CPU quota.
// Call Set explicitly, once near main:
//
//	import "github.com/v8fg/kit4go/maxprocs"
//
//	func main() {
//	    maxprocs.Set(nil) // or maxprocs.Set(log.Printf)
//	}
//
// This package does not mutate global state at import — opt in by calling Set.
// # When you need this
//
// Go 1.25+ already sets GOMAXPROCS from the cgroup CPU quota, so on Go 1.26 this
// package is usually a no-op safety net. It matters for:
//
//   - Go < 1.25, where GOMAXPROCS defaults to the host CPU count inside a
//     container (a 4-CPU k8s pod on a 64-core node would run 64 Go threads).
//   - Environments where the cgroup quota is not detected (some runtimes, very
//     old kernels, certain VMs).
//
// Right-sizing GOMAXPROCS matters for log4go's AutoShardCount, which derives the
// shard count from it — an over-reported GOMAXPROCS would over-shard (capped at
// 8 by AutoShardCount, so the damage is bounded, but still wasteful).
//
// # Why a subpackage (not inside log4go)
//
// Setting GOMAXPROCS is a process-global, application-level decision with side
// effects on the entire runtime. A library (log4go) must NOT impose it on every
// importer. Keeping it in its own package means only the application opt-in by
// importing it, and any other kit4go consumer can reuse the same import.
package maxprocs
