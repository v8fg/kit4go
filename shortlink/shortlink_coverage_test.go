package shortlink

import (
	"errors"
	"strings"
	"testing"
)

// TestWithAlphabet_Applied covers WithAlphabet's branch where len > 1.
func TestWithAlphabet_Applied(t *testing.T) {
	const alpha = "ABCDEFGH"
	s := New(WithAlphabet(alpha), WithCodeLength(4))
	code, err := s.Generate("https://example.com/x")
	if err != nil {
		t.Fatal(err)
	}
	if len(code) != 4 {
		t.Fatalf("code length = %d, want 4", len(code))
	}
	for _, c := range code {
		if !strings.ContainsRune(alpha, c) {
			t.Fatalf("code %q contains char %q not in alphabet %q", code, c, alpha)
		}
	}
}

// TestWithAlphabet_TooShortIgnored covers WithAlphabet's branch where len <= 1
// (the option is ignored and the default alphabet is used).
func TestWithAlphabet_TooShortIgnored(t *testing.T) {
	s := New(WithAlphabet("X")) // length 1 -> ignored
	if s.cfg.alphabet != Alphabet {
		t.Fatalf("expected default alphabet, got %q", s.cfg.alphabet)
	}
	// Generate must still work using the default alphabet.
	code, err := s.Generate("https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(code) != defaultCodeLen {
		t.Fatalf("code length = %d, want %d", len(code), defaultCodeLen)
	}
}

// collisionStore deterministically fails the first N Save calls with
// ErrCollision, then succeeds — exercising Generate's retry loop.
type collisionStore struct {
	failuresRemaining int
	m                 map[string]string
}

func (s *collisionStore) Save(code, url string) error {
	if s.failuresRemaining > 0 {
		s.failuresRemaining--
		return ErrCollision
	}
	if s.m == nil {
		s.m = make(map[string]string)
	}
	if _, exists := s.m[code]; exists {
		return ErrCollision
	}
	s.m[code] = url
	return nil
}

func (s *collisionStore) Load(code string) (string, bool) {
	url, ok := s.m[code]
	return url, ok
}

// TestGenerate_RetriesOnCollision covers the retry loop in Generate using a
// store that returns ErrCollision a fixed number of times before succeeding.
func TestGenerate_RetriesOnCollision(t *testing.T) {
	store := &collisionStore{failuresRemaining: 2}
	s := New(WithStore(store), WithCodeLength(6))
	code, err := s.Generate("https://example.com/collide")
	if err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}
	if code == "" {
		t.Fatal("expected non-empty code")
	}
	url, err := s.Resolve(code)
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://example.com/collide" {
		t.Fatalf("resolved %q", url)
	}
}

// nonCollisionErrorStore returns a non-collision error on Save.
type nonCollisionErrorStore struct{ err error }

func (s *nonCollisionErrorStore) Save(code, url string) error     { return s.err }
func (s *nonCollisionErrorStore) Load(code string) (string, bool) { return "", false }

// TestGenerate_NonCollisionErrorNoRetry covers the branch where a non-collision
// store error aborts Generate immediately (no retry).
func TestGenerate_NonCollisionErrorNoRetry(t *testing.T) {
	customErr := errors.New("disk full")
	s := New(WithStore(&nonCollisionErrorStore{err: customErr}), WithCodeLength(6))
	_, err := s.Generate("https://example.com")
	if !errors.Is(err, customErr) {
		t.Fatalf("expected custom error, got %v", err)
	}
}

// alwaysCollideStore fails every Save with ErrCollision — exercising the
// retry-exhaustion path of Generate.
type alwaysCollideStore struct{}

func (s *alwaysCollideStore) Save(code, url string) error     { return ErrCollision }
func (s *alwaysCollideStore) Load(code string) (string, bool) { return "", false }

// TestGenerate_CollisionExhausted covers the retry-exhaustion branch of
// Generate: after retries+1 attempts, the last error (ErrCollision) is returned.
func TestGenerate_CollisionExhausted(t *testing.T) {
	s := New(WithStore(&alwaysCollideStore{}), WithCodeLength(6))
	_, err := s.Generate("https://example.com")
	if !errors.Is(err, ErrCollision) {
		t.Fatalf("expected ErrCollision after retries exhausted, got %v", err)
	}
}

// TestWithCodeLength_NonPositive covers WithCodeLength's branch where n <= 0
// (ignored, default applies).
func TestWithCodeLength_NonPositive(t *testing.T) {
	s := New(WithCodeLength(0), WithCodeLength(-5))
	if s.cfg.codeLen != defaultCodeLen {
		t.Fatalf("expected default code length %d, got %d", defaultCodeLen, s.cfg.codeLen)
	}
}

// TestNewIDShortener_DefaultAlphabet covers the fallback to the default
// alphabet when the supplied alphabet is too short (len <= 1).
func TestNewIDShortener_DefaultAlphabet(t *testing.T) {
	s := NewIDShortener("", 0)
	if s.alphabet != Alphabet {
		t.Fatalf("expected default alphabet, got %q", s.alphabet)
	}
	s1 := NewIDShortener("X", 0)
	if s1.alphabet != Alphabet {
		t.Fatalf("expected default alphabet for len-1 input, got %q", s1.alphabet)
	}
}

// TestNewIDShortener_EncodeZero covers encodeBaseN's id==0 branch (returns the
// first character of the alphabet).
func TestNewIDShortener_EncodeZero(t *testing.T) {
	s := NewIDShortener(Alphabet, 0)
	code := s.Encode(0)
	if code != string(Alphabet[0]) {
		t.Fatalf("Encode(0) = %q, want %q", code, string(Alphabet[0]))
	}
}

// TestIDShortener_DecodeEmpty covers decodeBaseN with an empty code (returns 0,
// no error).
func TestIDShortener_DecodeEmpty(t *testing.T) {
	s := NewIDShortener(Alphabet, 0)
	id, err := s.Decode("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 0 {
		t.Fatalf("Decode(\"\") = %d, want 0", id)
	}
}
