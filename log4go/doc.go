// Package log4go is a structured logging library with console, file and kafka writers.
//
// Origin: github.com/xwi88/log4go (originally licensed under GPLv3).
// Integrated into kit4go by the original author (xwi88), who re-licenses this
// copy under the MIT License to match kit4go's license. See the repository
// root LICENSE for the full MIT text.
//
// # Async-writer lifecycle (formerly "sharp-edges", now hardened)
//
// The async FileWriter and KafKaWriter spawn a daemon goroutine in Init, which
// Logger.Register calls. Three failure modes that used to bite high-QPS
// configurations have been fixed; the notes below describe the fix for each so
// callers know what is now safe.
//
//  1. ShardLogger + async FileWriter: ShardLogger.Register(*FileWriter) is now
//     REJECTED at n>1 (it panics with a clear message) — registering one shared
//     async FileWriter across shards spawned N daemons racing the same
//     bufio/*os.File, corrupting output under load. To fan disk writes across
//     cores use ShardLogger.RegisterFunc(func() Writer { ... }) which builds an
//     INDEPENDENT FileWriter (own daemon + bufio + file) per shard. n==1 with a
//     single Register(fw) remains supported and safe (one shard owns it).
//
//  2. spill-policy async FileWriter shutdown is now race-free. Stop sets a
//     closing flag, closes a stop signal the daemon selects on, and waits for
//     the daemon to drain all queued records + the entire spill store, flush,
//     and exit. Stop never closes the messages channel, so there is no
//     close-vs-send race and no send-on-closed panic; the daemon's drainSpill
//     short-circuits while closing so it never re-injects during shutdown.
//     drop/block/spill policies are all shutdown-safe.
//
//  3. The package singleton is now reusable across Close cycles. Close swaps
//     the singleton to nil; the next package-level call (Register / SetupLog /
//     Debug / ...) rebuilds a fresh Logger with a live bootstrap goroutine and
//     open records channel via an atomic compare-and-swap. The earlier
//     one-shot behavior (Close orphaned the singleton, leaving writer daemons
//     on a dead bootstrap) is gone. Concurrent access is safe (atomic.Pointer).
//
// The simplest correct configuration remains ONE Logger with each async writer
// registered exactly once; but multi-shard fan-out, spill recovery, and Close
// reuse are all now first-class and safe.
//
// SPDX-License-Identifier: MIT
package log4go
