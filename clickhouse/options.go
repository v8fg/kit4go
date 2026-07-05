package clickhouse

import (
	"crypto/tls"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
)

// Protocol selects the wire protocol. The zero value ProtocolNative is the
// high-performance default (columnar TCP, port 9000 / TLS 9440). Use
// ProtocolHTTP behind a reverse proxy or load balancer (port 8123 / TLS 8443).
// The wrapper does NOT remap ports — pass the matching port in WithAddrs.
type Protocol int

const (
	ProtocolNative Protocol = iota // default -> clickhouse.Native (TCP :9000)
	ProtocolHTTP                   //          -> clickhouse.HTTP  (:8123)
)

// Options configures the clickhouse client. Zero-valued tuning fields defer to
// clickhouse-go's own defaults (DialTimeout 30s, MaxIdleConns 5,
// MaxOpenConns MaxIdleConns+5, ConnMaxLifetime 1h); Database defaults to
// "default".
type Options struct {
	Addrs            []string
	Protocol         Protocol
	Database         string
	Username         string
	Password         string
	TLSConfig        *tls.Config
	DialTimeout      time.Duration
	MaxOpenConns     int
	MaxIdleConns     int
	ConnMaxLifetime  time.Duration
	Settings         clickhouse.Settings
	Compression      *clickhouse.Compression
	ConnOpenStrategy clickhouse.ConnOpenStrategy
	Debug            bool
}

// Option configures Options.
type Option func(*Options)

// WithAddrs sets the ClickHouse node addresses (host:port). Use :9000 for the
// native protocol, :8123 for HTTP.
func WithAddrs(addrs ...string) Option {
	return func(o *Options) { o.Addrs = addrs }
}

// WithProtocol sets the wire protocol (default ProtocolNative).
func WithProtocol(p Protocol) Option { return func(o *Options) { o.Protocol = p } }

// WithDatabase sets the database (default "default").
func WithDatabase(db string) Option { return func(o *Options) { o.Database = db } }

// WithUsername sets the auth username.
func WithUsername(u string) Option { return func(o *Options) { o.Username = u } }

// WithPassword sets the auth password.
func WithPassword(p string) Option { return func(o *Options) { o.Password = p } }

// WithTLSConfig enables TLS. nil leaves the connection plaintext.
func WithTLSConfig(c *tls.Config) Option { return func(o *Options) { o.TLSConfig = c } }

// WithDialTimeout sets the dial timeout (0 -> driver default 30s).
func WithDialTimeout(d time.Duration) Option { return func(o *Options) { o.DialTimeout = d } }

// WithMaxOpenConns sets the max open connections (0 -> driver default).
func WithMaxOpenConns(n int) Option { return func(o *Options) { o.MaxOpenConns = n } }

// WithMaxIdleConns sets the max idle connections (0 -> driver default 5).
func WithMaxIdleConns(n int) Option { return func(o *Options) { o.MaxIdleConns = n } }

// WithConnMaxLifetime sets the max connection lifetime (0 -> driver default 1h).
func WithConnMaxLifetime(d time.Duration) Option {
	return func(o *Options) { o.ConnMaxLifetime = d }
}

// WithSettings passes clickhouse query settings through (e.g. max_execution_time).
func WithSettings(s clickhouse.Settings) Option {
	return func(o *Options) { o.Settings = s }
}

// WithCompression sets wire compression. nil = none (default). For OLAP traffic,
// LZ4 is recommended in production: WithCompression(&clickhouse.Compression{
//
//	Method: clickhouse.CompressionLZ4}).
func WithCompression(c *clickhouse.Compression) Option {
	return func(o *Options) { o.Compression = c }
}

// WithConnOpenStrategy sets how addresses are picked (default ConnOpenInOrder;
// for multi-node deployments ConnOpenRoundRobin is usually better).
func WithConnOpenStrategy(s clickhouse.ConnOpenStrategy) Option {
	return func(o *Options) { o.ConnOpenStrategy = s }
}

// WithDebug enables the driver's legacy debug logging. Deprecated upstream in
// favor of slog via the driver's Logger field (not exposed here — wire your own
// slog.Logger directly through clickhouse.Open if needed).
func WithDebug(b bool) Option { return func(o *Options) { o.Debug = b } }

// withDefaults applies the option chain and fills zero fields with package
// defaults. Protocol's zero value is already ProtocolNative; Database -> "default".
func withDefaults(opts []Option) Options {
	var o Options
	for _, opt := range opts {
		opt(&o)
	}
	if o.Database == "" {
		o.Database = "default"
	}
	return o
}

// toDriver maps Options onto clickhouse-go's Options. Zero-valued tuning fields
// are forwarded as-is so the driver applies its own defaults.
func (o Options) toDriver() *clickhouse.Options {
	return &clickhouse.Options{
		Protocol:         toDriverProtocol(o.Protocol),
		Addr:             o.Addrs,
		Auth:             clickhouse.Auth{Database: o.Database, Username: o.Username, Password: o.Password},
		TLS:              o.TLSConfig,
		DialTimeout:      o.DialTimeout,
		MaxOpenConns:     o.MaxOpenConns,
		MaxIdleConns:     o.MaxIdleConns,
		ConnMaxLifetime:  o.ConnMaxLifetime,
		ConnOpenStrategy: o.ConnOpenStrategy,
		Settings:         o.Settings,
		Compression:      o.Compression,
		Debug:            o.Debug,
	}
}

func toDriverProtocol(p Protocol) clickhouse.Protocol {
	if p == ProtocolHTTP {
		return clickhouse.HTTP
	}
	return clickhouse.Native
}
