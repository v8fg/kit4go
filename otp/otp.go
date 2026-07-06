package otp

import (
	"time"

	xtp "github.com/pquerna/otp"
	"github.com/pquerna/otp/hotp"
	"github.com/pquerna/otp/totp"
)

// Opts provides options for ValidateCustom().
//
//	Only for TOTP: Period, Skew.
type Opts struct {
	// Number of seconds a TOTP hash is valid for. Defaults to 30 seconds.
	Period uint
	// Periods before or after the current time to allow.  Value of 1 allows up to Period
	// of either side of the specified time.  Defaults to 0 allowed skews.  Values greater
	// than 1 are likely sketchy.
	Skew uint
	// Digits as part of the input. Defaults to 6.
	Digits xtp.Digits
	// Algorithm to use for HMAC. Defaults to SHA1.
	Algorithm xtp.Algorithm
}

func (opts *Opts) GetPeriod() uint {
	if opts == nil || opts.Period == 0 {
		return 30
	}
	return opts.Period
}

func (opts *Opts) GetSkew() uint {
	if opts == nil {
		return 0
	}
	return opts.Skew
}

func (opts *Opts) GetDigits() xtp.Digits {
	if opts == nil || opts.Digits == 0 {
		return xtp.DigitsSix
	}
	return opts.Digits
}

func (opts *Opts) GetAlgorithm() xtp.Algorithm {
	if opts == nil {
		return xtp.AlgorithmSHA1
	}
	return opts.Algorithm
}

// Code generates the totp code, with the default settings: digits=6, algorithm=SHA1, base now timestamp.
//
// It returns a non-nil error (typically xtp.ErrValidateSecretInvalidBase32) if
// the secret is not valid base32. On error the returned code is empty; callers
// MUST NOT treat an empty string as a valid passcode.
func Code(secret string) (string, error) {
	return TOTPCode(secret)
}

// CodeCustom generates the totp code, with the default settings: digits=6, algorithm=SHA1, with your specified timestamp.
//
// It returns a non-nil error (typically xtp.ErrValidateSecretInvalidBase32) if
// the secret is not valid base32. On error the returned code is empty; callers
// MUST NOT treat an empty string as a valid passcode.
func CodeCustom(secret string, t time.Time) (string, error) {
	return TOTPCodeCustom(secret, t, nil)
}

// TOTPCode generates a TOTP code at the current time with default opts
// (digits=6, algorithm=SHA1, period=30s, skew=0).
//
// It returns a non-nil error (typically xtp.ErrValidateSecretInvalidBase32) if
// the secret is not valid base32. On error the returned code is empty; callers
// MUST NOT treat an empty string as a valid passcode.
//
// Note: an empty or whitespace secret does NOT error — base32 decodes it to an
// empty key that HMAC still digests into a real (but insecure) code. Callers
// MUST validate the secret is non-empty before use.
func TOTPCode(secret string) (string, error) {
	return totp.GenerateCode(secret, time.Now())
}

// TOTPCodeCustom generates a TOTP code at the given time with the given opts.
//
// It returns a non-nil error (typically xtp.ErrValidateSecretInvalidBase32) if
// the secret is not valid base32. On error the returned code is empty; callers
// MUST NOT treat an empty string as a valid passcode. See TOTPCode for the
// empty-secret caveat.
func TOTPCodeCustom(secret string, t time.Time, opts *Opts) (string, error) {
	return totp.GenerateCodeCustom(secret, t, totp.ValidateOpts{
		Period:    opts.GetPeriod(),
		Skew:      opts.GetSkew(),
		Digits:    opts.GetDigits(),
		Algorithm: opts.GetAlgorithm(),
	})
}

// HOTPCode generates an HOTP code for the given counter with default opts
// (digits=6, algorithm=SHA1).
//
// It returns a non-nil error (typically xtp.ErrValidateSecretInvalidBase32) if
// the secret is not valid base32. On error the returned code is empty; callers
// MUST NOT treat an empty string as a valid passcode. See TOTPCode for the
// empty-secret caveat.
func HOTPCode(secret string, counter uint64) (string, error) {
	return hotp.GenerateCode(secret, counter)
}

// HOTPCodeCustom generates an HOTP code for the given counter with the given opts.
//
// It returns a non-nil error (typically xtp.ErrValidateSecretInvalidBase32) if
// the secret is not valid base32. On error the returned code is empty; callers
// MUST NOT treat an empty string as a valid passcode. See TOTPCode for the
// empty-secret caveat.
func HOTPCodeCustom(secret string, counter uint64, opts *Opts) (string, error) {
	return hotp.GenerateCodeCustom(secret, counter, hotp.ValidateOpts{
		Digits:    opts.GetDigits(),
		Algorithm: opts.GetAlgorithm(),
	})
}

// VerifyTOTP validates a TOTP passcode against the secret. Comparison is
// constant-time (no timing leak). Accepts the current step plus ±1 (upstream
// default Skew=1, ~90s window).
//
// NOT replay-safe: the same code is accepted repeatedly within its window. Per
// RFC 6238 §5.2 the caller MUST track the last-accepted step and reject reuse —
// this package is stateless and cannot enforce it.
func VerifyTOTP(passcode string, secret string) bool {
	return totp.Validate(passcode, secret)
}

func VerifyTOTPCustom(passcode string, secret string, t time.Time, opts *Opts) (ret bool) {
	ret, _ = totp.ValidateCustom(passcode, secret, t, totp.ValidateOpts{
		Period:    opts.GetPeriod(),
		Skew:      opts.GetSkew(),
		Digits:    opts.GetDigits(),
		Algorithm: opts.GetAlgorithm(),
	})
	return
}

// VerifyHOTP validates an HOTP passcode for the given counter. Comparison is
// constant-time. Replay protection comes from the caller monotonically advancing
// the counter per attempt — reusing the same counter accepts the same code again.
func VerifyHOTP(passcode string, counter uint64, secret string) bool {
	return hotp.Validate(passcode, counter, secret)
}

func VerifyHOTPCustom(passcode string, counter uint64, secret string, opts *Opts) (ret bool) {
	ret, _ = hotp.ValidateCustom(passcode, counter, secret, hotp.ValidateOpts{
		Digits:    opts.GetDigits(),
		Algorithm: opts.GetAlgorithm(),
	})
	return
}
