// Package log4go is a structured logging library with console, file and kafka writers.
//
// Origin: github.com/xwi88/log4go (originally licensed under GPLv3).
// Integrated into kit4go by the original author (xwi88), who re-licenses this
// copy under the MIT License to match kit4go's license. See the repository
// root LICENSE for the full MIT text.
//
// # Lifecycle sharp-edges (read before wiring async writers)
//
// The async FileWriter and KafKaWriter spawn a daemon goroutine in Init, which
// Logger.Register calls. Three sharp-edges follow from this; the simplest
// correct configuration is ONE Logger (the package singleton or a single-shard
// ShardLogger) with each async writer registered exactly once.
//
//  1. Do NOT register one async FileWriter across multiple shards via
//     ShardLogger(n>1).Register — each shard's Register calls Init, spawning n
//     daemons that race the same bufio / *os.File and corrupt output under load.
//     To fan out disk writes across cores, build n single-shard loggers each
//     with its own FileWriter.
//
//  2. The spill-policy async FileWriter can race its own shutdown: Stop closes
//     the messages channel, but the daemon's drainSpill (driven by the flush
//     ticker) may send on it concurrently. The drop policy has no spill store
//     and is shutdown-safe; prefer it unless spill recovery is required, and
//     when using spill ensure the store has drained before Stop.
//
//  3. The package singleton is one-shot: Close terminates its bootstrap
//     goroutine permanently and re-registering writers afterwards (or calling
//     SetupLog a second time) leaves orphaned daemons on it. Configure once at
//     startup; for per-test or multi-run isolation, use NewShardLogger(1)
//     instead of the singleton.
//
// These are documented for callers; the singleton + single-writer path used by
// most applications is unaffected.
//
// SPDX-License-Identifier: MIT
package log4go
