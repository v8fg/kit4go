package otp_test

import (
	"fmt"

	"github.com/v8fg/kit4go/otp"
)

// ExampleHOTPCode shows a full counter-based (HOTP) generate-and-verify
// round trip. HOTP is deterministic for a given secret+counter, so the codes
// below are stable and the example is verified by `go test`.
//
// Note: replay protection is the caller's job — the same (secret, counter)
// pair always yields the same code, so the counter must advance per attempt.
func ExampleHOTPCode() {
	const secret = "JBSWY3DPEHPK3PXP" // shared, base32, provisioned out-of-band

	// Server-side: emit a code for the current counter.
	code, err := otp.HOTPCode(secret, 1)
	if err != nil {
		fmt.Println("generate failed:", err)
		return
	}
	fmt.Println(code)

	// Verify the code the user submits for that counter.
	fmt.Println(otp.VerifyHOTP(code, 1, secret))

	// Next attempt must use counter 2 — a fresh code, still constant-time
	// compared.
	next, _ := otp.HOTPCode(secret, 2)
	fmt.Println(next)
	// Output:
	// 996554
	// true
	// 602287
}

// ExampleGenerateURLTOTP shows provisioning a TOTP authenticator: the server
// generates an otpauth:// URL the user scans as a QR code. Secret is supplied
// here for a deterministic URL; in real code leave KeyOpts.Secret nil to draw
// a cryptographically random secret (20 bytes by default).
func ExampleGenerateURLTOTP() {
	url, err := otp.GenerateURLTOTP(otp.KeyOpts{
		Issuer:      "Example",
		AccountName: "alice@google.com",
		Secret:      []byte("12345678901234567890"), // fixed here for stable output
	})
	if err != nil {
		fmt.Println("provision failed:", err)
		return
	}
	fmt.Println(url)
	// Output:
	// otpauth://totp/Example:alice@google.com?issuer=Example&secret=GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ
}
