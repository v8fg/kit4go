package cert

import (
	"crypto/tls"
	"net/http"

	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

// ACMEManager is the subset of *autocert.Manager the cert package consumes. The
// real implementation (acmeManagerAdapter) wraps autocert; tests inject a
// mockery mock so no network or Let's Encrypt call ever happens during tests.
// A future DNS-01 / lego backend can satisfy this interface without changes to
// [Client] or the renewal loop.
//
//go:generate mockery --name ACMEManager --inpackage --with-expecter --filename mock_ACMEManager.go
type ACMEManager interface {
	// GetCertificate obtains or loads a certificate for hello.ServerName. It is
	// the single issuance entry point — autocert's internal certKey is
	// unexported, so the public path is the only one usable. autocert caps each
	// call at a 5-minute internal timeout regardless of the caller's context.
	GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error)
	// HTTPHandler returns a handler that serves ACME http-01 challenge
	// responses; it must be reachable on port 80 for HTTP-01 to work.
	HTTPHandler(fallback http.Handler) http.Handler
	// TLSConfig returns a *tls.Config wired to GetCertificate, for in-process
	// HTTPS serving (secondary mode).
	TLSConfig() *tls.Config
}

// acmeManagerAdapter adapts *autocert.Manager to the [ACMEManager] interface.
type acmeManagerAdapter struct {
	m *autocert.Manager
}

func (a *acmeManagerAdapter) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	return a.m.GetCertificate(hello)
}

func (a *acmeManagerAdapter) HTTPHandler(fallback http.Handler) http.Handler {
	return a.m.HTTPHandler(fallback)
}

func (a *acmeManagerAdapter) TLSConfig() *tls.Config {
	return a.m.TLSConfig()
}

// newAutocertManager builds an *autocert.Manager configured from cfg: HostPolicy
// locked to the configured domains, AcceptTOS, a DirCache rooted at CacheDir,
// the requested ACME directory URL and the configured renewal window.
func newAutocertManager(cfg Config) *autocert.Manager {
	return &autocert.Manager{
		Prompt:      autocert.AcceptTOS,
		HostPolicy:  autocert.HostWhitelist(cfg.Domains...),
		Cache:       autocert.DirCache(cfg.CacheDir),
		Email:       cfg.Email,
		RenewBefore: cfg.RenewBefore,
		Client:      &acme.Client{DirectoryURL: cfg.directoryURL()},
	}
}
