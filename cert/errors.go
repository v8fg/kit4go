package cert

import "errors"

// Sentinel errors returned by the cert package. The config-validation errors
// below are returned directly so callers can branch on them with errors.Is;
// runtime (obtain / write) failures are wrapped with a "cert: ..." string
// prefix and %w around their underlying cause, mirroring the httpclient
// package. Runtime outcomes are also surfaced via the event hook
// ([Client.SetOnEvent]) for observability.
var (
	// ErrNoDomains is returned by [New] when [Config.Domains] is empty.
	ErrNoDomains = errors.New("cert: at least one domain is required")

	// ErrNoDir is returned by [New] when [Config.Dir] is empty.
	ErrNoDir = errors.New("cert: output directory is required")

	// ErrInvalidDomain is returned by [New] when a domain is not a plausible
	// hostname (empty, too long, or single-label). The offending value is
	// appended via fmt.Errorf("%w: %q", ...).
	ErrInvalidDomain = errors.New("cert: invalid domain")

	// ErrInvalidKeyType is returned by [New] when [Config.KeyType] is neither
	// "ecdsa" nor "rsa".
	ErrInvalidKeyType = errors.New("cert: key type must be ecdsa or rsa")
)
