package elasticsearch

import (
	"net/http"

	"github.com/elastic/go-elasticsearch/v8"
)

// Options configures the elasticsearch client. Only Addresses (or CloudID) is
// required.
type Options struct {
	Addresses []string
	Username  string
	Password  string
	CloudID   string // Elastic Cloud (mutually exclusive with Addresses)
	CACert    []byte // PEM-encoded CA cert for TLS verification
	Transport http.RoundTripper
}

// Option configures Options.
type Option func(*Options)

// WithAddresses sets the cluster URLs (e.g. "http://localhost:9200"). Required
// unless CloudID is set. Copies the slice.
func WithAddresses(addrs ...string) Option {
	return func(o *Options) { o.Addresses = append([]string(nil), addrs...) }
}

// WithCredentials sets basic-auth username/password.
func WithCredentials(user, password string) Option {
	return func(o *Options) { o.Username = user; o.Password = password }
}

// WithCloudID connects to Elastic Cloud (mutually exclusive with Addresses).
func WithCloudID(id string) Option { return func(o *Options) { o.CloudID = id } }

// WithCACert sets a PEM-encoded CA cert for TLS verification.
func WithCACert(cert []byte) Option { return func(o *Options) { o.CACert = cert } }

// WithTransport sets a custom http.RoundTripper.
func WithTransport(t http.RoundTripper) Option { return func(o *Options) { o.Transport = t } }

// withDefaults applies the option chain.
func withDefaults(opts []Option) Options {
	var o Options
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// toDriver maps Options onto the driver's elasticsearch.Config.
func (o Options) toDriver() elasticsearch.Config {
	return elasticsearch.Config{
		Addresses: o.Addresses,
		Username:  o.Username,
		Password:  o.Password,
		CloudID:   o.CloudID,
		CACert:    o.CACert,
		Transport: o.Transport,
	}
}
