// Package cert issues, renews and writes HTTPS certificates via an ACME
// certificate authority (Let's Encrypt by default). It wraps
// golang.org/x/crypto/acme/autocert with the cross-cutting concerns a real
// deployment needs, so callers do not have to re-implement them:
//
//   - Automatic issuance and renewal (HTTP-01 challenge). autocert owns the ACME
//     protocol, the account key and renewal timing; this package adds a
//     proactive loop so low-traffic sites renew without an inbound TLS
//     handshake and so the files are written even when nothing is serving TLS.
//   - Directory writer. For each domain, <domain>.crt (chain) and <domain>.key
//     (private key, 0600) are written atomically to [Config.Dir] — point an
//     external server (nginx, another process) at these files, certbot-style.
//   - In-process serving (secondary). [Client.GetCertificate], [Client.HTTPHandler]
//     and [Client.TLSConfig] let a Go process terminate TLS directly via autocert.
//   - Metrics & events. Atomic counters ([Client.Metrics]) and an event hook
//     ([Client.SetOnEvent]) expose issue/renew/write/skip/error outcomes for
//     monitoring and alerting.
//
// The package depends only on the Go standard library and
// golang.org/x/crypto/acme/autocert.
//
// # Usage (directory writer — primary)
//
//	mgr, err := cert.New(cert.Config{
//	    Domains: []string{"example.com"},
//	    Dir:     "/etc/myapp/certs",
//	    Email:   "ops@example.com",
//	    Staging: true, // use Let's Encrypt staging until verified
//	})
//	if err != nil {
//	    return err
//	}
//	stop := mgr.Start(context.Background())
//	defer stop()
//	// /etc/myapp/certs/example.com.crt and .key are now kept valid and renewed.
//
// # Port 80 (HTTP-01)
//
// The http-01 challenge requires a handler reachable on port 80. Mount it on an
// existing port-80 server (or a dedicated one):
//
//	http.Handle("/", mgr.HTTPHandler(yourFallbackHandler))
//
// # Notes
//
// Let's Encrypt rate limits are strict; set [Config.Staging] until everything is
// verified. autocert caps each issuance attempt at a 5-minute internal timeout.
// [Config.Domains] is wired into autocert's HostPolicy so only configured hosts
// are ever issued for. A future DNS-01 / lego backend can drop in behind the
// [ACMEManager] interface without changes to [Client] or the renewal loop.
//
// All public methods on [Client] are safe for concurrent use by multiple
// goroutines.
package cert
