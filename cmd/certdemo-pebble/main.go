// Command certdemo-pebble issues a certificate against a local Pebble ACME
// server (an offline test CA), to exercise the cert package end-to-end without
// touching Let's Encrypt.
//
// Unlike Let's Encrypt, Pebble uses a self-signed root CA, so the ACME HTTPS
// calls must trust it. This driver builds an *http.Client rooted with Pebble's
// CA (via cert.Config.HTTPClient) and points cert.Config.DirectoryURL at Pebble.
//
// Run the whole stack (install + start Pebble + this driver) via:
//
//	bash cmd/certdemo/pebble-e2e.sh
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/v8fg/kit4go/cert"
)

func main() {
	domain := flag.String("domain", "certdemo.test", "domain to issue for (must resolve to 127.0.0.1, see pebble-e2e.sh)")
	dir := flag.String("dir", "./pebble-certs-out", "output directory for <domain>.crt/<domain>.key")
	directory := flag.String("directory", "https://127.0.0.1:14000/dir", "Pebble ACME directory URL")
	caPath := flag.String("ca", "", "path to Pebble root CA PEM (required, e.g. minica root cert)")
	addr := flag.String("addr", ":5002", "challenge server address (must match Pebble's httpPort)")
	flag.Parse()

	if *caPath == "" {
		log.Fatal("certdemo-pebble: -ca (Pebble root CA PEM) is required")
	}

	caPEM, err := os.ReadFile(*caPath)
	if err != nil {
		log.Fatalf("certdemo-pebble: read CA: %v", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		log.Fatal("certdemo-pebble: CA PEM did not add any certificates")
	}
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: pool}, // trust Pebble's self-signed CA
		},
	}

	mgr, err := cert.New(cert.Config{
		Domains:      []string{*domain},
		Dir:          *dir,
		DirectoryURL: *directory, // wins over Staging
		HTTPClient:   httpClient,
		RenewBefore:  720 * time.Hour,
	})
	if err != nil {
		log.Fatalf("certdemo-pebble: new: %v", err)
	}

	mgr.SetOnEvent(func(e cert.Event) {
		log.Printf("event=%s domain=%s err=%v", e.Name, e.Domain, e.Err)
	})

	// Challenge server on Pebble's httpPort (configured to 5002 by the script).
	httpSrv := &http.Server{
		Addr:              *addr,
		Handler:           mgr.HTTPHandler(nil),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		log.Printf("certdemo-pebble: challenge server on %s", *addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("certdemo-pebble: http: %v", err)
		}
	}()

	log.Printf("certdemo-pebble: obtaining %s from %s ...", *domain, *directory)
	if err := mgr.EnsureCert(context.Background(), *domain); err != nil {
		log.Fatalf("certdemo-pebble: obtain FAILED: %v", err)
	}
	m := mgr.Metrics()
	log.Printf("certdemo-pebble: OK issued=%d written=%d -> %s", m.Issued, m.Written, *dir)

	shutCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutCtx)
}
