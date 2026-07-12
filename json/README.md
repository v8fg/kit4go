# Support json packages

- [x] [**standard json**](https://pkg.go.dev/encoding/json)
- [x] [**json-iterator**](https://github.com/json-iterator/go)
- [x] [**go_json**](https://github.com/goccy/go-json)
- [x] [**sonic**](https://github.com/bytedance/sonic)

## Build

- build tags: `['', 'jsoniter', 'go_json', 'sonic avx']`

> build with your json tag

- `go build -tags`
- `go build -tags jsoniter`
- `go build -tags go_json`
- `go build -tags "sonic avx"`

>**sonic requires `GOARCH=amd64`**: its build constraint gates on the amd64
>architecture, so `-tags "sonic avx"` on arm64 (e.g. Apple Silicon) is a silent
>no-op — the build falls back to stdlib with no warning. Use jsoniter or go_json
>for a faster backend on arm64.

>If run the test, also can use `go test -v -tags jsoniter .`

## Usage

Replace import statement from `encoding/json` to `github.com/v8fg/kit4go/json`.

```
-import "encoding/json"
+import "github.com/v8fg/kit4go/json"
```

### Example

```go
// go build -tags jsoniter
// go build -tags go_json
// go build -tags "sonic avx"

import "github.com/v8fg/kit4go/json"

var data YourSchema

// current json pkg
curJsonPKG := json.PKG

// Marshal
output, err := json.Marshal(&data)

// Unmarshal
err := json.Unmarshal(output, &data)
```
