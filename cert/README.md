# cert

HTTPS certificate issuance and renewal via an ACME certificate authority (Let's
Encrypt by default). Wraps `golang.org/x/crypto/acme/autocert` with the
cross-cutting concerns a real deployment needs. Isolated Go module
(`github.com/v8fg/kit4go/cert`).

`autocert` owns the ACME protocol, the account key, and renewal timing. This
package adds:

- **Proactive renewal loop** — low-traffic sites renew without an inbound TLS
  handshake, and the cert+key files are written even when nothing is serving TLS.
- **Atomic directory writer** — `<domain>.crt` (chain, 0644) and `<domain>.key`
  (private key, 0600) land at `Config.Dir` via temp+fsync+rename (no torn writes);
  point an external server (nginx, another process) at them, certbot-style.
- **In-process serving** (secondary) — `GetCertificate` / `HTTPHandler` /
  `TLSConfig` let a Go process terminate TLS directly.
- **Metrics & events** — atomic counters and hooks expose issue/renew/write/skip/
  error/panic outcomes for monitoring and alerting.

## Quick start (directory writer — primary)

```go
import "github.com/v8fg/kit4go/cert"

mgr, err := cert.New(cert.Config{
	Domains: []string{"example.com"},
	Dir:     "/etc/myapp/certs",
	Email:   "ops@example.com",
	Staging: true, // Let's Encrypt staging until verified
})
if err != nil {
	return err
}
stop := mgr.Start(context.Background())
defer stop()
// /etc/myapp/certs/example.com.crt and .key are now kept valid and renewed.
```

## Port 80 (HTTP-01)

The http-01 challenge needs a handler reachable on port 80:

```go
http.Handle("/", mgr.HTTPHandler(yourFallbackHandler))
```

## API

| Method / Function | Description |
|-------------------|-------------|
| `New(cfg)` | Construct with the default autocert (HTTP-01) backend |
| `NewWithManager(cfg, mgr)` | Construct with a custom `ACMEManager` (e.g. the lego DNS-01 backend) |
| `Start(ctx)` | Run the renewal loop in a goroutine; returns a `stop()` that blocks until exit |
| `Run(ctx)` | Run the renewal loop in the current goroutine (blocks) |
| `EnsureCert(ctx, domain)` | Force issue/renew for one domain (ad-hoc; see note below) |
| `GetCertificate` / `HTTPHandler(fb)` / `TLSConfig()` | In-process TLS serving (may return nil for non-HTTP-01 backends) |
| `Metrics()` | Atomic counter snapshot (Issued/Renewed/Skipped/Written/Failed/Ticks/Panics) |
| `SetOnEvent(fn)` | Hook for issue/renew/write/skip/error/panic events |
| `SetOnPanic(fn)` | Hook fired with the recovered value when the loop catches a panic |

## Notes

- **Staging first.** Let's Encrypt rate limits are strict; set `Config.Staging`
  until everything is verified. `autocert` caps each issuance attempt at a
  5-minute internal timeout.
- **Host policy.** `Config.Domains` is wired into autocert's `HostPolicy` — only
  configured hosts are ever issued for. Domains are validated to reject `/`,
  `:`, and space (path components); the filename is a single component, so POSIX
  path traversal is not possible.
- **Panic recovery.** The renewal loop is a library-owned goroutine: each tick is
  panic-recovered so a transient panic in the ACME backend, the certificate
  parser, or the directory writer is counted (`Metrics.Panics`), reported via
  `SetOnPanic` and a `"panic"` event, then swallowed — the loop keeps renewing
  and certs never silently expire. A panicking *user hook* is also guarded so an
  alerting/metrics bug cannot kill the loop.
- **EnsureCert (ad-hoc) is NOT recovered.** Unlike the loop, a direct
  `EnsureCert` call propagates a backend panic to the caller — defer-recover in
  your own goroutine if it must survive.
- **Pluggable backend.** A future DNS-01 / lego backend drops in behind the
  `ACMEManager` interface without changes to `Client` or the renewal loop. See
  the nested `cert/lego` module for a lego-based DNS-01 implementation.
- All public methods on `Client` are safe for concurrent use.
