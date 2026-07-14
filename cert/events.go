package cert

import (
	"crypto/x509"
	"time"
)

// Info is a compact description of a certificate, attached to [Event] for
// issue, renew and write events. It is derived from the leaf certificate.
type Info struct {
	Domain    string
	SANs      []string
	Issuer    string
	NotBefore time.Time
	NotAfter  time.Time
}

// certInfo builds an [Info] from a leaf certificate. Returns nil if leaf is
// nil, so skip/error events carry no cert payload.
func certInfo(domain string, leaf *x509.Certificate) *Info {
	if leaf == nil {
		return nil
	}
	return &Info{
		Domain:    domain,
		SANs:      append([]string(nil), leaf.DNSNames...),
		Issuer:    leaf.Issuer.String(),
		NotBefore: leaf.NotBefore,
		NotAfter:  leaf.NotAfter,
	}
}

// Event is passed to the hook installed via [Client.SetOnEvent] for every
// notable outcome of issuance, renewal and writing. It is the integration point
// for metrics push and alerting, mirroring the hook pattern used by the
// httpclient package.
//
// Name is one of:
//   - "issue": a certificate was written for a domain that had none on disk yet.
//   - "renew": the on-disk certificate for a domain was replaced by a renewed one.
//   - "write": the cert+key files for a domain were (re)written to [Config.Dir].
//   - "skip":  a loop tick left the on-disk cert unchanged (not yet due).
//   - "error": an obtain or write attempt failed; Err is non-nil.
//   - "panic": the renewal loop recovered a panic; Err carries the panic value.
//     The loop keeps running — this signals a bug to investigate (see OnPanic).
//
// Domain is the hostname the event pertains to. Cert is non-nil for issue,
// renew and write; nil for skip, error and panic. Err is non-nil only for
// "error" and "panic".
type Event struct {
	Name   string
	Domain string
	Cert   *Info
	Err    error
}

// Event name constants used by [Event.Name].
const (
	EventIssue = "issue"
	EventRenew = "renew"
	EventWrite = "write"
	EventSkip  = "skip"
	EventError = "error"
	EventPanic = "panic"
)
