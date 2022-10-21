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
func Code(secret string) string {
	return TOTPCode(secret)
}

// CodeCustom generates the totp code, with the default settings: digits=6, algorithm=SHA1, with your specified timestamp.
func CodeCustom(secret string, t time.Time) string {
	return TOTPCodeCustom(secret, t, nil)
}

func TOTPCode(secret string) (code string) {
	code, _ = totp.GenerateCode(secret, time.Now())
	return
}

func TOTPCodeCustom(secret string, t time.Time, opts *Opts) (code string) {
	code, _ = totp.GenerateCodeCustom(secret, t, totp.ValidateOpts{
		Period:    opts.GetPeriod(),
		Skew:      opts.GetSkew(),
		Digits:    opts.GetDigits(),
		Algorithm: opts.GetAlgorithm(),
	})
	return
}

func HOTPCode(secret string, counter uint64) (code string) {
	code, _ = hotp.GenerateCode(secret, counter)
	return
}

func HOTPCodeCustom(secret string, counter uint64, opts *Opts) (code string) {
	code, _ = hotp.GenerateCodeCustom(secret, counter, hotp.ValidateOpts{
		Digits:    opts.GetDigits(),
		Algorithm: opts.GetAlgorithm(),
	})
	return
}

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
