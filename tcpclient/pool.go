package tcpclient

import (
	"context"
	"net"
	"sync"
	"time"
)

// poolConn wraps a pooled [net.Conn] with the time it was last returned to the
// pool, so the pool can evict idle connections older than IdleTimeout on
// checkout. The wrapped connection is owned by whichever goroutine holds the
// poolConn; it must be either returned via [connPool.put] or closed by the
// caller — never both.
type poolConn struct {
	net.Conn
	lastUsed time.Time
}

// connPool is a bounded, channel-backed connection pool for a single network
// address. The pool channel has capacity PoolSize; a non-blocking send on put
// therefore fails (and the connection is closed) when the pool is already full,
// which is exactly the "don't queue waiters" semantics we want — an overspill
// connection is used for one call and discarded rather than blocking the caller.
//
// The pool is safe for concurrent use by multiple goroutines: the channel is
// the synchroniser, and dial/close are connection-local. There is no separate
// health-check goroutine; idle connections past IdleTimeout are lazily evicted
// on checkout in get.
type connPool struct {
	network     string        // "tcp" | "unix" | ...
	address     string        // host:port or /path/to/socket
	dialTimeout time.Duration // applied to each new dial
	idleTimeout time.Duration // max age of an idle pooled conn

	pool chan *poolConn

	// mu guards the single Close of the pool drain. Once closed is set, get
	// stops returning pooled connections (it still dials, but callers should
	// not be using a closed client) and put closes incoming conns instead of
	// pooling them.
	closeOnce sync.Once
	closed    bool
	mu        sync.Mutex
}

// newConnPool constructs a pool for the given network/address with capacity
// size and the supplied dial/idle timeouts. A size <= 0 yields a pool that
// never retains a connection (every put closes), which keeps the client
// correct even with a degenerate config.
func newConnPool(network, address string, size int, dialTimeout, idleTimeout time.Duration) *connPool {
	if size < 0 {
		size = 0
	}
	return &connPool{
		network:     network,
		address:     address,
		dialTimeout: dialTimeout,
		idleTimeout: idleTimeout,
		pool:        make(chan *poolConn, size),
	}
}

// get returns a usable connection for the pool's address, reusing an idle one
// when available and otherwise dialling a new one. Idle connections older than
// the pool's IdleTimeout are closed (and skipped) on checkout, so a caller
// never receives a stale socket. The dial respects dialTimeout and ctx.
//
// The returned connection is the caller's responsibility: put it back with put
// when done, or close it directly on an unrecoverable error (in which case do
// NOT also put it).
func (p *connPool) get(ctx context.Context, dialTimeout time.Duration) (net.Conn, error) {
	// Lazily evict idle connections older than idleTimeout. We pull until the
	// channel is empty or we find a live one; expired ones are closed.
	for {
		select {
		case pc := <-p.pool:
			if p.expired(pc) {
				_ = pc.Close()
				continue
			}
			return pc.Conn, nil
		default:
		}
		break
	}
	// No idle connection: dial a fresh one. Use a fresh dialer per call so the
	// timeout is applied cleanly; net.Dialer is cheap to construct.
	d := net.Dialer{Timeout: dialTimeout}
	conn, err := d.DialContext(ctx, p.network, p.address)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// expired reports whether pc has been idle longer than the pool's IdleTimeout.
// A zero or negative IdleTimeout disables eviction (conns are kept indefinitely).
func (p *connPool) expired(pc *poolConn) bool {
	if p.idleTimeout <= 0 {
		return false
	}
	return time.Since(pc.lastUsed) > p.idleTimeout
}

// put returns conn to the pool for reuse if there is room and the pool is open;
// otherwise it closes the connection. It is safe to call with a conn that
// failed mid-call only if the caller has not already closed it — in that case
// pass a freshly-dialled replacement or skip put entirely. put never blocks.
//
// The closed-check and the channel send are both performed under mu so that
// close() cannot run in the window between them: without holding mu across the
// send, close() could drain the pool and mark it closed AFTER put observed
// closed==false but BEFORE the send, leaving conn stranded in the (now dead)
// channel forever — an FD leak. The non-blocking send on a buffered channel
// under a mutex does not meaningfully contend: the critical section is a single
// channel op, and close() is rare (client shutdown).
//
// To support detecting reuse in tests, conn is wrapped with the current time
// before being enqueued.
func (p *connPool) put(conn net.Conn) {
	if conn == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		_ = conn.Close()
		return
	}

	select {
	case p.pool <- &poolConn{Conn: conn, lastUsed: time.Now()}:
		// Returned to the pool; will be reused by a future get.
	default:
		// Pool full: close the overspill connection rather than block.
		_ = conn.Close()
	}
}

// close drains the pool and marks it closed so subsequent puts close their
// connections instead of returning them. It is idempotent (guarded by
// sync.Once). In-flight connections held by callers are unaffected; they will
// be closed by whoever holds them.
func (p *connPool) close() {
	p.closeOnce.Do(func() {
		p.mu.Lock()
		p.closed = true
		p.mu.Unlock()
		// Drain and close every pooled connection.
		for {
			select {
			case pc := <-p.pool:
				_ = pc.Close()
			default:
				return
			}
		}
	})
}
