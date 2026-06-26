// Command certdemo issues and renews an HTTPS certificate via Let's Encrypt
// using the kit4go [cert] package, writing the cert+key to a directory.
//
// It demonstrates the primary (directory-writer) mode: an HTTP server on port
// 80 serves the ACME http-01 challenge, the renewal loop issues on the first
// tick and keeps the files renewed before expiry.
//
// # Prerequisites (all required for real issuance)
//
//   - the domain's A/AAAA record must point at this machine;
//   - port 80 must be reachable from the public internet (http-01 is validated
//     on :80 — binding it needs root, e.g. run via sudo or grant cap_net_bind);
//   - keep -staging true until issuance is verified, to avoid LE production rate
//     limits (5 failed validations per account per hour, 50 certs per domain per
//     week).
//
// # Usage (real run)
//
//	sudo go run ./cmd/certdemo \
//	    -domain example.com \
//	    -dir /etc/certdemo/certs \
//	    -email you@example.com \
//	    -addr :80
//
// Add -once to attempt a single issuance and exit (handy for smoke testing).
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/v8fg/kit4go/cert"
)

func main() {
	domain := flag.String("domain", "", "domain to issue a certificate for (required)")
	dir := flag.String("dir", "./certs-out", "directory to write <domain>.crt and <domain>.key")
	email := flag.String("email", "", "ACME account contact (recommended)")
	addr := flag.String("addr", ":80", "address for the http-01 challenge server (LE validates :80)")
	staging := flag.Bool("staging", true, "use Let's Encrypt staging (leave true until verified)")
	once := flag.Bool("once", false, "attempt a single issuance and exit")
	flag.Parse()

	if *domain == "" {
		log.Fatal("certdemo: -domain is required")
	}

	mgr, err := cert.New(cert.Config{
		Domains: []string{*domain},
		Dir:     *dir,
		Email:   *email,
		Staging: *staging,
	})
	if err != nil {
		log.Fatalf("certdemo: new: %v", err)
	}

	mgr.SetOnEvent(func(e cert.Event) {
		switch e.Name {
		case cert.EventIssue, cert.EventRenew:
			log.Printf("event=%s domain=%s not_after=%s", e.Name, e.Domain, e.Cert.NotAfter.Format(time.RFC3339))
		case cert.EventWrite:
			log.Printf("event=write domain=%s -> %s/%s.{crt,key}", e.Domain, *dir, e.Domain)
		case cert.EventError:
			log.Printf("event=error domain=%s err=%v", e.Domain, e.Err)
		}
	})

	// Port-80 server: answers ACME http-01 challenges, redirects the rest to HTTPS.
	// ReadHeaderTimeout bounds slowloris-style clients holding connections open.
	httpSrv := &http.Server{
		Addr:              *addr,
		Handler:           mgr.HTTPHandler(nil),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		log.Printf("certdemo: http-01 challenge server listening on %s", *addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("certdemo: http server: %v", err)
		}
	}()

	if *once {
		log.Printf("certdemo: once: attempting issuance for %s (autocert caps each attempt at 5m)...", *domain)
		err := mgr.EnsureCert(context.Background(), *domain)
		shutdown(httpSrv)
		if err != nil {
			log.Fatalf("certdemo: once: FAILED: %v", err)
		}
		m := mgr.Metrics()
		log.Printf("certdemo: once: OK issued=%d written=%d — see %s", m.Issued, m.Written, *dir)
		return
	}

	// Long-running mode: proactive renewal loop until SIGINT/SIGTERM.
	mgrStop := mgr.Start(context.Background())
	log.Printf("certdemo: renewal loop running; write/issue/renew events will appear above. Ctrl-C to stop.")

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Printf("certdemo: shutting down")
	mgrStop()
	shutdown(httpSrv)
}

func shutdown(httpSrv *http.Server) {
	shutCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutCtx)
}
