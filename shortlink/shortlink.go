// Package shortlink generates and resolves short link codes. It supports two
// strategies: sequential ID encoding (deterministic, collision-free, base62) and
// random codes (non-deterministic, configurable length). A pluggable Store
// backs the code→URL mapping (in-memory by default; Redis/DB via the interface).
//
// Pure standard library. Ad-tech uses: creative tracking URLs, redirect
// endpoints, impression/click shorteners — anywhere a long URL needs a compact,
// shareable alias.
package shortlink

import (
	"crypto/rand"
	"errors"
	"math/big"
	"sync"
	"sync/atomic"
)

// ErrNotFound is returned by Resolve when the code is not in the store.
var ErrNotFound = errors.New("shortlink: code not found")

// ErrEmptyURL is returned by Generate when the URL is empty.
var ErrEmptyURL = errors.New("shortlink: url is required")

// ErrCollision is returned by Store.Save when the code already exists.
var ErrCollision = errors.New("shortlink: code collision")

const (
	// Alphabet is the default base62 character set for code generation.
	Alphabet       = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	defaultCodeLen = 6
)

// Store is the backing store for code→URL mappings. Implementations must be
// safe for concurrent use.
type Store interface {
	// Save stores the code→url mapping. Returns an error if the code already
	// exists (collision — the caller should retry with a new code).
	Save(code, url string) error
	// Load retrieves the URL for a code. Returns ("", false) if not found.
	Load(code string) (string, bool)
}

// Option configures a Shortener.
type Option func(*config)

type config struct {
	codeLen  int
	alphabet string
	store    Store
}

// WithCodeLength sets the random code length (default 6). Ignored by IDShortener
// (which encodes a sequential ID).
func WithCodeLength(n int) Option {
	return func(c *config) {
		if n > 0 {
			c.codeLen = n
		}
	}
}

// WithAlphabet overrides the base62 alphabet (must be non-empty and unique).
func WithAlphabet(s string) Option {
	return func(c *config) {
		if len(s) > 1 {
			c.alphabet = s
		}
	}
}

// WithStore sets the backing store (default: in-memory).
func WithStore(s Store) Option {
	return func(c *config) { c.store = s }
}

func defaults(c *config) {
	if c.codeLen <= 0 {
		c.codeLen = defaultCodeLen
	}
	if c.alphabet == "" {
		c.alphabet = Alphabet
	}
	if c.store == nil {
		c.store = NewMemoryStore()
	}
}

// --- Shortener (random codes) ---

// Shortener generates random short codes and resolves them via a Store. Safe
// for concurrent use.
type Shortener struct {
	cfg     config
	retries int
}

// New builds a random-code Shortener.
func New(opts ...Option) *Shortener {
	c := config{}
	for _, opt := range opts {
		opt(&c)
	}
	defaults(&c)
	return &Shortener{cfg: c, retries: 3}
}

// Generate creates a short code for url and stores it. On a code collision (a
// Save failure indicating the code exists), it retries up to 3 times with a new
// random code before returning the store's error.
func (s *Shortener) Generate(url string) (string, error) {
	if url == "" {
		return "", ErrEmptyURL
	}
	var lastErr error
	for attempt := 0; attempt <= s.retries; attempt++ {
		code, err := s.randomCode()
		if err != nil {
			return "", err
		}
		err = s.cfg.store.Save(code, url)
		if err == nil {
			return code, nil
		}
		if !errors.Is(err, ErrCollision) {
			return "", err // non-collision store error — don't retry
		}
		lastErr = err
	}
	return "", lastErr
}

// Resolve returns the original URL for a code, or ErrNotFound.
func (s *Shortener) Resolve(code string) (string, error) {
	url, ok := s.cfg.store.Load(code)
	if !ok {
		return "", ErrNotFound
	}
	return url, nil
}

func (s *Shortener) randomCode() (string, error) {
	alpha := s.cfg.alphabet
	n := len(alpha)
	code := make([]byte, s.cfg.codeLen)
	for i := range code {
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
		if err != nil {
			return "", err
		}
		code[i] = alpha[idx.Int64()]
	}
	return string(code), nil
}

// --- IDShortener (sequential, deterministic, collision-free) ---

// IDShortener encodes a monotonically increasing counter as a base62 code. The
// codes are deterministic, collision-free, and short — ideal when you control
// the ID space (e.g., a DB auto-increment). It does NOT use the Store (the
// caller manages the ID↔URL mapping in their own DB).
type IDShortener struct {
	alphabet string
	counter  atomic.Uint64
}

// NewIDShortener builds an ID-based shortener. startID sets the initial counter
// (use a non-zero value to avoid very short codes at the beginning).
func NewIDShortener(alphabet string, startID uint64) *IDShortener {
	if len(alphabet) <= 1 {
		alphabet = Alphabet
	}
	s := &IDShortener{alphabet: alphabet}
	s.counter.Store(startID)
	return s
}

// Encode converts a uint64 ID to a base62 code string.
func (s *IDShortener) Encode(id uint64) string {
	return encodeBaseN(id, s.alphabet)
}

// Decode converts a base62 code back to the uint64 ID. Returns an error if the
// code contains characters not in the alphabet.
func (s *IDShortener) Decode(code string) (uint64, error) {
	return decodeBaseN(code, s.alphabet)
}

// Next generates the next sequential code (increments the internal counter).
func (s *IDShortener) Next() string {
	return s.Encode(s.counter.Add(1))
}

// encodeBaseN converts a uint64 to a base-N string using the given alphabet.
func encodeBaseN(id uint64, alphabet string) string {
	if id == 0 {
		return string(alphabet[0])
	}
	n := uint64(len(alphabet))
	var buf [12]byte // 2^64 in base62 fits in 11 chars
	i := len(buf)
	for id > 0 {
		i--
		buf[i] = alphabet[id%n]
		id /= n
	}
	return string(buf[i:])
}

// decodeBaseN converts a base-N string back to uint64.
func decodeBaseN(code, alphabet string) (uint64, error) {
	n := uint64(len(alphabet))
	index := make(map[byte]uint64, n)
	for i := 0; i < len(alphabet); i++ {
		index[alphabet[i]] = uint64(i)
	}
	var result uint64
	for i := 0; i < len(code); i++ {
		val, ok := index[code[i]]
		if !ok {
			return 0, errors.New("shortlink: invalid character in code")
		}
		result = result*n + val
	}
	return result, nil
}

// --- MemoryStore ---

// Compile-time interface assertion: guard that MemoryStore stays in sync with
// the Store contract.
var _ Store = (*MemoryStore)(nil)

// MemoryStore is an in-memory, concurrent-safe Store.
type MemoryStore struct {
	mu sync.RWMutex
	m  map[string]string
}

// NewMemoryStore builds an empty MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{m: make(map[string]string)}
}

// Save stores the mapping. Returns ErrCollision if the code already exists.
func (s *MemoryStore) Save(code, url string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.m[code]; exists {
		return ErrCollision
	}
	s.m[code] = url
	return nil
}

// Load retrieves the URL. Returns ("", false) if not found.
func (s *MemoryStore) Load(code string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	url, ok := s.m[code]
	return url, ok
}
