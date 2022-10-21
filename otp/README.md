# OTP: One-Time Password

>[Time-based one-time password](https://en.wikipedia.org/wiki/Time-based_one-time_password#Client_implementations)

Referenced the following packages:
- [pyotp](https://github.com/pyauth/pyotp)
- [otp](https://github.com/pquerna/otp)
- [Key-Uri-Format](https://github.com/google/google-authenticator/wiki/Key-Uri-Format)

## Formula

`OTP(K, C) = Truncate(HMAC-SHA-1(K, C))`

>`HMAC` supports the following algorithms:
> - SHA1 (Default)
> - SHA256
> - SHA512
> - MD5

## Why

`kit4go/otp` is a Go library for generating and verifying one-time passwords. It can be used to implement two-factor (2FA) 
or multi-factor (MFA) authentication methods in web applications and in other systems that require users to log in.

It enables you to easily add TOTPs to your own application, increasing your user's security against mass-password breaches and malware.

## Features

- Generating QR Code images for easy user enrollment.
- Time-based One-time Password Algorithm (TOTP) (RFC 6238): Time based OTP, _the most commonly used method_.
- HMAC-based One-time Password Algorithm (HOTP) (RFC 4226): Counter based OTP, which TOTP is based upon.
- Generation and Validation of codes for either algorithm.

## Shall Know

- OTPs involve a shared secret, stored both on the device(client) and the server
- OTPs can be generated on a device without internet connectivity
- OTPs should always be used as a second factor of authentication (if your device is lost, you account is still secured with a password)
- Microsoft Authenticator, Google Authenticator and other OTP client apps allow you to store multiple OTP secrets and provision those using a QR Code

## Usage

- secret key
  - `RandomSecret(length int) string`  generates a random secret, b32NoPadding.
  - `VerifySecret(secret string) bool` verifies the secret is base32.
- otp url
  - `GenerateURLHOTP(opts KeyOpts) string` generates the hotp url
  - `GenerateURLTOTP(opts KeyOpts) string` generates the totp url
- **code totp** _most commonly used_
  - `Code(secret string) string` generates the totp code
  - `CodeCustom(secret string, t time.Time) string` generates the totp code with time
  - `TOTPCode(secret string) (code string)` generates the totp code
  - `TOTPCodeCustom(secret string, t time.Time, opts *Opts) string` generates the totp code with time and opts
  - `VerifyTOTP(passcode string, secret string) bool` verifies the code of totp
  - `VerifyTOTPCustom(passcode string, secret string, t time.Time, opts *Opts) bool` verifies the code of totp with opts
- code hotp
  - `HOTPCode(secret string, counter uint64) string` generates the hotp code
  - `HOTPCodeCustom(secret string, counter uint64, opts *Opts) string` generates the hotp code with the opts
  - `VerifyHOTP(passcode string, counter uint64, secret string) bool` verifies the code of hotp
  - `VerifyHOTPCustom(passcode string, counter uint64, secret string, opts *Opts) bool` verifies the code of hotp with opts
