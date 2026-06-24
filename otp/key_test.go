package otp_test

import (
	"encoding/base32"
	"errors"
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
	})
}

func TestVerifySecret(t *testing.T) {
	convey.SetDefaultFailureMode(convey.FailureContinues)
	convey.Convey("TestVerifySecret", t, func() {
		convey.So(otp.VerifySecret("7ZDW4TVCYM"), convey.ShouldBeTrue)
		convey.So(otp.VerifySecret("JBSWY3DPEHPK3PXP"), convey.ShouldBeTrue)
	})
}

func TestGenerateURLHOTP(t *testing.T) {
	convey.SetDefaultFailureMode(convey.FailureContinues)
	convey.Convey("TestGenerateURLHOTP", t, func() {
		convey.So(otp.GenerateURLHOTP(otp.KeyOpts{Issuer: ""}), convey.ShouldBeEmpty)
		convey.So(otp.GenerateURLHOTP(otp.KeyOpts{Issuer: "xwi88"}), convey.ShouldBeEmpty)
		convey.So(otp.GenerateURLHOTP(otp.KeyOpts{Issuer: "xwi88", AccountName: "xwi88.com"}), convey.ShouldNotBeEmpty)
		convey.So(otp.GenerateURLHOTP(otp.KeyOpts{Issuer: "xwi88", AccountName: "xwi88.com", Algorithm: otp.AlgorithmSHA512}), convey.ShouldNotBeEmpty)
		convey.So(otp.GenerateURLHOTP(otp.KeyOpts{Issuer: "xwi88", AccountName: "xwi88.com", Secret: []byte("7ZDW4TVCYM")}), convey.ShouldNotBeEmpty)
		convey.So(otp.GenerateURLHOTP(otp.KeyOpts{Issuer: "xwi88", AccountName: "xwi88.com", SecretSize: uint(0)}), convey.ShouldNotBeEmpty)
		convey.So(otp.GenerateURLHOTP(otp.KeyOpts{Issuer: "xwi88", AccountName: "xwi88.com", Secret: []byte("7ZDW4TVCYM"), Digits: 8}), convey.ShouldNotBeEmpty)
	})
}

func TestGenerateURLTOTP(t *testing.T) {
	convey.SetDefaultFailureMode(convey.FailureContinues)
	convey.Convey("TestGenerateURLTOTP", t, func() {
		convey.So(otp.GenerateURLTOTP(otp.KeyOpts{Issuer: ""}), convey.ShouldBeEmpty)
		convey.So(otp.GenerateURLTOTP(otp.KeyOpts{Issuer: "xwi88"}), convey.ShouldBeEmpty)
		convey.So(otp.GenerateURLTOTP(otp.KeyOpts{Issuer: "xwi88", AccountName: "xwi88.com"}), convey.ShouldNotBeEmpty)
		convey.So(otp.GenerateURLTOTP(otp.KeyOpts{Issuer: "xwi88", AccountName: "xwi88.com", Algorithm: otp.AlgorithmSHA512}), convey.ShouldNotBeEmpty)
		convey.So(otp.GenerateURLTOTP(otp.KeyOpts{Issuer: "xwi88", AccountName: "xwi88.com", Secret: []byte("7ZDW4TVCYM")}), convey.ShouldNotBeEmpty)
		convey.So(otp.GenerateURLTOTP(otp.KeyOpts{Issuer: "xwi88", AccountName: "xwi88.com", SecretSize: uint(0)}), convey.ShouldNotBeEmpty)
		convey.So(otp.GenerateURLTOTP(otp.KeyOpts{Issuer: "xwi88", AccountName: "xwi88.com", Secret: []byte("7ZDW4TVCYM"), Digits: 8}), convey.ShouldNotBeEmpty)
	})
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
