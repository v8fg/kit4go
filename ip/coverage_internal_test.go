package ip

// White-box coverage tests closing the last few statement gaps to 100%.
//
// These live in package `ip` (not `ip_test`) so they can reach the unexported
// helpers (localIPFromSnapshot, updateCacheLocalIP, jsonRet) and drive the
// generated mockery branches (typed-func return slots, Run/RunAndReturn
// expecter helpers, the no-return-value panic, and the NewMockAddrLookup
// constructor) that the external test package cannot exercise.
//
// Production code is untouched; this file only adds tests.

import (
	"encoding/json"
	"fmt"
	"net"
	"testing"
	"time"

	mock "github.com/stretchr/testify/mock"
)

// --- net_info.go: localIPFromSnapshot / updateCacheLocalIP ---

// emptyAddrLookup returns no usable IPv4 (only a loopback and a
// link-local-unicast, both filtered by getLocalIPBytes), so updateCacheLocalIP
// yields "" and the fall-through branch of localIPFromSnapshot is taken.
type emptyAddrLookup struct{}

func (emptyAddrLookup) InterfaceAddrs() ([]net.Addr, error) {
	// getLocalIPBytes skips loopback and link-local-unicast addresses, so
	// neither of these produces an IPv4.
	return []net.Addr{
		&net.IPNet{IP: net.ParseIP("127.0.0.1"), Mask: net.CIDRMask(8, 32)},
		&net.IPNet{IP: net.ParseIP("169.254.1.1"), Mask: net.CIDRMask(16, 32)},
	}, nil
}
func (emptyAddrLookup) Interfaces() ([]net.Interface, error) { return nil, nil }

// errAddrLookup makes InterfaceAddrs error, so getLocalIPBytes returns nil and
// updateCacheLocalIP hits its `return ""` tail (line 134).
type errAddrLookup struct{}

func (errAddrLookup) InterfaceAddrs() ([]net.Addr, error) {
	return nil, fmt.Errorf("InterfaceAddrs unavailable")
}
func (errAddrLookup) Interfaces() ([]net.Interface, error) { return nil, nil }

// TestLocalIPFromSnapshotNilRefreshEmpty covers localIPFromSnapshot's final
// `return ""` (net_info.go:119): a nil snapshot whose refresh also produces
// nothing falls all the way through to the empty return.
func TestLocalIPFromSnapshotNilRefreshEmpty(t *testing.T) {
	origLookup := DefaultAddrLookup
	t.Cleanup(func() { DefaultAddrLookup = origLookup })
	DefaultAddrLookup = emptyAddrLookup{}

	if got := localIPFromSnapshot(nil); got != "" {
		t.Fatalf("localIPFromSnapshot(nil) = %q, want \"\"", got)
	}
}

// TestLocalIPFromSnapshotStaleRefreshEmpty covers the stale-snapshot branch of
// localIPFromSnapshot where the snapshot exists but TTL-expired and the refresh
// still yields nothing: the prior (nil-IP) snapshot is returned as "".
func TestLocalIPFromSnapshotStaleRefreshEmpty(t *testing.T) {
	origLookup := DefaultAddrLookup
	t.Cleanup(func() { DefaultAddrLookup = origLookup })
	DefaultAddrLookup = emptyAddrLookup{}

	// LatestTime far in the past + TTL 0 => time.Now().After(...) is true,
	// forcing the refresh branch.
	stale := &localIP{IP: nil, LatestTime: time.Now().Add(-time.Hour), TTL: 0}
	if got := localIPFromSnapshot(stale); got != "" {
		t.Fatalf("localIPFromSnapshot(stale) = %q, want \"\"", got)
	}
}

// TestUpdateCacheLocalIPEmpty covers updateCacheLocalIP's `return ""`
// (net_info.go:134): when getLocalIPBytes finds no IPv4 the function publishes
// nothing and returns the empty string.
func TestUpdateCacheLocalIPEmpty(t *testing.T) {
	origLookup := DefaultAddrLookup
	t.Cleanup(func() { DefaultAddrLookup = origLookup })
	DefaultAddrLookup = errAddrLookup{}

	if got := updateCacheLocalIP(); got != "" {
		t.Fatalf("updateCacheLocalIP() = %q, want \"\"", got)
	}
}

// --- net_info.go: jsonRet marshal-failure fallback ---

// unmarshallableValue is a map value encoding/json cannot serialize, so
// json.Marshal fails and jsonRet falls back to fmt.Sprintf("%v", data)
// (net_info.go:335). A function value is one of the few types json.Marshal
// rejects with an error.
var unmarshallableValue = func() {}

// TestJSONRetMarshalFailure covers the json.Marshal error branch of jsonRet.
func TestJSONRetMarshalFailure(t *testing.T) {
	data := map[string]any{
		"ip":   "1.2.3.4",
		"func": unmarshallableValue,
	}
	// Sanity: this data set must actually fail to marshal, otherwise the
	// fallback line would never be reached for it.
	if _, err := json.Marshal(data); err == nil {
		t.Fatalf("test data must fail json.Marshal to exercise the fallback")
	}

	got := jsonRet(data)
	if got == "" {
		t.Fatalf("jsonRet returned empty string for unmarshallable data")
	}
	// The fallback uses fmt.Sprintf("%v", data), which includes the map's
	// string form; just confirm it is non-empty and contains the ip entry.
	if !contains(got, "1.2.3.4") {
		t.Fatalf("jsonRet fallback output %q does not contain the ip value", got)
	}
}

// contains is a tiny local substring check to avoid pulling strings into the
// import list just for one assertion.
func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// --- mock_AddrLookup.go: generated mockery branches ---

// fakeAddrSlice is a stable []net.Addr used as the concrete return value.
var fakeAddrSlice = []net.Addr{
	&net.IPNet{IP: net.ParseIP("192.168.1.1"), Mask: net.CIDRMask(24, 32)},
}

var fakeInterfaceSlice = []net.Interface{
	{Index: 1, Name: "lo0"},
}

// TestMockAddrLookupInterfaceAddrsBranches exercises every return-resolution
// branch of the generated InterfaceAddrs mock:
//   - the panic when no return value is configured,
//   - the full-signature typed func (RunAndReturn),
//   - the per-slot typed funcs (func() []net.Addr and func() error),
//   - the Run and RunAndReturn expecter helpers.
func TestMockAddrLookupInterfaceAddrsBranches(t *testing.T) {
	t.Run("panic when no return value configured", func(t *testing.T) {
		m := new(MockAddrLookup)
		// Set up a matching call that supplies only a Run hook (no Return),
		// so Called() returns an empty Arguments slice and the mock panics.
		m.On("InterfaceAddrs").Run(func(args mock.Arguments) {})
		defer func() {
			if r := recover(); r == nil {
				t.Fatalf("InterfaceAddrs without Return must panic")
			}
		}()
		_, _ = m.InterfaceAddrs()
	})

	t.Run("RunAndReturn full signature", func(t *testing.T) {
		m := new(MockAddrLookup)
		m.EXPECT().InterfaceAddrs().RunAndReturn(func() ([]net.Addr, error) {
			return fakeAddrSlice, nil
		}).Once()
		got, err := m.InterfaceAddrs()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("got %d addrs, want 1", len(got))
		}
		m.AssertExpectations(t)
	})

	t.Run("per-slot typed funcs via On().Return", func(t *testing.T) {
		m := new(MockAddrLookup)
		// Slot 0 as func() []net.Addr (single-value func, not the full
		// signature), slot 1 as func() error. This hits the two per-slot
		// typed-func branches (lines 37-39 and 45-47).
		m.On("InterfaceAddrs").Return(
			func() []net.Addr { return fakeAddrSlice },
			func() error { return nil },
		).Once()
		got, err := m.InterfaceAddrs()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("got %d addrs, want 1", len(got))
		}
		m.AssertExpectations(t)
	})

	t.Run("Run expecter helper executes side effect", func(t *testing.T) {
		m := new(MockAddrLookup)
		ran := false
		m.EXPECT().InterfaceAddrs().
			Run(func() { ran = true }).
			Return(fakeAddrSlice, nil).Once()
		if _, err := m.InterfaceAddrs(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ran {
			t.Fatalf("Run side effect did not execute")
		}
		m.AssertExpectations(t)
	})
}

// TestMockAddrLookupInterfacesBranches mirrors the InterfaceAddrs coverage for
// the Interfaces mock method.
func TestMockAddrLookupInterfacesBranches(t *testing.T) {
	t.Run("panic when no return value configured", func(t *testing.T) {
		m := new(MockAddrLookup)
		m.On("Interfaces").Run(func(args mock.Arguments) {})
		defer func() {
			if r := recover(); r == nil {
				t.Fatalf("Interfaces without Return must panic")
			}
		}()
		_, _ = m.Interfaces()
	})

	t.Run("RunAndReturn full signature", func(t *testing.T) {
		m := new(MockAddrLookup)
		m.EXPECT().Interfaces().RunAndReturn(func() ([]net.Interface, error) {
			return fakeInterfaceSlice, nil
		}).Once()
		got, err := m.Interfaces()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("got %d interfaces, want 1", len(got))
		}
		m.AssertExpectations(t)
	})

	t.Run("per-slot typed funcs via On().Return", func(t *testing.T) {
		m := new(MockAddrLookup)
		m.On("Interfaces").Return(
			func() []net.Interface { return fakeInterfaceSlice },
			func() error { return nil },
		).Once()
		got, err := m.Interfaces()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("got %d interfaces, want 1", len(got))
		}
		m.AssertExpectations(t)
	})

	t.Run("Run expecter helper executes side effect", func(t *testing.T) {
		m := new(MockAddrLookup)
		ran := false
		m.EXPECT().Interfaces().
			Run(func() { ran = true }).
			Return(fakeInterfaceSlice, nil).Once()
		if _, err := m.Interfaces(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ran {
			t.Fatalf("Run side effect did not execute")
		}
		m.AssertExpectations(t)
	})
}

// TestNewMockAddrLookupConstructor covers the NewMockAddrLookup factory
// (mock_AddrLookup.go:143-149), which registers a testing-T, wires a Cleanup
// asserting expectations, and returns a usable mock.
func TestNewMockAddrLookupConstructor(t *testing.T) {
	m := NewMockAddrLookup(t)
	m.EXPECT().InterfaceAddrs().Return(fakeAddrSlice, nil).Once()
	m.EXPECT().Interfaces().Return(fakeInterfaceSlice, nil).Once()

	got, err := m.InterfaceAddrs()
	if err != nil || len(got) != 1 {
		t.Fatalf("InterfaceAddrs = %v, %v", got, err)
	}
	gotIfaces, err := m.Interfaces()
	if err != nil || len(gotIfaces) != 1 {
		t.Fatalf("Interfaces = %v, %v", gotIfaces, err)
	}
	// Expectations are asserted via the Cleanup registered by the
	// constructor; AssertExpectations is also called explicitly here for an
	// immediate signal within the subtest.
	m.AssertExpectations(t)
}
