package cert_test

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/v8fg/kit4go/cert"
)

// ExampleNew constructs a certificate manager that will issue, renew and write
// HTTPS certificates to Dir. Construction does not contact the ACME server —
// issuance happens on the first loop tick or EnsureCert call.
func ExampleNew() {
	mgr, err := cert.New(cert.Config{
		Domains: []string{"example.com"},
		Dir:     filepath.Join(os.TempDir(), "kit4go-cert-example"),
		Email:   "ops@example.com",
		Staging: true, // Let's Encrypt staging — use until verified
	})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	_ = mgr
	fmt.Println("client ready")

	// Output: client ready
}

// ExampleClient_HTTPHandler shows mounting the ACME http-01 challenge handler
// on a port-80 server, which is required for HTTP-01 domain validation.
func ExampleClient_HTTPHandler() {
	mgr, _ := cert.New(cert.Config{
		Domains: []string{"example.com"},
		Dir:     filepath.Join(os.TempDir(), "kit4go-cert-example2"),
		Staging: true,
	})
	http.Handle("/", mgr.HTTPHandler(nil)) // nil fallback → redirect to HTTPS
	fmt.Println("handler mounted")

	// Output: handler mounted
}
