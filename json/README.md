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

## Usage

Replace import statement from `encoding/json` to `github.com/goccy/go-json`.

```
-import "encoding/json"
+import "github.com/goccy/go-json"
```

### Example

```go
// go build -tags go_json

import "github.com/goccy/go-json"

var data YourSchema

// Marshal
output, err := json.Marshal(&data)

// Unmarshal
err := json.Unmarshal(output, &data)
```
