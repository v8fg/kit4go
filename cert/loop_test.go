package cert

import (
	"context"
	"crypto/tls"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/mock"
)

func TestRefreshAll(t *testing.T) {
	convey.Convey("refreshAll issues every configured domain and bumps ticks", t, func() {
		mgr := NewMockACMEManager(t)
		w := NewMockDirWriter(t)
		domains := []string{"a.com", "b.com"}
		for _, d := range domains {
			mgr.EXPECT().GetCertificate(matchDomain(d)).Return(selfSignedCert(t, d, true, 90*24*time.Hour), nil).Once()
		}
		w.EXPECT().Write(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Times(len(domains))
		c := newWithSeams(Config{Domains: domains, Dir: t.TempDir(), CheckInterval: time.Hour}, mgr, w)

		c.refreshAll(context.Background())
		convey.So(c.Metrics().Ticks, convey.ShouldEqual, uint64(1))
		convey.So(c.Metrics().Issued, convey.ShouldEqual, uint64(2))
	})
}

func TestRunReturnsOnCancel(t *testing.T) {
	convey.Convey("Run returns ctx.Err() and does no issuance when ctx is pre-cancelled", t, func() {
		mgr := NewMockACMEManager(t)
		w := NewMockDirWriter(t)
		// refreshAll checks ctx.Err() before any EnsureCert, so these are never
		// called; Maybe keeps the mock happy at zero calls.
		mgr.EXPECT().GetCertificate(mock.Anything).Maybe()
		w.EXPECT().Write(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Maybe()
		c := newWithSeams(Config{Domains: []string{"a.com"}, Dir: t.TempDir(), CheckInterval: time.Hour}, mgr, w)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := c.Run(ctx)
		convey.So(err, convey.ShouldBeError)
		convey.So(c.Metrics().Issued, convey.ShouldEqual, uint64(0))
	})
}

func TestStartStop(t *testing.T) {
	convey.Convey("Start runs the loop in a goroutine and stop blocks until it exits", t, func() {
		mgr := NewMockACMEManager(t)
		w := NewMockDirWriter(t)
		cert := selfSignedCert(t, "a.com", true, 90*24*time.Hour)
		mgr.EXPECT().GetCertificate(mock.Anything).Maybe().Return(cert, nil)
		w.EXPECT().Write(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Maybe().Return(nil)
		c := newWithSeams(Config{Domains: []string{"a.com"}, Dir: t.TempDir(), CheckInterval: 10 * time.Millisecond}, mgr, w)

		stop := c.Start(context.Background())
		time.Sleep(60 * time.Millisecond) // let a few ticks run
		stop()
		convey.So(c.Metrics().Ticks, convey.ShouldBeGreaterThanOrEqualTo, uint64(1))
	})
}

// TestRunRecoversPanic is the regression test for the renewal-loop P0: a panic
// in the ACME backend (or parser/writer) must be recovered so the loop keeps
// renewing — it must NOT die (leaving certs to expire) and Stop must NOT hang.
func TestRunRecoversPanic(t *testing.T) {
	convey.Convey("a panic in the renewal loop is recovered; the loop survives, Stop returns, and the hook fires", t, func() {
		mgr := NewMockACMEManager(t)
		w := NewMockDirWriter(t)
		// GetCertificate panics on every call — simulates a buggy/ACME-backend panic.
		mgr.EXPECT().GetCertificate(mock.Anything).Run(func(*tls.ClientHelloInfo) {
			panic("boom")
		}).Maybe()
		w.EXPECT().Write(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Maybe()

		c := newWithSeams(Config{
			Domains:       []string{"a.com"},
			Dir:           t.TempDir(),
			CheckInterval: 10 * time.Millisecond,
		}, mgr, w)

		var saw atomic.Value // captures the recovered panic value
		c.SetOnPanic(func(r any) { saw.Store(r) })

		stop := c.Start(context.Background())
		time.Sleep(60 * time.Millisecond) // let several panicking ticks run

		// Stop MUST return (the pre-fix bug hung here because close(done) was
		// skipped after the panic). Fail fast instead of hanging the suite.
		stopped := make(chan struct{})
		go func() { stop(); close(stopped) }()
		select {
		case <-stopped:
		case <-time.After(2 * time.Second):
			t.Fatal("Stop hung — renewal-loop P0 regression")
		}

		convey.So(c.Metrics().Panics, convey.ShouldBeGreaterThan, uint64(0))
		convey.So(c.Metrics().Ticks, convey.ShouldBeGreaterThan, uint64(1))
		// singleflight v0.21.0 wraps a panicking fn in newPanicError (an error whose
		// message embeds the original panic value + stack) and re-panics; the
		// recovered value's %v therefore contains the original "boom".
		convey.So(saw.Load(), convey.ShouldNotBeNil)
		convey.So(fmt.Sprintf("%v", saw.Load()), convey.ShouldContainSubstring, "boom")
	})
}
