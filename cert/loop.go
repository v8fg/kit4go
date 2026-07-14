package cert

import (
	"context"
	"fmt"
	"time"
)

// Run runs the proactive renewal loop until ctx is cancelled, then returns
// ctx.Err(). On the first tick it issues (or loads) a certificate for every
// domain in [Config.Domains] and writes the split files to [Config.Dir]; on
// subsequent ticks it re-writes a domain's files only when autocert has renewed
// the certificate (the leaf NotAfter changed).
//
// The actual ACME renewal timing is owned by autocert (configured via
// [Config.RenewBefore], and warmed up on the first tick); Run only mirrors
// renewed certificates to disk and guarantees the first write happens without
// waiting for an inbound TLS handshake. Per-domain work is single-flight
// deduped against concurrent [Client.EnsureCert] calls.
//
// Each tick is panic-recovered (see [Client.runTick]): a panic in the ACME
// backend, the certificate parser, or the directory writer is recorded and
// reported via the OnPanic hook and the "panic" event, then swallowed so the
// loop keeps renewing — a transient panic must never kill renewal (which would
// leave certs to expire) or hang Stop.
//
// Run blocks; use [Client.Start] to run it in a goroutine.
func (c *Client) Run(ctx context.Context) error {
	// First tick immediately so a fresh process writes all certs without delay
	// and warms up autocert's per-domain renewal timers.
	c.runTick(ctx)
	if err := ctx.Err(); err != nil {
		return err
	}

	ticker := time.NewTicker(c.cfg.CheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			c.runTick(ctx)
		}
	}
}

// Start runs [Client.Run] in a goroutine and returns a stop function that
// cancels the loop and blocks until it has exited. The done channel is closed
// via defer, so stop always returns — even if Run panics on a code path not
// covered by runTick's recover.
func (c *Client) Start(ctx context.Context) (stop func()) {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done) // guarantees stop returns even if Run panics
		defer cancel()    // release the derived context if Run exits abnormally
		_ = c.Run(ctx)
	}()
	return func() {
		cancel()
		<-done
	}
}

// runTick runs one refresh pass with panic recovery. The renewal loop owns this
// goroutine, so per the kit4go callback convention ([Client.SetOnPanic]) a panic
// is recovered rather than allowed to kill the loop: it is counted, reported to
// the OnPanic hook and as a "panic" event, then swallowed so the next tick runs
// normally.
func (c *Client) runTick(ctx context.Context) {
	defer c.recoverTick()
	c.refreshAll(ctx)
}

// recoverTick is the deferred recover for the renewal loop.
func (c *Client) recoverTick() {
	if r := recover(); r != nil {
		c.panics.Add(1)
		c.fireEvent(Event{Name: EventPanic, Err: fmt.Errorf("cert: renewal loop panic: %v", r)})
		if p := c.onPanic.Load(); p != nil {
			(*p)(r)
		}
	}
}

// refreshAll walks Config.Domains and ensures each one, surfacing per-domain
// failures via the event hook and the failed counter rather than aborting the
// whole tick.
func (c *Client) refreshAll(ctx context.Context) {
	c.ticks.Add(1)
	for _, domain := range c.cfg.Domains {
		if err := ctx.Err(); err != nil {
			return
		}
		_ = c.EnsureCert(ctx, domain)
	}
}
