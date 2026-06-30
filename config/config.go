// Package config assembles typed configuration from layered, read-only sources
// (environment variables, JSON files, programmatic maps) with a clear priority
// order.
//
// A Store holds an ordered list of Sources; the first Source that has a key
// wins, so callers express priority by construction order — typically
// env (highest), then file, then defaults. Typed getters (String/Int/Bool/
// Float64/Duration/IntSlice/StringSlice/Unmarshal) parse on demand and fall
// back to a caller-supplied default when the key is missing or fails to parse.
//
// Ad-tech uses: SSP endpoints, bidder timeouts, feature flags, pacing limits —
// values that differ per environment and are most naturally expressed as
// 12-factor env vars or a small JSON file.
package config

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"time"
)

// Source is a read-only configuration source. Lookup reports whether the key
// was present; the returned value is the raw string form.
type Source interface {
	Lookup(key string) (string, bool)
}

// Store resolves configuration from ordered Sources (index 0 = highest
// priority). The zero value is an empty store (every lookup misses).
type Store struct {
	sources []Source
}

// New builds a Store from the given sources in priority order (first wins).
func New(sources ...Source) *Store {
	return &Store{sources: sources}
}

// Has reports whether any source contains key.
func (s *Store) Has(key string) bool {
	_, ok := s.lookup(key)
	return ok
}

func (s *Store) lookup(key string) (string, bool) {
	for _, src := range s.sources {
		if v, ok := src.Lookup(key); ok {
			return v, true
		}
	}
	return "", false
}

// String returns the value of key, or def when missing.
func (s *Store) String(key, def string) string {
	if v, ok := s.lookup(key); ok {
		return v
	}
	return def
}

// Int parses key as an int; returns def when missing or unparseable.
func (s *Store) Int(key string, def int) int {
	if v, ok := s.lookup(key); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return def
}

// Int64 parses key as an int64; returns def when missing or unparseable.
func (s *Store) Int64(key string, def int64) int64 {
	if v, ok := s.lookup(key); ok {
		if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
			return n
		}
	}
	return def
}

// Bool parses key as a bool. Accepts 1/0, t/f, true/false, yes/no, on/off
// (case-insensitive); returns def when missing or unparseable.
func (s *Store) Bool(key string, def bool) bool {
	if v, ok := s.lookup(key); ok {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "t", "true", "yes", "on", "y":
			return true
		case "0", "f", "false", "no", "off", "n":
			return false
		}
	}
	return def
}

// Float64 parses key as a float64; returns def when missing or unparseable.
func (s *Store) Float64(key string, def float64) float64 {
	if v, ok := s.lookup(key); ok {
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			return f
		}
	}
	return def
}

// Duration parses key as a time.Duration (e.g. "250ms", "2s", "1m30s");
// returns def when missing or unparseable.
func (s *Store) Duration(key string, def time.Duration) time.Duration {
	if v, ok := s.lookup(key); ok {
		if d, err := time.ParseDuration(strings.TrimSpace(v)); err == nil {
			return d
		}
	}
	return def
}

// StringSlice splits key by sep into a trimmed string slice; returns def when
// missing. Empty fields are dropped.
func (s *Store) StringSlice(key, sep string, def []string) []string {
	if v, ok := s.lookup(key); ok {
		parts := strings.Split(v, sep)
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			if t := strings.TrimSpace(p); t != "" {
				out = append(out, t)
			}
		}
		if len(out) == 0 {
			return def
		}
		return out
	}
	return def
}

// IntSlice splits key by sep into an int slice; returns def when missing or if
// any field fails to parse.
func (s *Store) IntSlice(key, sep string, def []int) []int {
	if v, ok := s.lookup(key); ok {
		parts := strings.Split(v, sep)
		out := make([]int, 0, len(parts))
		for _, p := range parts {
			t := strings.TrimSpace(p)
			if t == "" {
				continue
			}
			n, err := strconv.Atoi(t)
			if err != nil {
				return def
			}
			out = append(out, n)
		}
		if len(out) == 0 {
			return def
		}
		return out
	}
	return def
}

// Unmarshal JSON-decodes key's value into dst. Returns the json error when
// present-but-unparseable, and os.ErrNotExist-equivalent (a sentinel) only
// indirectly: when missing it returns ErrMissing so callers can distinguish.
func (s *Store) Unmarshal(key string, dst any) error {
	v, ok := s.lookup(key)
	if !ok {
		return ErrMissing
	}
	return json.Unmarshal([]byte(v), dst)
}

// ErrMissing is returned by Unmarshal when no source has the key.
var ErrMissing = errMissing{}

type errMissing struct{}

func (errMissing) Error() string { return "config: key missing" }

// --- Sources ---

// EnvSource reads keys from the process environment. The optional prefix is
// applied (e.g. "app") and the key is normalized to upper snake-case:
// "redis.addr" with prefix "app" becomes "APP_REDIS_ADDR".
type EnvSource struct {
	prefix string
}

// Env returns an environment Source. Pass "" for no prefix.
func Env(prefix string) EnvSource { return EnvSource{prefix: prefix} }

// Lookup implements Source.
func (e EnvSource) Lookup(key string) (string, bool) {
	return os.LookupEnv(envName(e.prefix, key))
}

func envName(prefix, key string) string {
	key = strings.ToUpper(key)
	key = strings.NewReplacer(".", "_", "-", "_").Replace(key)
	if prefix == "" {
		return key
	}
	p := strings.ToUpper(strings.NewReplacer(".", "_", "-", "_").Replace(prefix))
	return p + "_" + key
}

// MapSource is an in-memory Source backed by a string map. Use it for defaults
// or test fixtures. The map is read as-is (no copying); do not mutate it after
// constructing the Source.
type MapSource map[string]string

// Lookup implements Source.
func (m MapSource) Lookup(key string) (string, bool) {
	v, ok := m[key]
	return v, ok
}

// FileSource is a Source backed by a flat JSON object mapping key paths to
// string values, e.g. {"redis.addr": "localhost:6379", "bidder.timeout": "2s"}.
type FileSource map[string]string

// FromFile parses a flat JSON file into a FileSource.
func FromFile(path string) (FileSource, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return FileSource(m), nil
}

// Lookup implements Source.
func (f FileSource) Lookup(key string) (string, bool) {
	v, ok := f[key]
	return v, ok
}
