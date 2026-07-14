package cert

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"
)

// Metrics is a point-in-time snapshot of the counters maintained by a [Client].
// Values are gathered via atomic loads and may be slightly inconsistent with
// one another under concurrent load; that is acceptable for monitoring.
type Metrics struct {
	// Issued is the number of certificates written for domains that had none on
	// disk yet (first-time issuance).
	Issued uint64
	// Renewed is the number of on-disk certificates replaced by a renewed one.
	Renewed uint64
	// Skipped is the number of loop ticks that left a domain's on-disk cert
	// unchanged (not yet due for renewal).
	Skipped uint64
	// Written is the number of successful cert+key file writes (issuances +
	// renewals). It equals Issued + Renewed.
	Written uint64
	// Failed is the number of obtain, parse or write attempts that failed.
	Failed uint64
	// Ticks is the number of renewal-loop refresh passes executed.
	Ticks uint64
	// Panics is the number of panics recovered inside the renewal loop. A
	// non-zero value means the ACME backend, parser, or writer panicked but was
	// caught so the loop kept running — investigate via the OnPanic hook.
	Panics uint64
}

// Client issues, renews and writes HTTPS certificates via an ACME certificate
// authority (Let's Encrypt by default). It wraps autocert, adding a proactive
// renewal loop and an atomic directory writer so the split cert+key files land
// at [Config.Dir] and stay current without an inbound TLS handshake. The zero
// value is not usable; construct one with [New].
//
// All methods are safe for concurrent use by multiple goroutines.
type Client struct {
	cfg    Config
	mgr    ACMEManager
	writer DirWriter

	// sf dedupes concurrent ensureCert calls per domain.
	sf singleflight.Group

	// lastNotAfter tracks the NotAfter of the cert last written per domain, so
	// the loop can skip re-writing unchanged certs.
	lastMu       sync.Mutex
	lastNotAfter map[string]time.Time

	// Counters are laid out as separate atomics rather than a single packed
	// struct so increments do not contend on the same cache line.
	issued  atomic.Uint64
	renewed atomic.Uint64
	skipped atomic.Uint64
	written atomic.Uint64
	failed  atomic.Uint64
	ticks   atomic.Uint64
	panics  atomic.Uint64

	// onEvent, when non-nil, is invoked for every issuance/renewal/write/skip/
	// error outcome. Set via SetOnEvent and read with an atomic load, so the
	// default (nil) is zero-overhead on the hot path.
	onEvent atomic.Pointer[func(Event)]

	// onPanic, when non-nil, is invoked with the recovered value when the
	// renewal loop catches a panic. Set via SetOnPanic; nil (the default) is a
	// single Load on the recover path — zero overhead when no panic occurs.
	// Mirrors the library-owned-worker convention used by workerpool/batcher/etc.
	onPanic atomic.Pointer[func(any)]
}

// New constructs a [Client] from cfg, filling zero fields with the package
// defaults and creating Config.Dir and Config.CacheDir (0700) if missing. It
// does NOT contact the ACME CA — issuance happens lazily on the first
// [Client.Run] tick or [Client.EnsureCert] call — so a misconfigured path fails
// here at construction rather than mid-issuance.
func New(cfg Config) (*Client, error) {
	cfg, err := prepare(cfg)
	if err != nil {
		return nil, err
	}
	return newClient(cfg, &acmeManagerAdapter{m: newAutocertManager(cfg)})
}

// NewWithManager is like New but injects a custom [ACMEManager] backend instead
// of the default autocert wrapper. Use it to plug in an alternative ACME engine
// — e.g. a lego-based DNS-01 backend for wildcard/internal certs — while keeping
// the same atomic directory writer, proactive renewal loop, metrics and events.
// Config.Dir and Config.CacheDir are created (0700) exactly as in [New].
func NewWithManager(cfg Config, mgr ACMEManager) (*Client, error) {
	cfg, err := prepare(cfg)
	if err != nil {
		return nil, err
	}
	return newClient(cfg, mgr)
}

// prepare applies defaults and validates the config, returning the resolved
// config. Shared by [New] and [NewWithManager].
func prepare(cfg Config) (Config, error) {
	cfg = cfg.withDefaults()
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// newClient creates the output dirs and wires the supplied manager with the real
// osDirWriter. The manager is injected so tests (and NewWithManager) can supply
// a non-autocert backend.
func newClient(cfg Config, mgr ACMEManager) (*Client, error) {
	if err := os.MkdirAll(cfg.Dir, 0o700); err != nil {
		return nil, fmt.Errorf("cert: create dir %q: %w", cfg.Dir, err)
	}
	if err := os.MkdirAll(cfg.CacheDir, 0o700); err != nil {
		return nil, fmt.Errorf("cert: create cache dir %q: %w", cfg.CacheDir, err)
	}
	return newWithSeams(cfg, mgr, &osDirWriter{dir: cfg.Dir}), nil
}

// newWithSeams wires a [Client] with the supplied (injectable) seams. New calls
// it with the real autocert adapter and os writer; tests pass mocks.
func newWithSeams(cfg Config, mgr ACMEManager, writer DirWriter) *Client {
	return &Client{
		cfg:          cfg,
		mgr:          mgr,
		writer:       writer,
		lastNotAfter: make(map[string]time.Time),
	}
}

// SetOnEvent installs a hook invoked for every issuance/renewal/write/skip/error
// event. Pass nil to disable a previously-installed hook. The hook is intended
// for metrics/alerting and must be cheap and non-blocking: it fires
// synchronously on the goroutine performing the work.
func (c *Client) SetOnEvent(fn func(Event)) {
	if fn == nil {
		c.onEvent.Store(nil)
		return
	}
	f := fn // copy to heap
	c.onEvent.Store(&f)
}

// fireEvent is the single chokepoint for hook dispatch. When onEvent is nil
// (the default) the call collapses to a single nil compare. The hook is invoked
// under a recover so a panicking user hook can never escape into the renewal
// loop (a buggy alerting/metrics hook must not kill renewal — the L contract).
func (c *Client) fireEvent(evt Event) {
	p := c.onEvent.Load()
	if p == nil {
		return
	}
	defer func() { _ = recover() }() // a panicking event hook is swallowed, not propagated
	(*p)(evt)
}

// SetOnPanic installs a hook invoked with the recovered value whenever the
// renewal loop catches a panic. Pass nil to disable. The hook is for alerting
// (a recovered panic is a bug worth investigating) and must be non-blocking: it
// fires on the loop goroutine. Mirrors the library-owned-worker convention.
func (c *Client) SetOnPanic(fn func(any)) {
	if fn == nil {
		c.onPanic.Store(nil)
		return
	}
	f := fn
	c.onPanic.Store(&f)
}

// failEvent records a failed obtain/parse/write for domain: it bumps the failed
// counter, fires an "error" event carrying err, and returns err for propagation.
func (c *Client) failEvent(domain string, err error) error {
	c.failed.Add(1)
	c.fireEvent(Event{Name: EventError, Domain: domain, Err: err})
	return err
}

// Metrics returns a point-in-time snapshot of the client's counters.
func (c *Client) Metrics() Metrics {
	return Metrics{
		Issued:  c.issued.Load(),
		Renewed: c.renewed.Load(),
		Skipped: c.skipped.Load(),
		Written: c.written.Load(),
		Failed:  c.failed.Load(),
		Ticks:   c.ticks.Load(),
		Panics:  c.panics.Load(),
	}
}

// EnsureCert ensures a valid certificate for domain is present on disk at
// <domain>.crt/<domain>.key, obtaining or renewing it via the ACME CA if
// needed. Concurrent calls for the same domain are single-flight deduped. It is
// the per-domain entry point used by the renewal loop and is also exported for
// ad-hoc use (e.g. forcing issuance at startup).
func (c *Client) EnsureCert(ctx context.Context, domain string) error {
	_, err, _ := c.sf.Do(domain, func() (any, error) {
		return nil, c.obtainAndWrite(ctx, domain)
	})
	return err
}

// obtainAndWrite is the core per-domain logic, executed under single-flight.
// It obtains/loads the certificate via the ACME manager, splits it into PEM
// blocks, and writes the files unless the on-disk cert is already this leaf.
func (c *Client) obtainAndWrite(ctx context.Context, domain string) error {
	// autocert.GetCertificate ignores ctx (it uses an internal 5m context), so
	// we honour cancellation up front rather than mid-issuance.
	if err := ctx.Err(); err != nil {
		return err
	}

	cert, err := c.mgr.GetCertificate(clientHello(domain, c.cfg.KeyType))
	if err != nil {
		return c.failEvent(domain, fmt.Errorf("cert: obtain %q: %w", domain, err))
	}

	leaf := cert.Leaf
	if leaf == nil {
		if len(cert.Certificate) == 0 {
			return c.failEvent(domain, fmt.Errorf("cert: obtain %q: empty certificate", domain))
		}
		parsed, perr := x509.ParseCertificate(cert.Certificate[0])
		if perr != nil {
			return c.failEvent(domain, fmt.Errorf("cert: parse leaf for %q: %w", domain, perr))
		}
		leaf = parsed
	}

	c.lastMu.Lock()
	last := c.lastNotAfter[domain]
	c.lastMu.Unlock()

	// Skip when the on-disk cert is already this leaf (same NotAfter). After a
	// process start last is zero, so every domain writes once.
	if !last.IsZero() && !leaf.NotAfter.After(last) {
		c.skipped.Add(1)
		c.fireEvent(Event{Name: EventSkip, Domain: domain, Cert: certInfo(domain, leaf)})
		return nil
	}

	certPEM, keyPEM, err := splitCertKey(cert)
	if err != nil {
		return c.failEvent(domain, err)
	}
	if err := c.writer.Write(ctx, domain, certPEM, keyPEM); err != nil {
		return c.failEvent(domain, fmt.Errorf("cert: write %q: %w", domain, err))
	}

	c.lastMu.Lock()
	c.lastNotAfter[domain] = leaf.NotAfter
	c.lastMu.Unlock()

	name := EventRenew
	if last.IsZero() {
		name = EventIssue
		c.issued.Add(1)
	} else {
		c.renewed.Add(1)
	}
	c.written.Add(1)
	c.fireEvent(Event{Name: name, Domain: domain, Cert: certInfo(domain, leaf)})
	c.fireEvent(Event{Name: EventWrite, Domain: domain, Cert: certInfo(domain, leaf)})
	return nil
}

// GetCertificate serves a certificate for an inbound TLS handshake, delegating
// to autocert (secondary, in-process serving mode). Use it as
// tls.Config.GetCertificate.
func (c *Client) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	return c.mgr.GetCertificate(hello)
}

// HTTPHandler returns a handler that serves ACME http-01 challenge responses,
// delegating non-challenge requests to fallback (nil → redirect-to-https). It
// must be reachable on port 80 for HTTP-01 to work.
func (c *Client) HTTPHandler(fallback http.Handler) http.Handler {
	return c.mgr.HTTPHandler(fallback)
}

// TLSConfig returns a *tls.Config wired to autocert's GetCertificate (with
// HTTP/2 and tls-alpn-01 NextProtos), for in-process HTTPS serving.
func (c *Client) TLSConfig() *tls.Config {
	return c.mgr.TLSConfig()
}
