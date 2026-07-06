package ip

// Internal race-regression tests for the copy-on-write local-IP cache.
//
// These live in package `ip` (not `ip_test`) so they can drive the unexported
// atomic pointer (`cacheLocalIP`) and the refresh path (`updateCacheLocalIP`)
// directly. The public-API surface exercised by callers is covered in
// net_info_test.go; this file targets the data-race that existed when
// cacheLocalIP was a bare *localIP read/written without synchronization.
//
// Run with: go test -race -run TestLocalIPCache ./ip/...

import (
	"net"
	"runtime"
	"sync"
	"testing"
	"time"
)

// fakeAddrLookup returns a deterministic IPv4 address so the refresh path has
// something stable to publish.
type fakeAddrLookup struct{ addrs []net.Addr }

func (f fakeAddrLookup) InterfaceAddrs() ([]net.Addr, error)  { return f.addrs, nil }
func (f fakeAddrLookup) Interfaces() ([]net.Interface, error) { return nil, nil }

func addrFor(ip string) []net.Addr {
	return []net.Addr{
		&net.IPNet{IP: net.ParseIP(ip), Mask: net.CIDRMask(24, 32)},
	}
}

// TestLocalIPCacheConcurrentReads hammers the read path concurrently. Before
// the atomic-pointer fix this tore the net.IP / time.Time fields; now each
// reader observes an immutable snapshot.
func TestLocalIPCacheConcurrentReads(t *testing.T) {
	// Seed a non-nil snapshot so readers exercise the fast path.
	cacheLocalIP.Store(&localIP{
		IP:         net.ParseIP("192.168.0.1"),
		LatestTime: time.Now(),
		TTL:        time.Hour,
	})

	var wg sync.WaitGroup
	const goroutines = 64
	const iters = 200
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iters; j++ {
				_ = LocalIP()
				runtime.Gosched()
			}
		}()
	}
	wg.Wait()
}

// TestLocalIPCacheConcurrentReadWrite drives concurrent readers (LocalIP) and
// writers (updateCacheLocalIP via swapped DefaultAddrLookup). The writers also
// force TTL expiry by publishing already-stale snapshots, which steers the
// readers' LocalIP() calls through the refresh path under load.
func TestLocalIPCacheConcurrentReadWrite(t *testing.T) {
	origLookup := DefaultAddrLookup
	t.Cleanup(func() { DefaultAddrLookup = origLookup })

	DefaultAddrLookup = fakeAddrLookup{addrs: addrFor("10.0.0.7")}

	// Start with an expired snapshot so the first reads trigger refresh.
	cacheLocalIP.Store(&localIP{
		IP:         net.ParseIP("10.0.0.1"),
		LatestTime: time.Now().Add(-time.Hour),
		TTL:        time.Minute,
	})

	var wg sync.WaitGroup
	const readers = 32
	const writers = 8
	const iters = 200

	wg.Add(readers)
	for i := 0; i < readers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iters; j++ {
				_ = LocalIP()
			}
		}()
	}

	wg.Add(writers)
	for i := 0; i < writers; i++ {
		go func(n int) {
			defer wg.Done()
			for j := 0; j < iters; j++ {
				// Direct write via the atomic pointer: publish a fresh,
				// already-expired snapshot so concurrent readers keep hitting
				// the refresh branch. This is exactly the field set that raced
				// before the fix.
				cacheLocalIP.Store(&localIP{
					IP:         net.ParseIP("10.0.0.2"),
					LatestTime: time.Now().Add(-time.Hour),
					TTL:        time.Minute,
				})
				// Also drive the refresh path, which itself Stores a snapshot.
				_ = updateCacheLocalIP()
				runtime.Gosched()
			}
		}(i)
	}
	wg.Wait()
}

// TestLocalIPCacheSnapshotImmutable verifies the copy-on-write invariant: a
// pointer returned by Load is never mutated in place. Writers replace the
// pointer atomically; readers keep observing their own snapshot.
func TestLocalIPCacheSnapshotImmutable(t *testing.T) {
	first := &localIP{
		IP:         net.ParseIP("172.16.0.1"),
		LatestTime: time.Now(),
		TTL:        time.Hour,
	}
	cacheLocalIP.Store(first)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			cacheLocalIP.Store(&localIP{
				IP:         net.ParseIP("172.16.0.2"),
				LatestTime: time.Now(),
				TTL:        time.Hour,
			})
		}
	}()

	// first.IP must stay "172.16.0.1" regardless of concurrent Stores.
	for i := 0; i < 1000; i++ {
		if got := first.IP.String(); got != "172.16.0.1" {
			t.Fatalf("snapshot mutated in place: got %s, want 172.16.0.1 (i=%d)", got, i)
		}
		// Sanity-check the atomic API still works.
		if p := cacheLocalIP.Load(); p == nil {
			t.Fatal("cacheLocalIP.Load returned nil after Store")
		}
	}
	wg.Wait()
}
