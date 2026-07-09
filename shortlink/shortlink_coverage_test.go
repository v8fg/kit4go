package shortlink

import (
	"crypto/rand"
	"errors"
	"io"
	"math"
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

// errRandReader is an io.Reader that always fails, used to exercise the
// crypto/rand error branches in randomCode and Generate. crypto/rand.Reader is
// a package-level var; swapping it is a white-box technique that requires no
// change to production code.
type errRandReader struct{}

func (errRandReader) Read(p []byte) (int, error) { return 0, io.ErrUnexpectedEOF }

// withFailingRand swaps crypto/rand.Reader for a failing reader for the
// duration of fn, restoring the original on return. It serializes access to
// the process-global reader via the test framework (tests using this helper
// must not run in parallel with each other).
func withFailingRand(t *testing.T, fn func()) {
	t.Helper()
	orig := rand.Reader
	rand.Reader = errRandReader{}
	defer func() { rand.Reader = orig }()
	fn()
}

// TestRandomCode_RandError covers randomCode's defensive branch where
// rand.Int fails (crypto/rand.Reader returns an error).
func TestRandomCode_RandError(t *testing.T) {
	s := New(WithCodeLength(6))
	withFailingRand(t, func() {
		_, err := s.randomCode()
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("expected io.ErrUnexpectedEOF from randomCode, got %v", err)
		}
	})
}

// TestGenerate_RandError covers Generate's branch where randomCode fails and
// the error propagates immediately (no retry, no store call).
func TestGenerate_RandError(t *testing.T) {
	// failStore's Save must never be called when randomCode already failed —
	// if it were, the test fails, proving Generate short-circuits on the
	// randomCode error.
	store := &recordingStore{}
	s := New(WithStore(store), WithCodeLength(6))
	withFailingRand(t, func() {
		_, err := s.Generate("https://example.com")
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("expected io.ErrUnexpectedEOF from Generate, got %v", err)
		}
	})
	if store.saves != 0 {
		t.Fatalf("expected 0 Save calls after randomCode failure, got %d", store.saves)
	}
}

// recordingStore counts Save calls and always succeeds (used to prove Generate
// never reaches Save when randomCode errors).
type recordingStore struct {
	saves int
	m     map[string]string
}

func (s *recordingStore) Save(code, url string) error {
	s.saves++
	if s.m == nil {
		s.m = make(map[string]string)
	}
	s.m[code] = url
	return nil
}

func (s *recordingStore) Load(code string) (string, bool) {
	url, ok := s.m[code]
	return url, ok
}

// --- Regression tests for R17 quality-review findings ---

// TestDecodeBaseN_Overflow regression for the decodeBaseN overflow bug (P2).
// Before the fix, a code >= ~12 base62 chars silently wrapped uint64, yielding
// a wrong ID with no error. A 20-char code overflows by a wide margin and must
// now be rejected rather than wrapping.
func TestDecodeBaseN_Overflow(t *testing.T) {
	s := NewIDShortener(Alphabet, 0)
	// 20 base62 chars: 62^20 >> 2^64, so this MUST overflow.
	code := strings.Repeat("z", 20)
	_, err := s.Decode(code)
	if err == nil {
		t.Fatalf("Decode(%q): expected overflow error, got nil (uint64 wrapped silently)", code)
	}
	if !strings.Contains(err.Error(), "overflow") {
		t.Fatalf("Decode(%q): error should mention overflow, got %v", code, err)
	}
}

// TestDecodeBaseN_NearOverflowValid ensures a code that fits in uint64 (just
// under the boundary) still decodes correctly — the guard must not be off-by-
// one and reject legitimate maximum-magnitude IDs.
func TestDecodeBaseN_NearOverflowValid(t *testing.T) {
	s := NewIDShortener(Alphabet, 0)
	// Encode(math.MaxUint64) is the largest representable ID; its code MUST
	// round-trip. This proves the overflow check only rejects codes BEYOND
	// uint64, not the maximum valid one.
	maxID := uint64(math.MaxUint64)
	code := s.Encode(maxID)
	decoded, err := s.Decode(code)
	if err != nil {
		t.Fatalf("Decode(MaxUint64 code %q): %v", code, err)
	}
	if decoded != maxID {
		t.Fatalf("Decode(MaxUint64 code %q) = %d, want %d", code, decoded, maxID)
	}
}

// TestWithAlphabet_DuplicateCharsIgnored regression for the duplicate-alphabet
// bug (P2). Duplicates break Encode/Decode bijectivity, so WithAlphabet must
// reject an alphabet with repeated characters (falling back to the default).
func TestWithAlphabet_DuplicateCharsIgnored(t *testing.T) {
	const dup = "AABCDEF" // 'A' repeats -> would break bijectivity
	s := New(WithAlphabet(dup))
	if s.cfg.alphabet != Alphabet {
		t.Fatalf("WithAlphabet(%q): expected fallback to default Alphabet, got %q", dup, s.cfg.alphabet)
	}
	// Default must still produce valid codes.
	code, err := s.Generate("https://example.com")
	if err != nil {
		t.Fatalf("Generate after duplicate-alphabet fallback: %v", err)
	}
	if len(code) != defaultCodeLen {
		t.Fatalf("code length = %d, want %d", len(code), defaultCodeLen)
	}
}

// TestWithAlphabet_UniqueAccepted confirms a valid unique alphabet is still
// accepted (the duplicate check must not over-reject).
func TestWithAlphabet_UniqueAccepted(t *testing.T) {
	const uniq = "0123456789abcdefghij" // 20 distinct chars
	s := New(WithAlphabet(uniq))
	if s.cfg.alphabet != uniq {
		t.Fatalf("WithAlphabet(%q): expected it accepted, got %q", uniq, s.cfg.alphabet)
	}
}

// TestNewIDShortener_DuplicateAlphabet regression for NewIDShortener: a
// duplicate-character alphabet must fall back to the default (duplicates would
// make Encode non-injective and Decode ambiguous).
func TestNewIDShortener_DuplicateAlphabet(t *testing.T) {
	const dup = "ABCA" // 'A' repeats
	s := NewIDShortener(dup, 0)
	if s.alphabet != Alphabet {
		t.Fatalf("NewIDShortener(%q): expected fallback to default Alphabet, got %q", dup, s.alphabet)
	}
	// Round-trip on the fallback alphabet must work.
	code := s.Encode(42)
	id, err := s.Decode(code)
	if err != nil {
		t.Fatalf("Decode after fallback: %v", err)
	}
	if id != 42 {
		t.Fatalf("round-trip 42 -> %q -> %d", code, id)
	}
}

// TestNewIDShortener_UniqueAlphabetAccepted confirms a valid unique alphabet is
// accepted by NewIDShortener.
func TestNewIDShortener_UniqueAlphabetAccepted(t *testing.T) {
	const uniq = "0123456789" // 10 distinct chars (base10)
	s := NewIDShortener(uniq, 0)
	if s.alphabet != uniq {
		t.Fatalf("NewIDShortener(%q): expected it accepted, got %q", uniq, s.alphabet)
	}
	// base10 round-trip sanity: 10 -> "10".
	if got := s.Encode(10); got != "10" {
		t.Fatalf("Encode(10) in base10 = %q, want %q", got, "10")
	}
}

// TestWithRetries_Configurable covers WithRetries: the configured value is
// applied and used as the retry count.
func TestWithRetries_Configurable(t *testing.T) {
	// store that fails 5 times then succeeds; default retries (3) would
	// exhaust before success, but WithRetries(5) must succeed.
	store := &collisionStore{failuresRemaining: 5}
	s := New(WithStore(store), WithCodeLength(6), WithRetries(5))
	code, err := s.Generate("https://example.com")
	if err != nil {
		t.Fatalf("WithRetries(5): expected success after 5 retries, got %v", err)
	}
	if code == "" {
		t.Fatal("expected non-empty code")
	}
}

// TestWithRetries_NegativeClamped covers WithRetries clamping n < 0.
func TestWithRetries_NegativeClamped(t *testing.T) {
	s := New(WithRetries(-1))
	if s.retries != defaultRetries {
		t.Fatalf("WithRetries(-1): expected default %d, got %d", defaultRetries, s.retries)
	}
}

// TestGenerate_CodeSpaceExhaustedSentinel regression for the saturation bug
// (P1). When all retries collide, Generate must return the DISTINCT
// ErrCodeSpaceExhausted sentinel (not just a bare ErrCollision) so callers can
// tell a saturating code space from a transient collision. It still wraps
// ErrCollision, so errors.Is(err, ErrCollision) also holds for compatibility.
func TestGenerate_CodeSpaceExhaustedSentinel(t *testing.T) {
	// alwaysCollideStore: every Save collides -> exhaustion after retries+1.
	s := New(WithStore(&alwaysCollideStore{}), WithCodeLength(6))
	_, err := s.Generate("https://example.com")
	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}
	if !errors.Is(err, ErrCodeSpaceExhausted) {
		t.Fatalf("expected ErrCodeSpaceExhausted sentinel, got %v", err)
	}
	// Backwards-compatible: the underlying ErrCollision is still detectable.
	if !errors.Is(err, ErrCollision) {
		t.Fatalf("expected wrapped ErrCollision, got %v", err)
	}
}

// TestGenerate_SmallCodeSpaceExhaustion proves the saturation path triggers on
// a genuinely small code space (codeLen=1 -> 62 codes). Once the space is FULL,
// every random draw collides and Generate must surface the DISTINCT
// ErrCodeSpaceExhausted sentinel (not a bare collision). The store is filled
// directly to guarantee saturation deterministically — filling via Generate
// would itself collide probabilistically near the end of the space.
func TestGenerate_SmallCodeSpaceExhaustion(t *testing.T) {
	store := NewMemoryStore()
	s := New(WithStore(store), WithCodeLength(1), WithRetries(3))
	// codeLen=1 over the 62-symbol Alphabet => exactly 62 codes possible.
	// Occupy every slot directly so the space is provably saturated.
	for _, c := range Alphabet {
		if err := store.Save(string(c), "https://example.com"); err != nil {
			t.Fatalf("pre-fill %q: %v", string(c), err)
		}
	}
	// The space is now full; every Generate draw collides and, after retries,
	// must return ErrCodeSpaceExhausted (not a bare collision).
	_, err := s.Generate("https://example.com/overflow")
	if err == nil {
		t.Fatal("expected error once code space is full")
	}
	if !errors.Is(err, ErrCodeSpaceExhausted) {
		t.Fatalf("expected ErrCodeSpaceExhausted for full code space, got %v", err)
	}
	// Backwards-compat: the wrapped ErrCollision is still detectable.
	if !errors.Is(err, ErrCollision) {
		t.Fatalf("expected wrapped ErrCollision, got %v", err)
	}
}
