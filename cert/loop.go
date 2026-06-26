package cert

import (
	"context"
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
// Run blocks; use [Client.Start] to run it in a goroutine.
func (c *Client) Run(ctx context.Context) error {
	// First tick immediately so a fresh process writes all certs without delay
	// and warms up autocert's per-domain renewal timers.
	c.refreshAll(ctx)
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
			c.refreshAll(ctx)
		}
	}
}

// Start runs [Client.Run] in a goroutine and returns a stop function that
// cancels the loop and blocks until it has exited.
func (c *Client) Start(ctx context.Context) (stop func()) {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		_ = c.Run(ctx)
		close(done)
	}()
	return func() {
		cancel()
		<-done
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
