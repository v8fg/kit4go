package otp

import (
	"crypto/rand"
	"encoding/base32"
	"errors"
	"io"
	"net/url"
	"strconv"
	"strings"

	xtp "github.com/pquerna/otp"
	"github.com/pquerna/otp/hotp"
	"github.com/pquerna/otp/totp"
)

const (
	// AlgorithmSHA1 should be used for compatibility with Google Authenticator.
	//
	// See https://github.com/pquerna/otp/issues/55 for additional details.
	AlgorithmSHA1   = xtp.AlgorithmSHA1
	AlgorithmSHA256 = xtp.AlgorithmSHA256
	AlgorithmSHA512 = xtp.AlgorithmSHA512
	AlgorithmMD5    = xtp.AlgorithmMD5
)

var b32NoPadding = base32.StdEncoding.WithPadding(base32.NoPadding)

// KeyOpts provides options for Generate().  The default values
// are compatible with Google-Authenticator.
//
// Required: Issuer, AccountName, htop also need counter.
type KeyOpts struct {
	// Name of the issuing Organization/Company.
	Issuer string
	// Name of the User's Account (eg, email address)
	AccountName string
	// Number of seconds a TOTP hash is valid for. Defaults to 30 seconds.
	Period uint
	// Size in size of the generated Secret. Defaults to 20 bytes.
	SecretSize uint
	// Secret to store. Defaults to a randomly generated secret of SecretSize.  You should generally leave this empty.
	Secret []byte
	// Digits to request. Defaults to 6.
	Digits xtp.Digits
	// Algorithm to use for HMAC. Defaults to SHA1.
	Algorithm xtp.Algorithm
	// Reader to use for generating TOTP Key.
	Rand io.Reader
	// Counter for HOTP. if type is hotp: The counter parameter is required when provisioning a key for use with HOTP. It will set the initial counter value.
	Counter uint64
}

// RandomSecret generates a random secret of given length (number of bytes) without padding,
// if rand.Read failed returns empty string.
func RandomSecret(length int) (secret string) {
	secretB := make([]byte, length)
	gen, err := rand.Read(secretB)
	if err != nil || gen != length {
		return secret
	}
	secret = b32NoPadding.EncodeToString(secretB)
	return
}

// VerifySecret verifies the secret is valid, support padding or NoPadding format.
func VerifySecret(secret string) bool {
	secret = strings.TrimSpace(secret)
	if n := len(secret) % 8; n != 0 {
		secret = secret + strings.Repeat("=", 8-n)
	}
	_, err := base32.StdEncoding.DecodeString(secret)
	return err == nil
}

// GenerateURLHOTP returns the HOTP URL as a string.
func GenerateURLHOTP(opts KeyOpts) (url string) {
	if key, err := simpleURL(opts, "hotp"); err == nil {
		key.Type()
		url = key.URL()
	}
	return
}

// GenerateURLTOTP returns the TOTP URL as a string.
func GenerateURLTOTP(opts KeyOpts) (url string) {
	if key, err := simpleURL(opts, "totp"); err == nil {
		url = key.URL()
	}
	return
}

// KeyFromTOTPOpts creates a new TOTP Key.
func KeyFromTOTPOpts(opts KeyOpts) (*xtp.Key, error) {
	return totp.Generate(totp.GenerateOpts{
		Issuer:      opts.Issuer,
		AccountName: opts.AccountName,
		Period:      opts.Period,
		Secret:      opts.Secret,
		SecretSize:  opts.SecretSize,
		Digits:      opts.Digits,
		Algorithm:   opts.Algorithm,
		Rand:        opts.Rand,
	})
}

// KeyFromHOTPOpts creates a new HOTP Key.
func KeyFromHOTPOpts(opts KeyOpts) (*xtp.Key, error) {
	return hotp.Generate(hotp.GenerateOpts{
		Issuer:      opts.Issuer,
		AccountName: opts.AccountName,
		Secret:      opts.Secret,
		SecretSize:  opts.SecretSize,
		Digits:      opts.Digits,
		Algorithm:   opts.Algorithm,
		Rand:        opts.Rand,
	})
}

// KeyFromURL creates a new Key from an TOTP or HOTP url.
//
// The URL format is documented here:
//
//	https://github.com/google/google-authenticator/wiki/Key-Uri-Format
func KeyFromURL(url string) (*xtp.Key, error) {
	if len(url) == 0 {
		return nil, errors.New("empty URL")
	}
	return xtp.NewKeyFromURL(url)
}

func simpleURL(opts KeyOpts, otpType string) (*xtp.Key, error) {
	// url encode the Issuer/AccountName
	if opts.Issuer == "" {
		return nil, xtp.ErrGenerateMissingIssuer
	}

	if opts.AccountName == "" {
		return nil, xtp.ErrGenerateMissingAccountName
	}

	if opts.SecretSize == 0 {
		opts.SecretSize = 10
	}

	if opts.Rand == nil {
		opts.Rand = rand.Reader
	}

	// otpauth://totp/Example:alice@google.com?secret=JBSWY3DPEHPK3PXP&issuer=Example

	v := url.Values{}
	if len(opts.Secret) != 0 {
		v.Set("secret", b32NoPadding.EncodeToString(opts.Secret))
	} else {
		secret := make([]byte, opts.SecretSize)
		_, _ = opts.Rand.Read(secret)
		v.Set("secret", b32NoPadding.EncodeToString(secret))
	}

	v.Set("issuer", opts.Issuer)
	if opts.Digits == 0 {
		opts.Digits = xtp.DigitsSix
	} else {
		v.Set("digits", opts.Digits.String())
	}
	if opts.Algorithm != xtp.AlgorithmSHA1 {
		v.Set("algorithm", opts.Algorithm.String())
	}

	if otpType == "hotp" {
		v.Set("counter", strconv.FormatUint(opts.Counter, 10))
	}

	u := url.URL{
		Scheme:   "otpauth",
		Host:     otpType,
		Path:     "/" + opts.Issuer + ":" + opts.AccountName,
		RawQuery: v.Encode(),
	}

	return xtp.NewKeyFromURL(u.String())
}
