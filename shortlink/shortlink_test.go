package shortlink

import (
	"testing"
)

func TestShortener_GenerateAndResolve(t *testing.T) {
	s := New(WithCodeLength(6))
	code, err := s.Generate("https://example.com/very/long/url")
	if err != nil {
		t.Fatal(err)
	}
	if len(code) != 6 {
		t.Fatalf("code length = %d, want 6", len(code))
	}
	url, err := s.Resolve(code)
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://example.com/very/long/url" {
		t.Fatalf("resolved url = %q", url)
	}
}

func TestShortener_NotFound(t *testing.T) {
	s := New()
	_, err := s.Resolve("nonexistent")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestShortener_EmptyURL(t *testing.T) {
	s := New()
	_, err := s.Generate("")
	if err != ErrEmptyURL {
		t.Fatalf("expected ErrEmptyURL, got %v", err)
	}
}

func TestShortener_CollisionRetry(t *testing.T) {
	store := NewMemoryStore()
	// Pre-occupy a code to force a collision on the first Save attempt.
	// (Probabilistic — the test verifies retry logic works when Save fails.)
	s := New(WithStore(store), WithCodeLength(2))
	// Generate 100 codes — with length 2 (62^2 = 3844 space), collisions are
	// rare but the retry logic must handle them if they occur.
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		code, err := s.Generate("https://example.com")
		if err != nil {
			t.Fatalf("generate %d: %v", i, err)
		}
		if seen[code] {
			t.Fatalf("duplicate code %q after retry", code)
		}
		seen[code] = true
	}
}

func TestIDShortener_EncodeDecode(t *testing.T) {
	s := NewIDShortener(Alphabet, 0)
	cases := []uint64{0, 1, 61, 62, 63, 3844, 99999, 1 << 32, 1 << 63}
	for _, id := range cases {
		code := s.Encode(id)
		decoded, err := s.Decode(code)
		if err != nil {
			t.Fatalf("decode %d (%q): %v", id, code, err)
		}
		if decoded != id {
			t.Fatalf("round-trip %d → %q → %d", id, code, decoded)
		}
	}
}

func TestIDShortener_NextSequential(t *testing.T) {
	s := NewIDShortener(Alphabet, 99)
	c1 := s.Next() // encodes 100
	c2 := s.Next() // encodes 101
	if c1 == c2 {
		t.Fatal("sequential codes should differ")
	}
	id1, _ := s.Decode(c1)
	id2, _ := s.Decode(c2)
	if id2 != id1+1 {
		t.Fatalf("expected sequential IDs: %d, %d", id1, id2)
	}
}

func TestIDShortener_InvalidCode(t *testing.T) {
	s := NewIDShortener(Alphabet, 0)
	_, err := s.Decode("!@#")
	if err == nil {
		t.Fatal("expected error for invalid characters")
	}
}

func TestMemoryStore_ConcurrentSafe(t *testing.T) {
	store := NewMemoryStore()
	done := make(chan struct{})
	// Writer
	go func() {
		for i := 0; i < 100; i++ {
			_ = store.Save("c", "url")
		}
		close(done)
	}()
	// Reader
	for i := 0; i < 100; i++ {
		_, _ = store.Load("c")
	}
	<-done
}
