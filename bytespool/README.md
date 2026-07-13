# bytespool

Size-classed `*bytes.Buffer` pool — avoids GC pressure on hot-path serialization
(log formatting, JSON encoding, HTTP response building). Pure standard library.

## Quick start

```go
import "github.com/v8fg/kit4go/bytespool"

// Get/Put pattern
b := bytespool.Get(256)
defer bytespool.Put(b)
b.WriteString("hello")

// WithBuffer (auto-put, panic-safe)
bytespool.WithBuffer(256, func(b *bytes.Buffer) {
    b.WriteString("hello")
    json.NewEncoder(b).Encode(data)
})
```

20 size classes from 64 B to 64 KiB (powers of 2). Buffers > 128 KiB are
discarded on Put (avoid retaining oversized buffers).
