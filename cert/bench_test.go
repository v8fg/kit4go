package cert

import (
	"context"
	"testing"
	"time"
)

func BenchmarkSplitCertKey(b *testing.B) {
	c := selfSignedCert(b, "example.com", true, 90*24*time.Hour)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = splitCertKey(c)
	}
}

func BenchmarkDirWriter_Write(b *testing.B) {
	dir := b.TempDir()
	cert := selfSignedCert(b, "example.com", true, 90*24*time.Hour)
	certPEM, keyPEM, _ := splitCertKey(cert)
	w := &osDirWriter{dir: dir}
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = w.Write(ctx, "example.com", certPEM, keyPEM)
	}
}
