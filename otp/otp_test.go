package otp_test

import (
	"errors"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"

	xtp "github.com/pquerna/otp"

	"github.com/v8fg/kit4go/otp"
)

const validSecret = "JBSWY3DPEHPK3PXP"

func TestCode(t *testing.T) {
	convey.Convey("TestCode", t, func() {
		convey.Convey("valid secret yields a 6-digit code", func() {
			code, err := otp.Code(validSecret)
			convey.So(err, convey.ShouldBeNil)
			convey.So(code, convey.ShouldHaveLength, 6)
		})
	})
}

func TestCodeCustom(t *testing.T) {
	convey.Convey("TestCodeCustom", t, func() {
		convey.Convey("valid secret yields a 6-digit code", func() {
			code, err := otp.CodeCustom(validSecret, time.Now())
			convey.So(err, convey.ShouldBeNil)
			convey.So(code, convey.ShouldHaveLength, 6)
		})
	})
}

func TestTOTPCode(t *testing.T) {
	convey.Convey("TestTOTPCode", t, func() {
		convey.Convey("valid secret yields a 6-digit code", func() {
			code, err := otp.TOTPCode(validSecret)
			convey.So(err, convey.ShouldBeNil)
			convey.So(code, convey.ShouldHaveLength, 6)
		})

		// NOTE: an empty ("") or whitespace secret does NOT error upstream —
		// base32 decodes it to an empty key, which HMAC still digests into a
		// real (but insecure) code. Only a non-base32 secret surfaces the
		// underlying DecodeString failure; that is the path asserted below.
		convey.Convey("non-base32 secret returns a non-nil error and empty code", func() {
			code, err := otp.TOTPCode("not!base32!secret")
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(errors.Is(err, xtp.ErrValidateSecretInvalidBase32), convey.ShouldBeTrue)
			convey.So(code, convey.ShouldBeEmpty)
		})
	})
}

func TestTOTPCodeCustom(t *testing.T) {
	convey.Convey("TestTOTPCodeCustom", t, func() {
		convey.Convey("valid secret yields a 6-digit code across opts", func() {
			now := time.Now()

			code, err := otp.TOTPCodeCustom(validSecret, now, nil)
			convey.So(err, convey.ShouldBeNil)
			convey.So(code, convey.ShouldHaveLength, 6)

			code, err = otp.TOTPCodeCustom(validSecret, now, &otp.Opts{Period: 60})
			convey.So(err, convey.ShouldBeNil)
			convey.So(code, convey.ShouldHaveLength, 6)

			code, err = otp.TOTPCodeCustom(validSecret, now, &otp.Opts{Period: 60, Digits: 6})
			convey.So(err, convey.ShouldBeNil)
			convey.So(code, convey.ShouldHaveLength, 6)
		})

		// See TestTOTPCode: empty secret does not error upstream.
		convey.Convey("non-base32 secret returns a non-nil error and empty code", func() {
			code, err := otp.TOTPCodeCustom("not!base32!secret", time.Now(), &otp.Opts{Period: 60})
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(errors.Is(err, xtp.ErrValidateSecretInvalidBase32), convey.ShouldBeTrue)
			convey.So(code, convey.ShouldBeEmpty)
		})
	})
}

func TestHOTPCode(t *testing.T) {
	convey.Convey("TestHOTPCode", t, func() {
		convey.Convey("valid secret yields a 6-digit code", func() {
			code, err := otp.HOTPCode(validSecret, 1)
			convey.So(err, convey.ShouldBeNil)
			convey.So(code, convey.ShouldHaveLength, 6)

			code, err = otp.HOTPCode(validSecret, 2)
			convey.So(err, convey.ShouldBeNil)
			convey.So(code, convey.ShouldHaveLength, 6)
		})

		// See TestTOTPCode: empty secret does not error upstream.
		convey.Convey("non-base32 secret returns a non-nil error and empty code", func() {
			code, err := otp.HOTPCode("not!base32!secret", 1)
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(errors.Is(err, xtp.ErrValidateSecretInvalidBase32), convey.ShouldBeTrue)
			convey.So(code, convey.ShouldBeEmpty)
		})
	})
}

func TestHOTPCodeCustom(t *testing.T) {
	convey.Convey("TestHOTPCodeCustom", t, func() {
		convey.Convey("valid secret yields the expected-length code", func() {
			code, err := otp.HOTPCodeCustom(validSecret, 1, &otp.Opts{Digits: 8})
			convey.So(err, convey.ShouldBeNil)
			convey.So(code, convey.ShouldHaveLength, 8)

			code, err = otp.HOTPCodeCustom(validSecret, 1, &otp.Opts{Digits: 6})
			convey.So(err, convey.ShouldBeNil)
			convey.So(code, convey.ShouldHaveLength, 6)

			code, err = otp.HOTPCodeCustom(validSecret, 2, &otp.Opts{Digits: 8})
			convey.So(err, convey.ShouldBeNil)
			convey.So(code, convey.ShouldHaveLength, 8)

			code, err = otp.HOTPCodeCustom(validSecret, 2, &otp.Opts{Digits: 6})
			convey.So(err, convey.ShouldBeNil)
			convey.So(code, convey.ShouldHaveLength, 6)
		})

		// See TestTOTPCode: empty secret does not error upstream.
		convey.Convey("non-base32 secret returns a non-nil error and empty code", func() {
			code, err := otp.HOTPCodeCustom("not!base32!secret", 1, &otp.Opts{Digits: 8})
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(errors.Is(err, xtp.ErrValidateSecretInvalidBase32), convey.ShouldBeTrue)
			convey.So(code, convey.ShouldBeEmpty)
		})
	})
}

func TestVerifyTOTP(t *testing.T) {
	convey.Convey("TestVerifyTOTP", t, func() {
		secret := validSecret
		code, err := otp.TOTPCode(secret)
		convey.So(err, convey.ShouldBeNil)
		convey.So(otp.VerifyTOTP(code, secret), convey.ShouldBeTrue)
	})
}

func TestVerifyTOTPCustom(t *testing.T) {
	convey.Convey("TestVerifyTOTPCustom", t, func() {
		secret := validSecret
		now := time.Now()
		code, err := otp.TOTPCodeCustom(secret, now, nil)
		convey.So(err, convey.ShouldBeNil)
		convey.So(otp.VerifyTOTPCustom(code, secret, now, nil), convey.ShouldBeTrue)
	})
}

func TestVerifyHOTP(t *testing.T) {
	convey.Convey("TestVerifyHOTP", t, func() {
		secret := validSecret

		code, err := otp.HOTPCode(secret, 1)
		convey.So(err, convey.ShouldBeNil)
		convey.So(otp.VerifyHOTP(code, 1, secret), convey.ShouldBeTrue)

		code, err = otp.HOTPCode(secret, 2)
		convey.So(err, convey.ShouldBeNil)
		convey.So(otp.VerifyHOTP(code, 2, secret), convey.ShouldBeTrue)
	})
}

func TestVerifyHOTPCustom(t *testing.T) {
	convey.Convey("TestVerifyHOTPCustom", t, func() {
		secret := validSecret

		code, err := otp.HOTPCodeCustom(secret, 1, nil)
		convey.So(err, convey.ShouldBeNil)
		convey.So(otp.VerifyHOTPCustom(code, 1, secret, nil), convey.ShouldBeTrue)

		code, err = otp.HOTPCodeCustom(secret, 2, nil)
		convey.So(err, convey.ShouldBeNil)
		convey.So(otp.VerifyHOTPCustom(code, 2, secret, nil), convey.ShouldBeTrue)
	})
}
