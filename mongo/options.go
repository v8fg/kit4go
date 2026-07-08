package mongo

import (
	"time"

	"go.mongodb.org/mongo-driver/mongo/options"
)

// Options configures the mongo client. Zero-valued tuning fields defer to the
// driver's own defaults; only URI is required.
type Options struct {
	URI              string
	Database         string // default database (used by Collection when db == "")
	ConnectTimeout   time.Duration
	ServerSelTimeout time.Duration
	MaxPoolSize      uint64
	Username         string
	Password         string
	AuthSource       string
}

// Option configures Options.
type Option func(*Options)

// WithURI sets the connection URI (e.g. "mongodb://user:pass@host:27017/dbname").
// Required.
func WithURI(uri string) Option { return func(o *Options) { o.URI = uri } }

// WithDatabase sets the default database (used by Collection when db == "").
func WithDatabase(db string) Option { return func(o *Options) { o.Database = db } }

// WithConnectTimeout sets the connect timeout (0 -> driver default 30s).
func WithConnectTimeout(d time.Duration) Option { return func(o *Options) { o.ConnectTimeout = d } }

// WithServerSelectionTimeout sets the server-selection timeout (0 -> driver
// default 30s).
func WithServerSelectionTimeout(d time.Duration) Option {
	return func(o *Options) { o.ServerSelTimeout = d }
}

// WithMaxPoolSize sets the max connection-pool size (0 -> driver default 100).
func WithMaxPoolSize(n uint64) Option { return func(o *Options) { o.MaxPoolSize = n } }

// WithCredentials sets username/password (and auth source). Prefer putting these
// in the URI; this is for when the URI must stay credential-free.
func WithCredentials(username, password, authSource string) Option {
	return func(o *Options) { o.Username = username; o.Password = password; o.AuthSource = authSource }
}

// withDefaults applies the option chain. ConnectTimeout defaults to 10s so a
// forgotten timeout never hangs startup against a dead node.
func withDefaults(opts []Option) Options {
	var o Options
	for _, opt := range opts {
		opt(&o)
	}
	if o.ConnectTimeout == 0 {
		o.ConnectTimeout = 10 * time.Second
	}
	return o
}

// toDriver builds the driver's *options.ClientOptions from our Options.
func (o Options) toDriver() *options.ClientOptions {
	co := options.Client().ApplyURI(o.URI)
	if o.ConnectTimeout > 0 {
		co.SetConnectTimeout(o.ConnectTimeout)
	}
	if o.ServerSelTimeout > 0 {
		co.SetServerSelectionTimeout(o.ServerSelTimeout)
	}
	if o.MaxPoolSize > 0 {
		co.SetMaxPoolSize(o.MaxPoolSize)
	}
	if o.Username != "" || o.Password != "" {
		co.SetAuth(options.Credential{
			Username:   o.Username,
			Password:   o.Password,
			AuthSource: o.AuthSource,
		})
	}
	return co
}
