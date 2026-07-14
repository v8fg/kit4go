package cert

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/mock"
)

// matchDomain returns a mock matcher that asserts the synthetic ClientHello
// carries the expected ServerName (i.e. clientHello wired the domain through).
func matchDomain(domain string) any {
	return mock.MatchedBy(func(h *tls.ClientHelloInfo) bool { return h != nil && h.ServerName == domain })
}

func TestNew(t *testing.T) {
	convey.Convey("missing domains", t, func() {
		_, err := New(Config{Dir: t.TempDir()})
		convey.So(errors.Is(err, ErrNoDomains), convey.ShouldBeTrue)
	})
	convey.Convey("missing dir", t, func() {
		_, err := New(Config{Domains: []string{"a.com"}})
		convey.So(errors.Is(err, ErrNoDir), convey.ShouldBeTrue)
	})
	convey.Convey("wires staging URL and HostPolicy to the configured domains", t, func() {
		c, err := New(Config{Domains: []string{"a.com", "b.com"}, Dir: t.TempDir(), Staging: true})
		convey.So(err, convey.ShouldBeNil)
		m := c.mgr.(*acmeManagerAdapter).m
		convey.So(m.Client.DirectoryURL, convey.ShouldEqual, LEStagingDirectoryURL)
		convey.So(m.HostPolicy(context.Background(), "a.com"), convey.ShouldBeNil)
		convey.So(m.HostPolicy(context.Background(), "evil.com"), convey.ShouldNotBeNil)
	})
	convey.Convey("production directory by default", t, func() {
		c, _ := New(Config{Domains: []string{"a.com"}, Dir: t.TempDir()})
		convey.So(c.mgr.(*acmeManagerAdapter).m.Client.DirectoryURL, convey.ShouldEqual, LEProdDirectoryURL)
	})
	convey.Convey("HTTPClient is wired into the ACME client when set", t, func() {
		hc := &http.Client{Timeout: 7 * time.Second}
		c, _ := New(Config{Domains: []string{"a.com"}, Dir: t.TempDir(), HTTPClient: hc})
		convey.So(c.mgr.(*acmeManagerAdapter).m.Client.HTTPClient, convey.ShouldEqual, hc)
	})
	convey.Convey("HTTPClient nil leaves the ACME client default", t, func() {
		c, _ := New(Config{Domains: []string{"a.com"}, Dir: t.TempDir()})
		convey.So(c.mgr.(*acmeManagerAdapter).m.Client.HTTPClient, convey.ShouldBeNil)
	})
}

// newTestClient wires a Client with fresh mocks and a self-signed cert for domain.
func newTestClient(t *testing.T, domain string) (*Client, *MockACMEManager, *MockDirWriter, *tls.Certificate) {
	t.Helper()
	mgr := NewMockACMEManager(t)
	w := NewMockDirWriter(t)
	cert := selfSignedCert(t, domain, true, 90*24*time.Hour)
	c := newWithSeams(Config{Domains: []string{domain}, Dir: t.TempDir(), KeyType: KeyTypeECDSA}, mgr, w)
	return c, mgr, w, cert
}

func TestObtainAndWriteIssue(t *testing.T) {
	domain := "example.com"
	convey.Convey("first call writes, fires issue+write, bumps metrics", t, func() {
		c, mgr, w, cert := newTestClient(t, domain)
		mgr.EXPECT().GetCertificate(matchDomain(domain)).Return(cert, nil).Once()
		w.EXPECT().Write(mock.Anything, domain, mock.Anything, mock.Anything).Return(nil).Once()

		var got []Event
		c.SetOnEvent(func(e Event) { got = append(got, e) })

		convey.So(c.EnsureCert(context.Background(), domain), convey.ShouldBeNil)
		convey.So(c.Metrics().Issued, convey.ShouldEqual, uint64(1))
		convey.So(c.Metrics().Written, convey.ShouldEqual, uint64(1))
		convey.So(c.Metrics().Failed, convey.ShouldEqual, uint64(0))
		convey.So(len(got), convey.ShouldEqual, 2)
		convey.So(got[0].Name, convey.ShouldEqual, EventIssue)
		convey.So(got[0].Cert.Domain, convey.ShouldEqual, domain)
		convey.So(got[1].Name, convey.ShouldEqual, EventWrite)
	})
}

func TestObtainAndWriteSkipAndRenew(t *testing.T) {
	domain := "example.com"
	convey.Convey("same cert on second call → skip, no extra write", t, func() {
		c, mgr, w, cert := newTestClient(t, domain)
		mgr.EXPECT().GetCertificate(matchDomain(domain)).Return(cert, nil).Times(2)
		w.EXPECT().Write(mock.Anything, domain, mock.Anything, mock.Anything).Return(nil).Once()

		convey.So(c.EnsureCert(context.Background(), domain), convey.ShouldBeNil) // issue
		convey.So(c.EnsureCert(context.Background(), domain), convey.ShouldBeNil) // skip
		convey.So(c.Metrics().Issued, convey.ShouldEqual, uint64(1))
		convey.So(c.Metrics().Skipped, convey.ShouldEqual, uint64(1))
		convey.So(c.Metrics().Written, convey.ShouldEqual, uint64(1))
	})
	convey.Convey("newer cert on second call → renew + write", t, func() {
		mgr := NewMockACMEManager(t)
		w := NewMockDirWriter(t)
		c1 := selfSignedCert(t, domain, true, 90*24*time.Hour)
		c2 := selfSignedCert(t, domain, true, 91*24*time.Hour) // later NotAfter
		mgr.EXPECT().GetCertificate(matchDomain(domain)).Return(c1, nil).Once()
		mgr.EXPECT().GetCertificate(matchDomain(domain)).Return(c2, nil).Once()
		w.EXPECT().Write(mock.Anything, domain, mock.Anything, mock.Anything).Return(nil).Times(2)
		c := newWithSeams(Config{Domains: []string{domain}, Dir: t.TempDir()}, mgr, w)

		convey.So(c.EnsureCert(context.Background(), domain), convey.ShouldBeNil) // issue
		convey.So(c.EnsureCert(context.Background(), domain), convey.ShouldBeNil) // renew
		convey.So(c.Metrics().Issued, convey.ShouldEqual, uint64(1))
		convey.So(c.Metrics().Renewed, convey.ShouldEqual, uint64(1))
		convey.So(c.Metrics().Written, convey.ShouldEqual, uint64(2))
	})
}

func TestObtainAndWriteError(t *testing.T) {
	domain := "example.com"
	convey.Convey("obtain failure fires error event and bumps failed", t, func() {
		c, mgr, _, _ := newTestClient(t, domain)
		mgr.EXPECT().GetCertificate(matchDomain(domain)).Return(nil, errors.New("network down")).Once()

		var got []Event
		c.SetOnEvent(func(e Event) { got = append(got, e) })

		err := c.EnsureCert(context.Background(), domain)
		convey.So(err, convey.ShouldNotBeNil)
		convey.So(c.Metrics().Failed, convey.ShouldEqual, uint64(1))
		convey.So(len(got), convey.ShouldEqual, 1)
		convey.So(got[0].Name, convey.ShouldEqual, EventError)
		convey.So(got[0].Err, convey.ShouldNotBeNil)
	})
}

func TestObtainAndWriteLeafFallback(t *testing.T) {
	domain := "example.com"
	convey.Convey("parses the leaf when the manager returns a cert without Leaf set", t, func() {
		mgr := NewMockACMEManager(t)
		w := NewMockDirWriter(t)
		full := selfSignedCert(t, domain, true, 90*24*time.Hour)
		noLeaf := &tls.Certificate{Certificate: full.Certificate, PrivateKey: full.PrivateKey} // Leaf intentionally nil
		mgr.EXPECT().GetCertificate(mock.Anything).Return(noLeaf, nil).Once()
		w.EXPECT().Write(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
		c := newWithSeams(Config{Domains: []string{domain}, Dir: t.TempDir()}, mgr, w)

		convey.So(c.EnsureCert(context.Background(), domain), convey.ShouldBeNil)
		convey.So(c.Metrics().Issued, convey.ShouldEqual, uint64(1))
	})
}

func TestServingDelegates(t *testing.T) {
	domain := "example.com"
	convey.Convey("GetCertificate/HTTPHandler/TLSConfig delegate to the manager", t, func() {
		c, mgr, _, cert := newTestClient(t, domain)
		mgr.EXPECT().GetCertificate(mock.Anything).Return(cert, nil).Once()
		mgr.EXPECT().HTTPHandler(mock.Anything).Return(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).Once()
		mgr.EXPECT().TLSConfig().Return(&tls.Config{}).Once()

		gc, err := c.GetCertificate(&tls.ClientHelloInfo{ServerName: domain})
		convey.So(err, convey.ShouldBeNil)
		convey.So(gc, convey.ShouldEqual, cert)
		convey.So(c.HTTPHandler(nil), convey.ShouldNotBeNil)
		convey.So(c.TLSConfig(), convey.ShouldNotBeNil)
	})
}

func TestNewWithManager(t *testing.T) {
	convey.Convey("NewWithManager wires an injected backend with the real writer", t, func() {
		mgr := NewMockACMEManager(t)
		cert := selfSignedCert(t, "a.com", true, 90*24*time.Hour)
		mgr.EXPECT().GetCertificate(matchDomain("a.com")).Return(cert, nil).Once()
		c, err := NewWithManager(Config{Domains: []string{"a.com"}, Dir: t.TempDir()}, mgr)
		convey.So(err, convey.ShouldBeNil)
		convey.So(c.EnsureCert(context.Background(), "a.com"), convey.ShouldBeNil)
		convey.So(c.Metrics().Issued, convey.ShouldEqual, uint64(1))
		convey.So(c.Metrics().Written, convey.ShouldEqual, uint64(1))
	})
	convey.Convey("NewWithManager still validates config", t, func() {
		mgr := NewMockACMEManager(t)
		_, err := NewWithManager(Config{}, mgr)
		convey.So(errors.Is(err, ErrNoDomains), convey.ShouldBeTrue)
	})
}

func TestEnsureCertSingleFlight(t *testing.T) {
	domain := "example.com"
	convey.Convey("100 concurrent calls dedupe to one obtain and one write", t, func() {
		mgr := NewMockACMEManager(t)
		w := NewMockDirWriter(t)
		cert := selfSignedCert(t, domain, true, 90*24*time.Hour)
		mgr.EXPECT().GetCertificate(mock.Anything).Return(cert, nil).Once()
		w.EXPECT().Write(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
		c := newWithSeams(Config{Domains: []string{domain}, Dir: t.TempDir()}, mgr, w)

		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = c.EnsureCert(context.Background(), domain)
			}()
		}
		wg.Wait()
		convey.So(c.Metrics().Issued, convey.ShouldEqual, uint64(1))
		convey.So(c.Metrics().Written, convey.ShouldEqual, uint64(1))
	})
}
