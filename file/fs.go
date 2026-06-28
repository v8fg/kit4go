package file

import (
	"io"
	"io/fs"
	"os"
)

// FS is the filesystem subset used by CopyFile / CreateIfNotExists / InfoStr.
// The default implementation (osFS) bridges the standard library os/io calls.
// Tests may inject a mockery-generated mock to drive error-path coverage
// without relying on syscall patching (gomonkey).
//
// The exported package API (CopyFile, CreateIfNotExists, InfoStr, ...) keeps
// its original signatures; injection happens through the package-level
// DefaultFS variable, which can be swapped per-test and restored via defer.
type FS interface {
	// Stat mirrors os.Stat.
	Stat(name string) (fs.FileInfo, error)
	// Open mirrors os.Open.
	Open(name string) (*os.File, error)
	// MkdirAll mirrors os.MkdirAll.
	MkdirAll(path string, perm os.FileMode) error
	// Create mirrors os.Create.
	Create(name string) (*os.File, error)
	// OpenFile mirrors os.OpenFile.
	OpenFile(name string, flag int, perm os.FileMode) (*os.File, error)
	// Copy mirrors io.Copy (src -> dst). It is split out so tests can
	// deterministically inject an io.Copy failure without touching globals.
	Copy(dst io.Writer, src io.Reader) (written int64, err error)
}

// osFS is the default FS implementation delegating to the standard library.
type osFS struct{}

// Stat implements FS.
func (osFS) Stat(name string) (fs.FileInfo, error) { return os.Stat(name) }

// Open implements FS.
func (osFS) Open(name string) (*os.File, error) { return os.Open(name) }

// MkdirAll implements FS.
func (osFS) MkdirAll(path string, perm os.FileMode) error { return os.MkdirAll(path, perm) }

// Create implements FS.
func (osFS) Create(name string) (*os.File, error) { return os.Create(name) }

// OpenFile implements FS.
func (osFS) OpenFile(name string, flag int, perm os.FileMode) (*os.File, error) {
	return os.OpenFile(name, flag, perm)
}

// Copy implements FS.
func (osFS) Copy(dst io.Writer, src io.Reader) (int64, error) { return io.Copy(dst, src) }

// DefaultFS is the FS used by the package functions. It is a package-level
// variable so tests can temporarily replace it (defer restore) to inject
// failures. Production callers must not mutate it.
//
//go:generate mockery --name FS --inpackage --with-expecter --filename mock_FS.go
var DefaultFS FS = osFS{}
