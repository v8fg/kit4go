package minio

import (
	"net/http"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// BucketLookup selects how bucket names are encoded into the request URL.
// BucketLookupAuto (the default) lets minio-go pick; use BucketLookupPath behind
// a reverse proxy or with non-DNS-compliant bucket names, BucketLookupDNS for
// virtual-host-style (https://<bucket>.endpoint).
type BucketLookup int

// BucketLookupAuto defers the style to minio-go; BucketLookupDNS uses
// virtual-host-style URLs; BucketLookupPath uses path-style URLs.
const (
	BucketLookupAuto BucketLookup = iota // default -> minio.BucketLookupAuto
	BucketLookupDNS                      //          -> minio.BucketLookupDNS
	BucketLookupPath                     //          -> minio.BucketLookupPath
)

// Options configures the minio client. Zero-valued fields defer to minio-go's
// own defaults; Secure defaults to true (see withDefaults) for production
// safety.
type Options struct {
	Endpoint     string
	AccessKey    string
	SecretKey    string
	Secure       bool
	Region       string
	BucketLookup BucketLookup
	Transport    http.RoundTripper
}

// Option configures Options.
type Option func(*Options)

// WithEndpoint sets the MinIO/S3 endpoint (host:port, e.g. "s3.amazonaws.com"
// or "minio.local:9000"). Required.
func WithEndpoint(endpoint string) Option { return func(o *Options) { o.Endpoint = endpoint } }

// WithCredentials sets the access/secret key.
func WithCredentials(accessKey, secretKey string) Option {
	return func(o *Options) { o.AccessKey = accessKey; o.SecretKey = secretKey }
}

// WithSecure toggles HTTPS (default true). Set false only for local plaintext
// MinIO dev servers.
func WithSecure(secure bool) Option { return func(o *Options) { o.Secure = secure } }

// WithRegion sets the AWS region (required for some S3 endpoints; auto-detected
// for others).
func WithRegion(region string) Option { return func(o *Options) { o.Region = region } }

// WithBucketLookup sets the URL style (default BucketLookupAuto).
func WithBucketLookup(b BucketLookup) Option { return func(o *Options) { o.BucketLookup = b } }

// WithTransport sets a custom http.RoundTripper. If set, the caller owns it and
// must close/release it; the wrapper has no Close (minio.Client is stateless).
func WithTransport(t http.RoundTripper) Option { return func(o *Options) { o.Transport = t } }

// withDefaults applies the option chain and fills defaults. Secure defaults to
// true: it is set BEFORE the option chain runs, so WithSecure(false) still wins.
// HTTPS-by-default means a forgotten flag never ships plaintext credentials.
func withDefaults(opts []Option) Options {
	o := Options{Secure: true}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// toDriver maps Options onto minio-go's *minio.Options (endpoint is passed
// separately to minio.New — it is not a field of minio.Options).
func (o Options) toDriver() *minio.Options {
	return &minio.Options{
		Creds:        credentials.NewStaticV4(o.AccessKey, o.SecretKey, ""),
		Secure:       o.Secure,
		Region:       o.Region,
		BucketLookup: toDriverBucketLookup(o.BucketLookup),
		Transport:    o.Transport,
	}
}

func toDriverBucketLookup(b BucketLookup) minio.BucketLookupType {
	switch b {
	case BucketLookupDNS:
		return minio.BucketLookupDNS
	case BucketLookupPath:
		return minio.BucketLookupPath
	default:
		return minio.BucketLookupAuto
	}
}
