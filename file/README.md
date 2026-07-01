# file: file & directory helpers

Existence checks, copy, and listing. The `FS` interface mirrors the signatures
so the file-touching logic is mockable in tests (mock generated via mockery).

## Usage

- `IsDir(path string) bool`.
- `IsFile(path string) bool`.
- `IsExist(path string) bool`.
- `CreateIfNotExists(path string, isFile bool) error` make a file or directory.
- `CopyFile(src, dst string) error`.
- `CopyDir(src, dst string) error` recursive.
- `ListFiles(dir string, fileType Type) ([]string, error)` filter by `Type`.
- `InfoStr(file string) string` human-readable file info.

`Type` selects which entries `ListFiles` returns (files, dirs, or both).

## Example

```go
import "github.com/v8fg/kit4go/file"

file.CreateIfNotExists("/var/data/cache", false)
files, _ := file.ListFiles("/var/log", file.Type(0))
_ = file.CopyFile("/a/src.txt", "/b/dst.txt")
```
