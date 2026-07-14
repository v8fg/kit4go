package cert

import (
	"crypto/tls"
	"errors"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
)

func TestConfigWithDefaults(t *testing.T) {
	convey.Convey("withDefaults fills zero fields and derives CacheDir from Dir", t, func() {
		c := Config{Domains: []string{"a.com"}, Dir: "/tmp/x"}.withDefaults()
		convey.So(c.RenewBefore, convey.ShouldEqual, 720*time.Hour)
		convey.So(c.CheckInterval, convey.ShouldEqual, time.Hour)
		convey.So(c.KeyType, convey.ShouldEqual, KeyTypeECDSA)
		convey.So(c.CacheDir, convey.ShouldEqual, "/tmp/x/.acme")
	})
	convey.Convey("non-zero fields are preserved", t, func() {
		c := Config{Dir: "/x", RenewBefore: time.Hour, KeyType: KeyTypeRSA, CacheDir: "/c"}.withDefaults()
		convey.So(c.RenewBefore, convey.ShouldEqual, time.Hour)
		convey.So(c.KeyType, convey.ShouldEqual, KeyTypeRSA)
		convey.So(c.CacheDir, convey.ShouldEqual, "/c")
	})
}

func TestConfigValidate(t *testing.T) {
	convey.Convey("missing domains", t, func() {
		err := Config{Dir: "/x"}.withDefaults().validate()
		convey.So(errors.Is(err, ErrNoDomains), convey.ShouldBeTrue)
	})
	convey.Convey("missing dir", t, func() {
		err := Config{Domains: []string{"a.com"}}.withDefaults().validate()
		convey.So(errors.Is(err, ErrNoDir), convey.ShouldBeTrue)
	})
	convey.Convey("invalid (single-label) domain", t, func() {
		err := Config{Domains: []string{"localhost"}, Dir: "/x"}.withDefaults().validate()
		convey.So(errors.Is(err, ErrInvalidDomain), convey.ShouldBeTrue)
	})
	convey.Convey("invalid key type", t, func() {
		c := Config{Domains: []string{"a.com"}, Dir: "/x", KeyType: "bogus"}.withDefaults()
		convey.So(errors.Is(c.validate(), ErrInvalidKeyType), convey.ShouldBeTrue)
	})
	convey.Convey("valid config passes", t, func() {
		convey.So(Config{Domains: []string{"a.com"}, Dir: "/x"}.withDefaults().validate(), convey.ShouldBeNil)
	})
}

func TestConfigDirectoryURL(t *testing.T) {
	convey.Convey("explicit DirectoryURL wins over Staging", t, func() {
		c := Config{DirectoryURL: "https://pebble:14000/dir", Staging: true}
		convey.So(c.directoryURL(), convey.ShouldEqual, "https://pebble:14000/dir")
	})
	convey.Convey("staging endpoint", t, func() {
		convey.So(Config{Staging: true}.directoryURL(), convey.ShouldEqual, LEStagingDirectoryURL)
	})
	convey.Convey("default production endpoint", t, func() {
		convey.So(Config{}.directoryURL(), convey.ShouldEqual, LEProdDirectoryURL)
	})
}

func TestClientHello(t *testing.T) {
	convey.Convey("ecdsa hello carries an ECDSA cipher suite", t, func() {
		h := clientHello("a.com", KeyTypeECDSA)
		convey.So(h.ServerName, convey.ShouldEqual, "a.com")
		convey.So(h.CipherSuites, convey.ShouldContain, tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256)
	})
	convey.Convey("rsa hello carries an RSA cipher suite and no ECDSA suite", t, func() {
		h := clientHello("a.com", KeyTypeRSA)
		convey.So(h.CipherSuites, convey.ShouldContain, tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256)
		convey.So(h.CipherSuites, convey.ShouldNotContain, tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256)
	})
}
