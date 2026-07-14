// Package lego provides a DNS-01 ACME backend for [github.com/v8fg/kit4go/cert]
// built on top of go-acme/lego. It satisfies [cert.ACMEManager], so it drops
// straight into [cert.NewWithManager] and reuses the same atomic directory
// writer, proactive renewal loop, metrics and events as the default autocert
// backend.
//
// DNS-01 is required for wildcard certificates (*.example.com) and for issuing
// without exposing a public port-80 HTTP server. autocert cannot do DNS-01;
// this backend is the reason DNS-01 is supported at all.
//
// lego is a large dependency, so this lives in its own module
// (github.com/v8fg/kit4go/cert/lego) — importing it does not pull lego (or its
// DNS-provider packages) into the core kit4go module. Callers pull in only the
// specific DNS provider(s) they use, e.g.:
//
//	import (
//	    "github.com/go-acme/lego/v4/providers/dns/cloudflare"
//	    legobackend "github.com/v8fg/kit4go/cert/lego"
//	)
//
//	cfg := dnscloudflare.NewDefaultConfig()
//	cfg.AuthToken = "..."
//	prov, _ := dnscloudflare.NewDNSProviderConfig(cfg)
//	backend, _ := legobackend.New(legobackend.Config{
//	    Email:       "ops@example.com",
//	    DNSProvider: prov,
//	})
//	mgr, _ := cert.NewWithManager(cert.Config{
//	    Domains: []string{"*.example.com", "example.com"},
//	    Dir:     "/etc/app/certs",
//	}, backend)
//	stop := mgr.Start(context.Background())
//	defer stop()
package lego

import (
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/registration"

	"github.com/v8fg/kit4go/cert"
)

// Compile-time check that Manager satisfies the cert backend interface.
var _ cert.ACMEManager = (*Manager)(nil)

// Config configures the lego DNS-01 backend.
type Config struct {
	// DirectoryURL is the ACME directory. Empty uses Let's Encrypt production
	// (lego's default). Point at Pebble for local testing.
	DirectoryURL string

	// HTTPClient, when non-nil, is used for ACME HTTPS calls (custom CA for
	// Pebble, egress proxy). nil → lego's default.
	HTTPClient *http.Client

	// Email is the ACME account contact.
	Email string

	// KeyType selects the certificate key algorithm. Defaults to ECDSA P-256.
	KeyType certcrypto.KeyType

	// DNSProvider solves DNS-01 challenges (e.g. a cloudflare/route53 provider
	// from lego/providers/dns). Required.
	DNSProvider challenge.Provider

	// RenewBefore: a cached certificate is re-obtained when its remaining
	// lifetime drops below this. Default 30d (720h).
	RenewBefore time.Duration
}

// Manager is a renewal-aware DNS-01 ACME backend implementing [cert.ACMEManager].
// It caches obtained certificates and only re-obtains when a certificate is
// within RenewBefore of expiry, so the cert renewal loop's change detection
// (leaf NotAfter) works correctly instead of re-issuing every tick.
type Manager struct {
	client      *lego.Client
	keyType     certcrypto.KeyType
	renewBefore time.Duration
	now         func() time.Time

	mu    sync.Mutex
	cache map[string]*cachedCert
}

type cachedCert struct {
	tls      *tls.Certificate
	notAfter time.Time
}

// New registers an ACME account and returns a ready Manager. Unlike the
// autocert backend (which is lazy), New contacts the CA to register, so a
// misconfigured DirectoryURL / DNSProvider fails here at construction.
func New(cfg Config) (*Manager, error) {
	if cfg.DNSProvider == nil {
		return nil, errors.New("cert/lego: DNSProvider is required for DNS-01")
	}
	if cfg.RenewBefore <= 0 {
		cfg.RenewBefore = 720 * time.Hour
	}
	keyType := cfg.KeyType
	if keyType == "" {
		keyType = certcrypto.EC256
	}

	key, err := certcrypto.GeneratePrivateKey(keyType)
	if err != nil {
		return nil, fmt.Errorf("cert/lego: generate account key: %w", err)
	}
	u := &user{email: cfg.Email, key: key}

	legoCfg := lego.NewConfig(u)
	if cfg.DirectoryURL != "" {
		legoCfg.CADirURL = cfg.DirectoryURL
	}
	legoCfg.Certificate.KeyType = keyType
	if cfg.HTTPClient != nil {
		legoCfg.HTTPClient = cfg.HTTPClient
	}

	client, err := lego.NewClient(legoCfg)
	if err != nil {
		return nil, fmt.Errorf("cert/lego: new client: %w", err)
	}
	if err := client.Challenge.SetDNS01Provider(cfg.DNSProvider); err != nil {
		return nil, fmt.Errorf("cert/lego: set dns-01 provider: %w", err)
	}
	reg, err := client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
	if err != nil {
		return nil, fmt.Errorf("cert/lego: register: %w", err)
	}
	u.reg = reg

	return &Manager{
		client:      client,
		keyType:     keyType,
		renewBefore: cfg.RenewBefore,
		now:         time.Now,
		cache:       make(map[string]*cachedCert),
	}, nil
}

// GetCertificate returns a cached certificate for hello.ServerName when it is
// still valid (remaining lifetime > RenewBefore), or obtains a new one via
// DNS-01. It is the single issuance entry point consumed by [cert.Client].
func (m *Manager) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	if hello == nil || hello.ServerName == "" {
		return nil, errors.New("cert/lego: missing server name")
	}
	domain := hello.ServerName

	m.mu.Lock()
	c := m.cache[domain]
	m.mu.Unlock()
	now := m.now()
	if c != nil && now.Add(m.renewBefore).Before(c.notAfter) {
		return c.tls, nil // cached and still valid
	}

	res, err := m.client.Certificate.Obtain(certificate.ObtainRequest{Domains: []string{domain}})
	if err != nil {
		return nil, fmt.Errorf("cert/lego: obtain %q: %w", domain, err)
	}
	// Build a full chain (leaf + issuer) for serving and for correct file output.
	chain := res.Certificate
	if len(res.IssuerCertificate) > 0 {
		chain = append(append([]byte{}, res.Certificate...), res.IssuerCertificate...)
	}
	tlsCert, err := tls.X509KeyPair(chain, res.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("cert/lego: parse certificate for %q: %w", domain, err)
	}
	if len(tlsCert.Certificate) > 0 {
		if leaf, perr := x509.ParseCertificate(tlsCert.Certificate[0]); perr == nil {
			tlsCert.Leaf = leaf
		}
	}

	out := &tlsCert
	notAfter := time.Time{}
	if out.Leaf != nil {
		notAfter = out.Leaf.NotAfter
	}
	m.mu.Lock()
	m.cache[domain] = &cachedCert{tls: out, notAfter: notAfter}
	m.mu.Unlock()
	return out, nil
}

// HTTPHandler is not applicable to DNS-01 (no HTTP challenge is served) and
// returns nil. It exists to satisfy [cert.ACMEManager].
func (*Manager) HTTPHandler(http.Handler) http.Handler { return nil }

// TLSConfig returns nil; DNS-01 issuance does not require a special TLS config.
// It exists to satisfy [cert.ACMEManager]. Use [cert.Client.GetCertificate]
// directly as tls.Config.GetCertificate if you serve TLS in-process.
func (*Manager) TLSConfig() *tls.Config { return nil }

// user implements registration.User for the lego account.
type user struct {
	email string
	key   crypto.PrivateKey
	reg   *registration.Resource
}

func (u *user) GetEmail() string                        { return u.email }
func (u *user) GetRegistration() *registration.Resource { return u.reg }
func (u *user) GetPrivateKey() crypto.PrivateKey        { return u.key }
