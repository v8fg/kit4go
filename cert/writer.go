package cert

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
)

// DirWriter writes a certificate chain and private key for a domain to a
// directory as <domain>.crt and <domain>.key. The default implementation
// (osDirWriter) does so atomically with strict permissions; tests inject a
// mockery mock.
//
//go:generate mockery --name DirWriter --inpackage --with-expecter --filename mock_DirWriter.go
type DirWriter interface {
	// Write persists certPEM (<domain>.crt) and keyPEM (<domain>.key) to the
	// configured directory.
	Write(ctx context.Context, domain string, certPEM, keyPEM []byte) error
}

// osDirWriter writes files into a single directory, atomically.
type osDirWriter struct {
	dir string
}

// Write atomically writes certPEM to <dir>/<domain>.crt (0644) and keyPEM to
// <dir>/<domain>.key (0600). Each file is written to a temp sibling and renamed
// into place, so readers (e.g. nginx) never observe a partially-written file.
// The directory is created with 0700 if missing.
func (w *osDirWriter) Write(_ context.Context, domain string, certPEM, keyPEM []byte) error {
	if err := os.MkdirAll(w.dir, 0o700); err != nil {
		return fmt.Errorf("cert: mkdir %s: %w", w.dir, err)
	}
	if err := writeFileAtomic(filepath.Join(w.dir, domain+".crt"), certPEM, 0o644); err != nil {
		return err
	}
	if err := writeFileAtomic(filepath.Join(w.dir, domain+".key"), keyPEM, 0o600); err != nil {
		return err
	}
	return nil
}

// writeFileAtomic writes data to finalPath via a temp file in the same directory
// (so os.Rename is atomic on POSIX), fsyncs, sets perm, then renames into place.
// On any error the temp file is removed and the cause is wrapped.
func writeFileAtomic(finalPath string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(finalPath)
	base := filepath.Base(finalPath)
	// os.CreateTemp creates the file with O_EXCL uniqueness and 0600; the temp
	// name carries the base so the final rename stays on the same filesystem.
	tmp, err := os.CreateTemp(dir, "."+base+".tmp-*")
	if err != nil {
		return fmt.Errorf("cert: create temp for %s: %w", base, err)
	}
	tmpName := tmp.Name()
	removeTmp := func() { _ = os.Remove(tmpName) }

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		removeTmp()
		return fmt.Errorf("cert: write %s: %w", base, err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		removeTmp()
		return fmt.Errorf("cert: sync %s: %w", base, err)
	}
	if err := tmp.Close(); err != nil {
		removeTmp()
		return fmt.Errorf("cert: close %s: %w", base, err)
	}
	// Set the intended permission explicitly. CreateTemp defaults to 0600; gosec
	// (G306) expects an explicit mode on private-key writes, so we always Chmod
	// before the rename rather than relying on the create-time mode.
	if err := os.Chmod(tmpName, perm); err != nil {
		removeTmp()
		return fmt.Errorf("cert: chmod %s: %w", base, err)
	}
	if err := os.Rename(tmpName, finalPath); err != nil {
		removeTmp()
		return fmt.Errorf("cert: rename %s: %w", base, err)
	}
	return nil
}

// splitCertKey renders a *tls.Certificate as standard PEM blocks: the full
// certificate chain (leaf first) as CERTIFICATE blocks and the private key as a
// PKCS#8 PRIVATE KEY block. PKCS#8 is type-agnostic, so the same code handles
// ECDSA and RSA keys.
func splitCertKey(c *tls.Certificate) (certPEM, keyPEM []byte, err error) {
	if c == nil || len(c.Certificate) == 0 {
		return nil, nil, fmt.Errorf("cert: empty certificate chain")
	}
	var certBuf bytes.Buffer
	for _, der := range c.Certificate {
		if err := pem.Encode(&certBuf, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
			return nil, nil, fmt.Errorf("cert: encode certificate: %w", err)
		}
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(c.PrivateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("cert: marshal private key: %w", err)
	}
	var keyBuf bytes.Buffer
	if err := pem.Encode(&keyBuf, &pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}); err != nil {
		return nil, nil, fmt.Errorf("cert: encode private key: %w", err)
	}
	return certBuf.Bytes(), keyBuf.Bytes(), nil
}
