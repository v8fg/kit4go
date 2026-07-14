package cert

import (
	"context"
	"crypto/tls"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
)

func TestSplitCertKey(t *testing.T) {
	convey.Convey("ecdsa cert renders parseable CERTIFICATE + PKCS8 PRIVATE KEY", t, func() {
		c := selfSignedCert(t, "a.com", true, 90*24*time.Hour)
		certPEM, keyPEM, err := splitCertKey(c)
		convey.So(err, convey.ShouldBeNil)
		convey.So(string(certPEM), convey.ShouldContainSubstring, "BEGIN CERTIFICATE")
		convey.So(string(keyPEM), convey.ShouldContainSubstring, "BEGIN PRIVATE KEY")
		_, err = tls.X509KeyPair(certPEM, keyPEM)
		convey.So(err, convey.ShouldBeNil)
	})
	convey.Convey("rsa cert round-trips through tls.X509KeyPair", t, func() {
		c := selfSignedCert(t, "a.com", false, 90*24*time.Hour)
		certPEM, keyPEM, err := splitCertKey(c)
		convey.So(err, convey.ShouldBeNil)
		_, err = tls.X509KeyPair(certPEM, keyPEM)
		convey.So(err, convey.ShouldBeNil)
	})
	convey.Convey("nil cert errors", t, func() {
		_, _, err := splitCertKey(nil)
		convey.So(err, convey.ShouldBeError)
	})
}

func TestOSDirWriter(t *testing.T) {
	domain := "example.com"
	cert := selfSignedCert(t, domain, true, 90*24*time.Hour)
	certPEM, keyPEM, err := splitCertKey(cert)
	if err != nil {
		t.Fatalf("splitCertKey: %v", err)
	}

	convey.Convey("writes .crt (0644) and .key (0600), creates dir, leaves no temp files", t, func() {
		dir := filepath.Join(t.TempDir(), "out") // absent → MkdirAll must create it
		w := &osDirWriter{dir: dir}

		convey.So(w.Write(context.Background(), domain, certPEM, keyPEM), convey.ShouldBeNil)

		cfi, err := os.Stat(filepath.Join(dir, domain+".crt"))
		convey.So(err, convey.ShouldBeNil)
		convey.So(cfi.Mode().Perm(), convey.ShouldEqual, os.FileMode(0o644))
		kfi, err := os.Stat(filepath.Join(dir, domain+".key"))
		convey.So(err, convey.ShouldBeNil)
		convey.So(kfi.Mode().Perm(), convey.ShouldEqual, os.FileMode(0o600))

		entries, err := os.ReadDir(dir)
		convey.So(err, convey.ShouldBeNil)
		convey.So(len(entries), convey.ShouldEqual, 2) // no leftover .tmp-* files
	})

	convey.Convey("re-writing the same domain replaces files atomically", t, func() {
		dir := t.TempDir()
		w := &osDirWriter{dir: dir}
		convey.So(w.Write(context.Background(), domain, certPEM, keyPEM), convey.ShouldBeNil)
		convey.So(w.Write(context.Background(), domain, certPEM, keyPEM), convey.ShouldBeNil)
		entries, _ := os.ReadDir(dir)
		convey.So(len(entries), convey.ShouldEqual, 2)
	})
}
