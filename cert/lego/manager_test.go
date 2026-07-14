package lego

import "testing"

// TestNewRequiresDNSProvider covers the one New path that does not touch the
// network: a nil DNSProvider is rejected before any ACME call. Full DNS-01
// issuance requires a real DNS provider + domain and is exercised end-to-end
// via cmd/certdemo-pebble's DNS variant or a live CA, not here.
func TestNewRequiresDNSProvider(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatal("cert/lego: expected an error when DNSProvider is nil")
	}
}
