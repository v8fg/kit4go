package cert

import (
	"crypto/tls"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// Let's Encrypt ACME directory URLs. Use the staging endpoint for first runs
// and tests to avoid hitting production rate limits.
const (
	LEProdDirectoryURL    = "https://acme-v02.api.letsencrypt.org/directory"
	LEStagingDirectoryURL = "https://acme-staging-v02.api.letsencrypt.org/directory"
)

// Key-type constants for [Config.KeyType].
const (
	// KeyTypeECDSA requests ECDSA P-256 certificates: smaller, modern, the
	// default.
	KeyTypeECDSA = "ecdsa"
	// KeyTypeRSA requests RSA 2048 certificates: widest legacy-client
	// compatibility.
	KeyTypeRSA = "rsa"
)

// Config configures a [Client]. Zero values are replaced with sensible defaults
// by withDefaults at construction time, but Domains and Dir are required and
// must be set explicitly — the zero Config is not usable on its own.
//
// Field tags carry both json and mapstructure names so the struct can be loaded
// from either a JSON config or a Viper-style mapstructure source.
type Config struct {
	// Domains is the allow-list of hostnames to issue certificates for. Each
	// becomes a SAN and is wired into autocert's HostPolicy, so only these
	// hosts are ever issued for (preventing rate-limit abuse via crafted SNI).
	// Required.
	Domains []string `json:"domains" mapstructure:"domains"`

	// Dir is the directory where <domain>.crt (chain) and <domain>.key (private
	// key, 0600) are atomically written. This is the primary output of the
	// package — point an external server (nginx, another process) at these
	// files, certbot-style. Required.
	Dir string `json:"dir" mapstructure:"dir"`

	// CacheDir is where autocert stores its internal state: the ACME account
	// key and the opaque certificate cache blobs. Defaults to
	// filepath.Join(Dir, ".acme"); it is separate from Dir so it never collides
	// with the split-file output. Contains private keys → keep 0700.
	CacheDir string `json:"cache_dir" mapstructure:"cache_dir"`

	// Email is the ACME account contact. Optional but recommended: CAs use it
	// to notify about certificate problems.
	Email string `json:"email" mapstructure:"email"`

	// Staging selects the Let's Encrypt staging directory. Strongly recommended
	// for first runs and tests to avoid production rate limits. Tri-state:
	// StagingSet records whether Staging was configured explicitly, so a
	// deliberate false is honoured rather than overwritten with the default.
	Staging    bool `json:"staging" mapstructure:"staging"`
	StagingSet bool `json:"-" mapstructure:"-"`

	// DirectoryURL overrides the ACME directory (e.g. a private ACME CA such as
	// Pebble for local testing). When non-empty it wins over Staging.
	DirectoryURL string `json:"directory_url" mapstructure:"directory_url"`

	// RenewBefore is forwarded to autocert's Manager.RenewBefore: autocert
	// renews a certificate when its remaining lifetime drops below this. Default
	// 30d (720h), matching autocert's own default for 90-day Let's Encrypt
	// certs. The renewal loop mirrors the renewed cert to disk on the next
	// [Config.CheckInterval] tick.
	RenewBefore time.Duration `json:"renew_before" mapstructure:"renew_before"`

	// CheckInterval is how often the renewal loop polls each domain and re-writes
	// the on-disk files when the certificate has been renewed. Default 1h.
	CheckInterval time.Duration `json:"check_interval" mapstructure:"check_interval"`

	// KeyType controls the public-key algorithm of issued certificates:
	// [KeyTypeECDSA] (default, P-256) or [KeyTypeRSA] (2048).
	KeyType string `json:"key_type" mapstructure:"key_type"`
}

// defaultConfig returns the package defaults used to fill zero config fields.
func defaultConfig() Config {
	return Config{
		RenewBefore:   720 * time.Hour, // 30d
		CheckInterval: time.Hour,
		KeyType:       KeyTypeECDSA,
	}
}

// withDefaults returns a copy of c with every zero field replaced by the
// corresponding default and CacheDir derived from Dir when unset. Non-zero
// fields are preserved, so callers can override only what they need.
func (c Config) withDefaults() Config {
	d := defaultConfig()
	if c.RenewBefore <= 0 {
		c.RenewBefore = d.RenewBefore
	}
	if c.CheckInterval <= 0 {
		c.CheckInterval = d.CheckInterval
	}
	if c.KeyType == "" {
		c.KeyType = d.KeyType
	}
	if c.CacheDir == "" && c.Dir != "" {
		c.CacheDir = filepath.Join(c.Dir, ".acme")
	}
	return c
}

// validate checks the config for required fields and coherent values, returning
// a sentinel error the caller can branch on with errors.Is.
func (c Config) validate() error {
	if len(c.Domains) == 0 {
		return ErrNoDomains
	}
	for _, d := range c.Domains {
		if !validDomain(d) {
			return fmt.Errorf("%w: %q", ErrInvalidDomain, d)
		}
	}
	if c.Dir == "" {
		return ErrNoDir
	}
	switch c.KeyType {
	case KeyTypeECDSA, KeyTypeRSA:
	default:
		return fmt.Errorf("%w: %q", ErrInvalidKeyType, c.KeyType)
	}
	return nil
}

// directoryURL resolves the ACME directory URL: an explicit DirectoryURL wins,
// then Staging, then production.
func (c Config) directoryURL() string {
	switch {
	case c.DirectoryURL != "":
		return c.DirectoryURL
	case c.Staging:
		return LEStagingDirectoryURL
	default:
		return LEProdDirectoryURL
	}
}

// validDomain reports whether s is a plausible hostname: non-empty, no spaces,
// slashes or ports, and containing at least one dot (autocert's GetCertificate
// rejects single-label names such as "localhost"). It is a guard rail, not a
// full RFC parser.
func validDomain(s string) bool {
	if s == "" || len(s) > 255 {
		return false
	}
	for _, r := range s {
		switch r {
		case ' ', '/', ':':
			return false
		}
	}
	return strings.Count(s, ".") >= 1
}

// clientHello builds the synthetic tls.ClientHelloInfo used to drive
// autocert.Manager.GetCertificate for a given domain, biased to the configured
// key type. autocert inspects hello.CipherSuites (and, when set,
// SignatureSchemes / SupportedCurves) via supportsECDSA to decide ECDSA vs RSA
// issuance, so we populate it to obtain the requested key type reliably.
func clientHello(domain, keyType string) *tls.ClientHelloInfo {
	hello := &tls.ClientHelloInfo{ServerName: domain}
	switch keyType {
	case KeyTypeRSA:
		hello.CipherSuites = []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256}
	default: // KeyTypeECDSA
		hello.CipherSuites = []uint16{tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256}
		hello.SupportedCurves = []tls.CurveID{tls.CurveP256}
		hello.SignatureSchemes = []tls.SignatureScheme{
			tls.ECDSAWithP256AndSHA256,
			tls.ECDSAWithP384AndSHA384,
			tls.ECDSAWithP521AndSHA512,
		}
	}
	return hello
}
