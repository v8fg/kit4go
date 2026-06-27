package log4go

import (
	"hash/fnv"
	"math"
	"strconv"
	"strings"
)

// SamplingStrategy decides whether a record — identified by its correlation id
// (trace_id / request_id / device_id) — is actually logged (shipped). It is a
// PURE FUNCTION of the id: every service/language that sees the same id reaches
// the same verdict, so a request's whole chain is kept-or-dropped TOGETHER
// across services (no fragmentation), without coordination or propagation.
//
// A strategy is evaluated once per request (at WithContext) and the verdict is
// cached on the request-scoped logger, so the delivery hot path pays only an
// atomic-bool load (see P1b wiring). Built-in strategies mirror OpenTelemetry:
// FullSampling (default) and TraceIDRatioBased (OTel TraceIDRatioBased
// semantics). Install via SetSamplingStrategy.
type SamplingStrategy interface {
	ShouldLog(id string) bool
}

// FullSampling keeps every record. It is the default (dev/test, and any track
// that must not be sampled, e.g. the business/audit trail).
type FullSampling struct{}

// ShouldLog always returns true.
func (FullSampling) ShouldLog(string) bool { return true }

// TraceIDRatioBased keeps a record with probability Ratio, decided
// deterministically by the id — the OpenTelemetry TraceIDRatioBased algorithm.
//
// The id is reduced to a uint64 (see idUint64) and kept when
//
//	float64(v) < Ratio * float64(math.MaxUint64)
//
// No hash is needed when the id is a random hex (a W3C trace_id is 128-bit
// random), so the decision is uniform and high-precision; for non-hex ids a
// fixed, portable hash (FNV-1a) is used. The formula is documented so language
// ports (Java/Python) that reduce the id the same way decide identically —
// matching the OTel SDKs, which implement TraceIDRatioBased consistently across
// languages.
type TraceIDRatioBased struct {
	Ratio float64
}

// ShouldLog reports whether the id is sampled under the ratio.
func (s TraceIDRatioBased) ShouldLog(id string) bool {
	switch {
	case s.Ratio >= 1:
		return true
	case s.Ratio <= 0:
		return false
	}
	v, ok := idUint64(id)
	if !ok {
		return true // no/invalid id: keep (never drop on bad input)
	}
	return float64(v) < s.Ratio*float64(math.MaxUint64)
}

// TailDigitSampling keeps a record when idUint64(id) % Modulus < Keep — the
// classic "hash(request_id) % N < K" pattern (e.g. Modulus=10, Keep=3 ⇒ ~30%).
// It MUST use a fixed, portable hash (idUint64 uses hex-then-FNV-1a) so every
// language port agrees; language-builtin hashes differ and would fragment
// chains. Coarser than TraceIDRatioBased; prefer the latter when fine ratios
// matter.
type TailDigitSampling struct {
	Modulus uint64
	Keep    uint64
}

// ShouldLog reports whether the id falls in the kept tail bucket.
func (s TailDigitSampling) ShouldLog(id string) bool {
	if s.Modulus == 0 {
		return true
	}
	v, ok := idUint64(id)
	if !ok {
		return true
	}
	return v%s.Modulus < s.Keep
}

// idUint64 reduces a correlation id to a uint64 in a portable, cross-language
// way:
//   - If the id is (or starts with) >= 16 hex digits, parse the first 16 as a
//     64-bit value (a W3C trace_id is 32 hex digits; we use its high 64 bits).
//   - Otherwise hash it with FNV-1a 64 (a fixed, well-specified hash that every
//     language implements identically).
//
// Language ports MUST replicate this exact reduction (hex-first-16 / FNV-1a-64)
// so the same id maps to the same uint64 everywhere.
func idUint64(id string) (uint64, bool) {
	if id == "" {
		return 0, false
	}
	// Fast path: hex id (W3C trace_id / UUID without dashes / hex request id).
	hex := id
	if len(hex) > 16 {
		hex = hex[:16]
	}
	if isHex(hex) && len(hex) >= 8 { // enough hex to be meaningful
		if len(hex) < 16 {
			hex = strings.Repeat("0", 16-len(hex)) + hex
		}
		if v, err := strconv.ParseUint(hex, 16, 64); err == nil {
			return v, true
		}
	}
	// Fallback: FNV-1a 64 (portable, deterministic across languages).
	h := fnv.New64a()
	_, _ = h.Write([]byte(id))
	return h.Sum64(), true
}

// isHex reports whether s consists only of [0-9a-fA-F].
func isHex(s string) bool {
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return len(s) > 0
}
