package log4go

import (
	"os"
	"runtime"
	"sync"
	"testing"
)

// This file holds the regression test for Bug 2 (spill-policy async FileWriter
// shutdown race: send-on-closed / use-after-close when the spiller is non-empty
// at Stop). Bug 1's regression test lives in shard_logger_test.go (RegisterFunc);
// Bug 3's lives in singleton_reuse_test.go.

// Test_FileWriter_Async_SpillOverflowRecovery exercises the spill overflow path
// AND shutdown with a non-empty spill store: a tiny async buffer + a large burst
// forces records into the ring spiller, then Stop() drains the channel and the
// spiller while the flush ticker is still armed. Before the fix Stop closed the
// messages channel and the daemon's drainSpill (driven by the ticker) could send
// on it after the close, racing shutdown (close-vs-send data race +
// send-on-closed panic). Run under -race -count=5 to catch flakiness.
func Test_FileWriter_Async_SpillOverflowRecovery(t *testing.T) {
	spillDir := t.TempDir()
	for iter := 0; iter < 5; iter++ {
		fw, dir := newAsyncFileWriter(t, FileWriterOptions{
			AsyncBufferSize: 4, // deliberately tiny -> overflows into the spiller
			OverflowPolicy:  "spill",
			SpillType:       "ring",
			SpillSize:       4096,
			SpillDir:        spillDir,
			SpillMaxBytes:   1 << 20,
		})
		if err := fw.Init(); err != nil {
			os.RemoveAll(dir)
			t.Fatalf("iter %d Init: %v", iter, err)
		}

		// Large burst from multiple producers overwhelms the tiny channel and
		// forces the spill path; the spiller is very likely non-empty when we
		// call Stop immediately after.
		const perWorker = 5000
		const workers = 4
		var wg sync.WaitGroup
		for w := 0; w < workers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < perWorker; i++ {
					_ = fw.Write(&Record{level: INFO, time: "t", file: "f", msg: "spill stress"})
				}
			}()
		}
		wg.Wait()
		// Do NOT sleep — we want the spiller hot at Stop to exercise the race.
		fw.Stop()
		os.RemoveAll(dir)
	}
	// If we reached here 5x without -race reporting a close-vs-send race or a
	// send-on-closed panic, the shutdown ordering is stable.
	_ = runtime.NumGoroutine
}
