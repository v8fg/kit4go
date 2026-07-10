// Package postgres is a thin pool wrapper around pgx (pure Go, cross-platform,
// no C toolchain required). It provides a bounded connection pool with sane
// defaults and is safe for concurrent use.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Options configures the postgres pool.
type Options struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string // disable|require|verify-ca|verify-full; empty -> disable

	MaxConns        int           // <=0 -> 10
	MinConns        int           // <=0 -> 2
	MaxConnLifetime time.Duration // <=0 -> 30m
	MaxConnIdleTime time.Duration // <=0 -> 5m
	ConnectTimeout  time.Duration // <=0 -> 5s
}

// PoolConn is the subset of *pgxpool.Pool that Client uses internally.
// *pgxpool.Pool satisfies this interface; tests can inject a mock.
type PoolConn interface {
	Ping(context.Context) error
	Close()
}

// Client wraps a pgx connection pool.
type Client struct {
	pool    PoolConn
	rawPool *pgxpool.Pool // non-nil in production; nil when mock-injected
}

// New creates a pool and pings it.
func New(ctx context.Context, opts Options) (*Client, error) {
	if opts.Host == "" {
		return nil, errors.New("postgres: empty host")
	}
	if opts.DBName == "" {
		return nil, errors.New("postgres: empty db name")
	}
	ssl := opts.SSLMode
	if ssl == "" {
		ssl = "disable"
	}
	// Escape the userinfo components (user, password) per RFC 3986: they
	// live in the authority's userinfo section, where url.PathEscape is the
	// correct encoder. Raw interpolation would let a password containing
	// URL-special chars (@ : / # % or space) silently misparse — e.g. an
	// embedded @ splits userinfo and rebinds the host, which is the classic
	// RDS/Azure managed-PG credential injection footgun. PathEscape (not
	// QueryEscape) is the precise choice for the userinfo component.
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		url.PathEscape(opts.User), url.PathEscape(opts.Password),
		opts.Host, opts.Port, opts.DBName, ssl)

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	if opts.MaxConns > 0 {
		cfg.MaxConns = int32(opts.MaxConns)
	} else {
		cfg.MaxConns = 10
	}
	if opts.MinConns > 0 {
		cfg.MinConns = int32(opts.MinConns)
	} else {
		cfg.MinConns = 2
	}
	if opts.MaxConnLifetime > 0 {
		cfg.MaxConnLifetime = opts.MaxConnLifetime
	} else {
		cfg.MaxConnLifetime = 30 * time.Minute
	}
	if opts.MaxConnIdleTime > 0 {
		cfg.MaxConnIdleTime = opts.MaxConnIdleTime
	} else {
		cfg.MaxConnIdleTime = 5 * time.Minute
	}
	if opts.ConnectTimeout > 0 {
		cfg.ConnConfig.ConnectTimeout = opts.ConnectTimeout
	} else {
		cfg.ConnConfig.ConnectTimeout = 5 * time.Second
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Client{pool: pool, rawPool: pool}, nil
}

// Pool returns the underlying pgx pool for advanced use (queries, transactions).
// Returns nil if the Client was created with a mock pool (testing only).
func (c *Client) Pool() *pgxpool.Pool { return c.rawPool }

// Ping checks connectivity.
func (c *Client) Ping(ctx context.Context) error { return c.pool.Ping(ctx) }

// Close releases all connections.
func (c *Client) Close() { c.pool.Close() }

// newWithPool builds a Client from an injected PoolConn (for testing).
func newWithPool(pool PoolConn) *Client { return &Client{pool: pool} }
