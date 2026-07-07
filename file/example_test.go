package file_test

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/v8fg/kit4go/file"
)

// ExampleCreateIfNotExists shows the existence helpers: CreateIfNotExists
// makes a file or directory idempotently, and IsDir/IsFile/IsExist inspect a
// path. The example builds a small tree under a temp dir and prints the
// boolean classifications, which are deterministic.
func ExampleCreateIfNotExists() {
	root, err := os.MkdirTemp("", "kit4go-file-example-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(root)

	// Create a directory and a file that do not yet exist. Re-running on an
	// existing path is a no-op.
	dir := filepath.Join(root, "cache")
	if err := file.CreateIfNotExists(dir, false); err != nil {
		panic(err)
	}
	cfg := filepath.Join(dir, "app.yaml")
	if err := file.CreateIfNotExists(cfg, true); err != nil {
		panic(err)
	}

	fmt.Println("dir exists:", file.IsExist(dir))
	fmt.Println("is dir:   ", file.IsDir(dir))
	fmt.Println("cfg is dir:", file.IsDir(cfg))
	fmt.Println("cfg is file:", file.IsFile(cfg))
	fmt.Println("missing:  ", file.IsExist(filepath.Join(root, "nope")))
	// Output:
	// dir exists: true
	// is dir:    true
	// cfg is dir: false
	// cfg is file: true
	// missing:   false
}

// ExampleListFiles shows ListFiles with the Type filter plus CopyFile/CopyDir.
// A nested source tree is materialized under a temp dir, copied to a second
// temp dir, then the regular files inside the destination are listed. Because
// ListFiles returns sorted paths, the relative output is deterministic.
func ExampleListFiles() {
	src, err := os.MkdirTemp("", "kit4go-src-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(src)
	dst, err := os.MkdirTemp("", "kit4go-dst-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dst)

	// Materialize: src/a.txt, src/sub/b.txt
	if err := os.MkdirAll(filepath.Join(src, "sub"), 0o750); err != nil {
		panic(err)
	}
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("a"), 0o600); err != nil {
		panic(err)
	}
	if err := os.WriteFile(filepath.Join(src, "sub", "b.txt"), []byte("b"), 0o600); err != nil {
		panic(err)
	}

	// Recursively mirror src into dst, then list the regular files there.
	if err := file.CopyDir(src, dst); err != nil {
		panic(err)
	}
	files, err := file.ListFiles(dst, file.TypeFile)
	if err != nil {
		panic(err)
	}
	for _, f := range files {
		rel, _ := filepath.Rel(dst, f)
		fmt.Println(rel)
	}
	// Output:
	// a.txt
	// sub/b.txt
}
