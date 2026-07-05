# shortlink

Short-link code generation and resolution. Two strategies: random codes (CSPRNG, collision-retry, pluggable Store) and sequential ID encoding (base62, deterministic, collision-free). Pure standard library.

## Usage

- `New(opts ...Option) *Shortener` — random-code shortener (default 6 chars).
- `NewIDShortener(alphabet, startID) *IDShortener` — sequential base62 shortener.
- `NewMemoryStore() *MemoryStore` — in-memory store (default; implement `Store` for Redis/DB).
- Options: `WithCodeLength`, `WithAlphabet`, `WithStore`.

## Example

```go
s := shortlink.New(shortlink.WithCodeLength(8))
code, _ := s.Generate("https://example.com/long/url")
url, _ := s.Resolve(code) // → original URL

id := shortlink.NewIDShortener(shortlink.Alphabet, 10000)
code := id.Next() // deterministic base62 from sequential counter
```
