package lego

import (
	"crypto/tls"
	"testing"
	"time"
)

// TestNewRequiresDNSProvider covers the one New path that does not touch the
// network: a nil DNSProvider is rejected before any ACME call. Full DNS-01
// issuance requires a real DNS provider + domain and is exercised end-to-end
// via cmd/certdemo-pebble's DNS variant or a live CA, not here.
func TestNewRequiresDNSProvider(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatal("cert/lego: expected an error when DNSProvider is nil")
	}
}

// TestGetCertificateRequiresServerName covers the guard: a nil or empty
// ServerName is rejected before any cache lookup or obtain attempt (no network).
func TestGetCertificateRequiresServerName(t *testing.T) {
	m := &Manager{cache: make(map[string]*cachedCert), now: time.Now}

	if _, err := m.GetCertificate(nil); err == nil {
		t.Fatal("cert/lego: expected an error for a nil ClientHelloInfo")
	}
	if _, err := m.GetCertificate(&tls.ClientHelloInfo{}); err == nil {
		t.Fatal("cert/lego: expected an error for an empty ServerName")
	}
}

// TestGetCertificateCacheHit covers the cache-hit branch: when a cached
// certificate is still valid (remaining lifetime > RenewBefore), GetCertificate
// returns it WITHOUT calling Obtain (no network). Constructed white-box with a
// pre-seeded cache so New's network registration is bypassed.
func TestGetCertificateCacheHit(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cached := &tls.Certificate{}
	m := &Manager{
		renewBefore: 720 * time.Hour, // 30d
		now:         func() time.Time { return now },
		cache: map[string]*cachedCert{
			// 90d left → well past the 30d RenewBefore threshold → fresh.
			"example.com": {tls: cached, notAfter: now.Add(90 * 24 * time.Hour)},
		},
	}

	got, err := m.GetCertificate(&tls.ClientHelloInfo{ServerName: "example.com"})
	if err != nil {
		t.Fatalf("cert/lego: cache hit returned error: %v", err)
	}
	if got != cached {
		t.Fatal("cert/lego: cache hit did not return the cached certificate (would have re-obtained)")
	}
}
