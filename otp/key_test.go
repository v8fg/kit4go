package otp_test

import (
	"encoding/base32"
	"errors"
	"io"
	"net/url"
	"strings"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/mock"

	"github.com/v8fg/kit4go/otp"
)

var b32NoPadding = base32.StdEncoding.WithPadding(base32.NoPadding)

// withRandomReader temporarily replaces the package-level DefaultRandomReader
// for the duration of fn (defer-restored). It asserts the mock expectations
// were met. fn must run within a convey.Convey context.
func withRandomReader(t *testing.T, mockReader *otp.MockRandomReader, fn func()) {
	t.Helper()
	orig := otp.DefaultRandomReader
	otp.DefaultRandomReader = mockReader
	defer func() { otp.DefaultRandomReader = orig }()
	fn()
	if !mockReader.Mock.AssertExpectations(t) {
		t.Fail()
	}
}

func TestRandomSecret(t *testing.T) {
	convey.SetDefaultFailureMode(convey.FailureContinues)
	convey.Convey("TestRandomSecret", t, func() {
		convey.Convey("TestRandomSecret-Success", func() {
			code := otp.RandomSecret(4)
			decodeString, _ := b32NoPadding.DecodeString(code)
			convey.So(decodeString, convey.ShouldHaveLength, 4)
			code = otp.RandomSecret(6)
			decodeString, _ = b32NoPadding.DecodeString(code)
			convey.So(decodeString, convey.ShouldHaveLength, 6)
		})

		// error-path: rand.Read returns an error -> RandomSecret returns "".
		convey.Convey("TestRandomSecret-ReadError", func() {
			mockReader := new(otp.MockRandomReader)
			mockReader.EXPECT().Read(mock.Anything).
				Return(0, errors.New("rand.Read error")).Once()
			withRandomReader(t, mockReader, func() {
				convey.So(otp.RandomSecret(6), convey.ShouldEqual, "")
			})
		})

		// error-path: rand.Read returns short read (n < length) -> "".
		convey.Convey("TestRandomSecret-ShortRead", func() {
			mockReader := new(otp.MockRandomReader)
			mockReader.EXPECT().Read(mock.Anything).
				Return(0, nil).Once()
			withRandomReader(t, mockReader, func() {
				convey.So(otp.RandomSecret(6), convey.ShouldEqual, "")
			})
		})

		// Regression (F5): a negative length previously panicked inside
		// make([]byte, -5) ("makeslice: len out of range"). The guard now
		// returns "" before the allocation, so the call must not panic and
		// must return the same sentinel as a CSPRNG failure.
		convey.Convey("TestRandomSecret-NegativeLength-NoPanic", func() {
			convey.So(otp.RandomSecret(-5), convey.ShouldEqual, "")
			convey.So(otp.RandomSecret(-1), convey.ShouldEqual, "")
		})

		// Regression (F5): length 0 returns "" rather than allocating. This is
		// the documented ""-on-no-op contract; it is indistinguishable from a
		// CSPRNG failure, which the doc calls out explicitly.
		convey.Convey("TestRandomSecret-ZeroLength", func() {
			convey.So(otp.RandomSecret(0), convey.ShouldEqual, "")
		})
	})
}

func TestVerifySecret(t *testing.T) {
	convey.SetDefaultFailureMode(convey.FailureContinues)
	convey.Convey("TestVerifySecret", t, func() {
		convey.So(otp.VerifySecret("7ZDW4TVCYM"), convey.ShouldBeTrue)
		convey.So(otp.VerifySecret("JBSWY3DPEHPK3PXP"), convey.ShouldBeTrue)

		// Regression (F6): an empty or whitespace-only secret previously passed
		// because the bare base32 decoder accepts "" as valid, which would let
		// a caller gate 2FA provisioning on an empty secret. Now rejected.
		convey.So(otp.VerifySecret(""), convey.ShouldBeFalse)
		convey.So(otp.VerifySecret("   "), convey.ShouldBeFalse)
		convey.So(otp.VerifySecret("\t\n"), convey.ShouldBeFalse)

		// "0" is not a valid base32 symbol -> must be false (format check).
		convey.So(otp.VerifySecret("0"), convey.ShouldBeFalse)
		convey.So(otp.VerifySecret("not!base32!secret"), convey.ShouldBeFalse)

		// A whitespace-padded valid secret still trims and verifies true.
		convey.So(otp.VerifySecret("  JBSWY3DPEHPK3PXP  "), convey.ShouldBeTrue)
	})
}

func TestGenerateURLHOTP(t *testing.T) {
	convey.SetDefaultFailureMode(convey.FailureContinues)
	convey.Convey("TestGenerateURLHOTP", t, func() {
		// invalid opts -> non-nil error, empty URL.
		url, err := otp.GenerateURLHOTP(otp.KeyOpts{Issuer: ""})
		convey.So(err, convey.ShouldBeError)
		convey.So(url, convey.ShouldBeEmpty)

		url, err = otp.GenerateURLHOTP(otp.KeyOpts{Issuer: "xwi88"})
		convey.So(err, convey.ShouldBeError)
		convey.So(url, convey.ShouldBeEmpty)

		// success paths -> non-empty URL, nil error.
		url, err = otp.GenerateURLHOTP(otp.KeyOpts{Issuer: "xwi88", AccountName: "xwi88.com"})
		convey.So(err, convey.ShouldBeNil)
		convey.So(url, convey.ShouldNotBeEmpty)
		convey.So(url, convey.ShouldStartWith, "otpauth://hotp/")

		url, err = otp.GenerateURLHOTP(otp.KeyOpts{Issuer: "xwi88", AccountName: "xwi88.com", Algorithm: otp.AlgorithmSHA512})
		convey.So(err, convey.ShouldBeNil)
		convey.So(url, convey.ShouldNotBeEmpty)

		url, err = otp.GenerateURLHOTP(otp.KeyOpts{Issuer: "xwi88", AccountName: "xwi88.com", Secret: []byte("7ZDW4TVCYM")})
		convey.So(err, convey.ShouldBeNil)
		convey.So(url, convey.ShouldNotBeEmpty)

		url, err = otp.GenerateURLHOTP(otp.KeyOpts{Issuer: "xwi88", AccountName: "xwi88.com", SecretSize: uint(0)})
		convey.So(err, convey.ShouldBeNil)
		convey.So(url, convey.ShouldNotBeEmpty)

		url, err = otp.GenerateURLHOTP(otp.KeyOpts{Issuer: "xwi88", AccountName: "xwi88.com", Secret: []byte("7ZDW4TVCYM"), Digits: 8})
		convey.So(err, convey.ShouldBeNil)
		convey.So(url, convey.ShouldNotBeEmpty)
	})
}

func TestGenerateURLTOTP(t *testing.T) {
	convey.SetDefaultFailureMode(convey.FailureContinues)
	convey.Convey("TestGenerateURLTOTP", t, func() {
		// invalid opts -> non-nil error, empty URL.
		url, err := otp.GenerateURLTOTP(otp.KeyOpts{Issuer: ""})
		convey.So(err, convey.ShouldBeError)
		convey.So(url, convey.ShouldBeEmpty)

		url, err = otp.GenerateURLTOTP(otp.KeyOpts{Issuer: "xwi88"})
		convey.So(err, convey.ShouldBeError)
		convey.So(url, convey.ShouldBeEmpty)

		// success paths -> non-empty URL, nil error.
		url, err = otp.GenerateURLTOTP(otp.KeyOpts{Issuer: "xwi88", AccountName: "xwi88.com"})
		convey.So(err, convey.ShouldBeNil)
		convey.So(url, convey.ShouldNotBeEmpty)
		convey.So(url, convey.ShouldStartWith, "otpauth://totp/")

		url, err = otp.GenerateURLTOTP(otp.KeyOpts{Issuer: "xwi88", AccountName: "xwi88.com", Algorithm: otp.AlgorithmSHA512})
		convey.So(err, convey.ShouldBeNil)
		convey.So(url, convey.ShouldNotBeEmpty)

		url, err = otp.GenerateURLTOTP(otp.KeyOpts{Issuer: "xwi88", AccountName: "xwi88.com", Secret: []byte("7ZDW4TVCYM")})
		convey.So(err, convey.ShouldBeNil)
		convey.So(url, convey.ShouldNotBeEmpty)

		url, err = otp.GenerateURLTOTP(otp.KeyOpts{Issuer: "xwi88", AccountName: "xwi88.com", SecretSize: uint(0)})
		convey.So(err, convey.ShouldBeNil)
		convey.So(url, convey.ShouldNotBeEmpty)

		url, err = otp.GenerateURLTOTP(otp.KeyOpts{Issuer: "xwi88", AccountName: "xwi88.com", Secret: []byte("7ZDW4TVCYM"), Digits: 8})
		convey.So(err, convey.ShouldBeNil)
		convey.So(url, convey.ShouldNotBeEmpty)

		// non-zero Period: simpleURL only emits the period query param when
		// otpType == "totp" && opts.Period != 0 (key.go line 200).
		url, err = otp.GenerateURLTOTP(otp.KeyOpts{Issuer: "xwi88", AccountName: "xwi88.com", Period: 60})
		convey.So(err, convey.ShouldBeNil)
		convey.So(url, convey.ShouldNotBeEmpty)
		convey.So(url, convey.ShouldContainSubstring, "period=60")
	})
}

// TestGenerateURLRandFailure is the regression test for the P0 CSPRNG-failure
// bug: previously simpleURL did `_, _ = opts.Rand.Read(secret)`, discarding
// both the error and the short-read count. On entropy exhaustion the secret
// stayed all-zero and was embedded — visibly — in the returned otpauth URL,
// silently breaking 2FA. The fix uses io.ReadFull and propagates the error.
//
// We exercise three failing-reader shapes through the opts.Rand seam:
//   - read returns an error (e.g. CSPRNG failure);
//   - read returns fewer than SecretSize bytes then io.EOF (entropy source
//     exhausted early — io.ReadFull treats any short read ending in io.EOF
//     before the buffer is full as io.ErrUnexpectedEOF);
//   - read returns a partial buffer with io.EOF on the same call.
//
// All three must surface a non-nil error wrapping otp.ErrSecretReadFailed and
// an empty URL. The pre-fix code returned a URL carrying an all-zero (or
// partial) secret and no error.
func TestGenerateURLRandFailure(t *testing.T) {
	convey.SetDefaultFailureMode(convey.FailureContinues)

	base := otp.KeyOpts{Issuer: "xwi88", AccountName: "xwi88.com"}

	cases := []struct {
		name string
		rand io.Reader
	}{
		{name: "read-error", rand: errReader{}},
		{name: "short-then-eof", rand: strings.NewReader("short")}, // 5 < 20 bytes -> EOF
		{name: "partial-then-eof", rand: partialEOFReader{n: 4}},
	}

	for _, tc := range cases {
		convey.Convey("HOTP/"+tc.name, t, func() {
			opts := base
			opts.Rand = tc.rand
			url, err := otp.GenerateURLHOTP(opts)
			convey.So(err, convey.ShouldBeError)
			convey.So(errors.Is(err, otp.ErrSecretReadFailed), convey.ShouldBeTrue)
			convey.So(url, convey.ShouldBeEmpty)
		})
		convey.Convey("TOTP/"+tc.name, t, func() {
			opts := base
			opts.Rand = tc.rand
			url, err := otp.GenerateURLTOTP(opts)
			convey.So(err, convey.ShouldBeError)
			convey.So(errors.Is(err, otp.ErrSecretReadFailed), convey.ShouldBeTrue)
			convey.So(url, convey.ShouldBeEmpty)
		})
	}
}

// TestGenerateURLIssuerColon is the regression test for the F7 otpauth-label
// bug. The label path is "/Issuer:AccountName" and the upstream parser
// (pquerna/otp) splits AccountName on the FIRST ':', so a ':' inside Issuer
// previously round-tripped a corrupted AccountName: Issuer "A:B" +
// AccountName "user" came back as "B:user". url.PathEscape does not escape
// ':' (it is a valid path char), so the fix rejects ':' in Issuer with
// otp.ErrIssuerContainsColon. The secret is an independent query param and is
// not affected. For a well-formed (colon-free) Issuer the URL must still
// round-trip AccountName unchanged.
func TestGenerateURLIssuerColon(t *testing.T) {
	convey.SetDefaultFailureMode(convey.FailureContinues)

	convey.Convey("colon in Issuer is rejected for TOTP and HOTP", t, func() {
		for _, fn := range []struct {
			name string
			gen  func(otp.KeyOpts) (string, error)
		}{
			{"TOTP", otp.GenerateURLTOTP},
			{"HOTP", otp.GenerateURLHOTP},
		} {
			convey.Convey(fn.name, func() {
				opts := otp.KeyOpts{
					Issuer:      "A:B",
					AccountName: "user",
					Secret:      []byte("12345678901234567890"),
				}
				url, err := fn.gen(opts)
				convey.So(err, convey.ShouldBeError)
				convey.So(errors.Is(err, otp.ErrIssuerContainsColon), convey.ShouldBeTrue)
				convey.So(url, convey.ShouldBeEmpty)
			})
		}
	})

	convey.Convey("colon-free Issuer round-trips AccountName unchanged", t, func() {
		opts := otp.KeyOpts{
			Issuer:      "ACME Co",
			AccountName: "john.doe@email.com",
			Secret:      []byte("12345678901234567890"),
		}
		rawURL, err := otp.GenerateURLTOTP(opts)
		convey.So(err, convey.ShouldBeNil)

		parsed, err := url.Parse(rawURL)
		convey.So(err, convey.ShouldBeNil)
		// Mirror the upstream AccountName() parser: trim leading '/', split on
		// the first ':'. The re-parsed AccountName must equal the input.
		p := strings.TrimPrefix(parsed.Path, "/")
		i := strings.Index(p, ":")
		var account string
		if i == -1 {
			account = p
		} else {
			account = p[i+1:]
		}
		convey.So(account, convey.ShouldEqual, opts.AccountName)
		convey.So(account, convey.ShouldNotEqual, "B:"+opts.AccountName)
	})
}

// errReader always fails to read.
type errReader struct{}

func (errReader) Read(p []byte) (int, error) { return 0, errors.New("entropy exhausted") }

// partialEOFReader returns at most n bytes then io.EOF on the first call,
// so the secret buffer is never filled and io.ReadFull returns an error.
type partialEOFReader struct{ n int }

func (r partialEOFReader) Read(p []byte) (int, error) {
	if r.n < len(p) {
		for i := 0; i < r.n && i < len(p); i++ {
			p[i] = 0xAA
		}
		return r.n, io.EOF
	}
	for i := range p {
		p[i] = 0xAA
	}
	return len(p), io.EOF
}

func TestKeyFromURL(t *testing.T) {
	convey.SetDefaultFailureMode(convey.FailureContinues)
	convey.Convey("TestKeyFromURL", t, func() {
		key, err := otp.KeyFromURL("")
		convey.So(key, convey.ShouldBeNil)
		convey.So(err, convey.ShouldBeError)
		key, err = otp.KeyFromURL("otpauth://totp/Example:alice@google.com?secret=JBSWY3DPEHPK3PXP&issuer=Example")
		convey.So(key, convey.ShouldNotBeNil)
		convey.So(err, convey.ShouldBeNil)
		key, err = otp.KeyFromURL("otpauth://totp/ACME%20Co:john.doe@email.com?secret=HXDMVJECJJWSRB3HWIZR4IFUGFTMXBOZ&issuer=ACME%20Co&algorithm=SHA1&digits=6&period=30")
		convey.So(key, convey.ShouldNotBeNil)
		convey.So(err, convey.ShouldBeNil)
	})
}

func TestKeyFromHOTPOpts(t *testing.T) {
	convey.SetDefaultFailureMode(convey.FailureContinues)
	convey.Convey("TestKeyFromHOTPOpts", t, func() {
		key, err := otp.KeyFromHOTPOpts(otp.KeyOpts{Issuer: ""})
		convey.So(key, convey.ShouldBeNil)
		convey.So(err, convey.ShouldBeError)
		key, err = otp.KeyFromHOTPOpts(otp.KeyOpts{Issuer: "xwi88"})
		convey.So(key, convey.ShouldBeNil)
		convey.So(err, convey.ShouldBeError)
		key, err = otp.KeyFromHOTPOpts(otp.KeyOpts{Issuer: "xwi88", AccountName: "xwi88.com"})
		convey.So(key, convey.ShouldNotBeNil)
		convey.So(err, convey.ShouldBeNil)
	})
}

func TestKeyFromTOTPOpts(t *testing.T) {
	convey.SetDefaultFailureMode(convey.FailureContinues)
	convey.Convey("TestKeyFromTOTPOpts", t, func() {
		key, err := otp.KeyFromTOTPOpts(otp.KeyOpts{Issuer: ""})
		convey.So(key, convey.ShouldBeNil)
		convey.So(err, convey.ShouldBeError)
		key, err = otp.KeyFromTOTPOpts(otp.KeyOpts{Issuer: "xwi88"})
		convey.So(key, convey.ShouldBeNil)
		convey.So(err, convey.ShouldBeError)
		key, err = otp.KeyFromTOTPOpts(otp.KeyOpts{Issuer: "xwi88", AccountName: "xwi88.com"})
		convey.So(key, convey.ShouldNotBeNil)
		convey.So(err, convey.ShouldBeNil)
	})
}
