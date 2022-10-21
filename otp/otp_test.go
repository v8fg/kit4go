package otp_test

import (
	"testing"
	"time"

	"github.com/agiledragon/gomonkey"
	"github.com/pquerna/otp/hotp"
	"github.com/pquerna/otp/totp"
	"github.com/smartystreets/goconvey/convey"

	"github.com/v8fg/kit4go/otp"
)

func TestCode(t *testing.T) {
	convey.Convey("TestCode", t, func() {
		outputs := []gomonkey.OutputCell{
			{Values: gomonkey.Params{"563324"}, Times: 1},
			{Values: gomonkey.Params{"487978"}, Times: 1},
		}
		af := gomonkey.ApplyFuncSeq(otp.TOTPCode, outputs)
		defer af.Reset()
		convey.So(otp.Code("JBSWY3DPEHPK3PXP"), convey.ShouldEqual, "563324")
		convey.So(otp.Code("JBSWY3DPEHPK3PXP"), convey.ShouldEqual, "487978")
	})
}

func TestCodeCustom(t *testing.T) {
	convey.Convey("TestCodeCustom", t, func() {
		outputs := []gomonkey.OutputCell{
			{Values: gomonkey.Params{"385109"}, Times: 1},
			{Values: gomonkey.Params{"833446"}, Times: 1},
		}
		af := gomonkey.ApplyFuncSeq(otp.TOTPCodeCustom, outputs)
		defer af.Reset()
		convey.So(otp.CodeCustom("JBSWY3DPEHPK3PXP", time.Now()), convey.ShouldEqual, "385109")
		convey.So(otp.CodeCustom("JBSWY3DPEHPK3PXP", time.Now()), convey.ShouldEqual, "833446")
	})
}

func TestTOTPCode(t *testing.T) {
	convey.Convey("TestTOTPCode", t, func() {
		outputs := []gomonkey.OutputCell{
			{Values: gomonkey.Params{"563324", nil}, Times: 1},
			{Values: gomonkey.Params{"487978", nil}, Times: 1},
		}
		af := gomonkey.ApplyFuncSeq(totp.GenerateCode, outputs)
		defer af.Reset()
		convey.So(otp.TOTPCode("JBSWY3DPEHPK3PXP"), convey.ShouldEqual, "563324")
		convey.So(otp.TOTPCode("JBSWY3DPEHPK3PXP"), convey.ShouldEqual, "487978")
	})
}

func TestTOTPCodeCustom(t *testing.T) {
	convey.Convey("TestTOTPCodeCustom", t, func() {
		outputs := []gomonkey.OutputCell{
			{Values: gomonkey.Params{"563324", nil}, Times: 1},
			{Values: gomonkey.Params{"487978", nil}, Times: 1},
			{Values: gomonkey.Params{"008395", nil}, Times: 1},
			{Values: gomonkey.Params{"116644", nil}, Times: 1},
		}
		af := gomonkey.ApplyFuncSeq(totp.GenerateCodeCustom, outputs)
		defer af.Reset()
		convey.So(otp.TOTPCodeCustom("JBSWY3DPEHPK3PXP", time.Now(), nil), convey.ShouldEqual, "563324")
		convey.So(otp.TOTPCodeCustom("JBSWY3DPEHPK3PXP", time.Now(), nil), convey.ShouldEqual, "487978")
		convey.So(otp.TOTPCodeCustom("JBSWY3DPEHPK3PXP", time.Now(), &otp.Opts{Period: 60}), convey.ShouldEqual, "008395")
		convey.So(otp.TOTPCodeCustom("JBSWY3DPEHPK3PXP", time.Now(), &otp.Opts{Period: 60, Digits: 6}), convey.ShouldEqual, "116644")
	})
}

func TestHOTPCode(t *testing.T) {
	convey.Convey("TestHOTPCode", t, func() {
		outputs := []gomonkey.OutputCell{
			{Values: gomonkey.Params{"996554", nil}, Times: 1},
			{Values: gomonkey.Params{"602287", nil}, Times: 1},
		}
		af := gomonkey.ApplyFuncSeq(hotp.GenerateCode, outputs)
		defer af.Reset()
		convey.So(otp.HOTPCode("JBSWY3DPEHPK3PXP", 1), convey.ShouldEqual, "996554")
		convey.So(otp.HOTPCode("JBSWY3DPEHPK3PXP", 2), convey.ShouldEqual, "602287")
	})
}

func TestHOTPCodeCustom(t *testing.T) {
	convey.Convey("TestHOTPCodeCustom", t, func() {
		outputs := []gomonkey.OutputCell{
			{Values: gomonkey.Params{"41996554", nil}, Times: 1},
			{Values: gomonkey.Params{"996554", nil}, Times: 1},
			{Values: gomonkey.Params{"88602287", nil}, Times: 1},
			{Values: gomonkey.Params{"602287", nil}, Times: 1},
		}
		af := gomonkey.ApplyFuncSeq(hotp.GenerateCodeCustom, outputs)
		defer af.Reset()
		convey.So(otp.HOTPCodeCustom("JBSWY3DPEHPK3PXP", 1, &otp.Opts{Digits: 8}), convey.ShouldEqual, "41996554")
		convey.So(otp.HOTPCodeCustom("JBSWY3DPEHPK3PXP", 1, &otp.Opts{Digits: 6}), convey.ShouldEqual, "996554")
		convey.So(otp.HOTPCodeCustom("JBSWY3DPEHPK3PXP", 2, &otp.Opts{Digits: 8}), convey.ShouldEqual, "88602287")
		convey.So(otp.HOTPCodeCustom("JBSWY3DPEHPK3PXP", 2, &otp.Opts{Digits: 6}), convey.ShouldEqual, "602287")
	})
}

func TestVerifyTOTP(t *testing.T) {
	convey.Convey("TestVerifyTOTP", t, func() {
		outputs := []gomonkey.OutputCell{
			{Values: gomonkey.Params{true}, Times: 1},
			{Values: gomonkey.Params{true}, Times: 1},
		}
		af := gomonkey.ApplyFuncSeq(totp.Validate, outputs)
		defer af.Reset()
		convey.So(otp.VerifyTOTP("563324", "JBSWY3DPEHPK3PXP"), convey.ShouldBeTrue)
		convey.So(otp.VerifyTOTP("487978", "JBSWY3DPEHPK3PXP"), convey.ShouldBeTrue)
	})
}

func TestVerifyTOTPCustom(t *testing.T) {
	convey.Convey("TestVerifyTOTPCustom", t, func() {
		outputs := []gomonkey.OutputCell{
			{Values: gomonkey.Params{true, nil}, Times: 1},
			{Values: gomonkey.Params{true, nil}, Times: 1},
		}
		af := gomonkey.ApplyFuncSeq(totp.ValidateCustom, outputs)
		defer af.Reset()
		convey.So(otp.VerifyTOTPCustom("563324", "JBSWY3DPEHPK3PXP", time.Now(), nil), convey.ShouldBeTrue)
		convey.So(otp.VerifyTOTPCustom("487978", "JBSWY3DPEHPK3PXP", time.Now(), nil), convey.ShouldBeTrue)
	})
}

func TestVerifyHOTP(t *testing.T) {
	convey.Convey("TestVerifyHOTP", t, func() {
		outputs := []gomonkey.OutputCell{
			{Values: gomonkey.Params{true}, Times: 1},
			{Values: gomonkey.Params{true}, Times: 1},
		}
		af := gomonkey.ApplyFuncSeq(hotp.Validate, outputs)
		defer af.Reset()
		convey.So(otp.VerifyHOTP("996554", 1, "JBSWY3DPEHPK3PXP"), convey.ShouldBeTrue)
		convey.So(otp.VerifyHOTP("602287", 2, "JBSWY3DPEHPK3PXP"), convey.ShouldBeTrue)
	})
}

func TestVerifyHOTPCustom(t *testing.T) {
	convey.Convey("TestVerifyHOTPCustom", t, func() {
		outputs := []gomonkey.OutputCell{
			{Values: gomonkey.Params{true, nil}, Times: 1},
			{Values: gomonkey.Params{true, nil}, Times: 1},
		}
		af := gomonkey.ApplyFuncSeq(hotp.ValidateCustom, outputs)
		defer af.Reset()
		convey.So(otp.VerifyHOTPCustom("996554", 1, "JBSWY3DPEHPK3PXP", nil), convey.ShouldBeTrue)
		convey.So(otp.VerifyHOTPCustom("602287", 2, "JBSWY3DPEHPK3PXP", nil), convey.ShouldBeTrue)
	})
}
