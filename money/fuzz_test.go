package money

import (
	"errors"
	"testing"
)

// FuzzMoneyParse asserts that Parse never panics on arbitrary (currency, amount)
// string pairs. Parse may legitimately return an error (unknown currency,
// unparseable amount, too many fractional digits, overflow) — it must never
// panic. The seed corpus covers the documented happy paths and edge cases.
func FuzzMoneyParse(f *testing.F) {
	corpora := []struct {
		code, amount string
	}{
		{"USD", "12.34"},
		{"USD", "-0.05"},
		{"USD", "+1.00"},
		{"USD", "1.5"},    // short fraction
		{"USD", "1.234"},  // too many decimals
		{"JPY", "1500"},   // 0-decimal
		{"KWD", "1.234"},  // 3-decimal
		{"usd", "12.34"},  // case-insensitive code
		{"USD", ""},       // empty
		{"USD", "   "},    // whitespace only
		{"USD", "."},      // bare dot
		{"USD", "-"},      // sign only
		{"USD", "0.00"},
		{"USD", "-0.00"},
		{"XXX", "1.00"},   // unknown currency
		{"", "1.00"},      // empty currency
		{"USD", "9223372036854775807"},   // int64 max whole
		{"USD", "-9223372036854775808"},  // int64 min whole
		{"USD", "999999999999999999999"}, // overflow whole
		{"USD", "1e5"},                   // non-digit
		{"USD", "1.2.3"},                 // multiple dots
		{"USD", "12a"},
	}
	for _, c := range corpora {
		f.Add(c.code, c.amount)
	}

	f.Fuzz(func(t *testing.T, code, amount string) {
		// The sole invariant: Parse must not panic for any input.
		_, _ = Parse(code, amount)
	})
}

// FuzzMoneyAddOverflow asserts that Add and Sub never panic for any pair of
// Money values. Overflow and currency mismatch are reachable and must surface
// as errors (ErrOverflow / ErrCurrencyMismatch) rather than panics.
//
// To cover the broadest behavior the harness fuzzes:
//   - the int64 minor-unit amounts of both operands (extreme values hit overflow)
//   - which registered currency each operand uses (same vs. mismatched)
//
// Because Money.amount is private, amounts are injected via FromMinor, which is
// the documented public constructor.
func FuzzMoneyAddOverflow(f *testing.F) {
	codes := Currencies()
	if len(codes) < 2 {
		f.Fatal("test requires at least 2 registered currencies")
	}

	corpora := []struct {
		a, b       int64
		codeA      string
		codeBIdx   int
	}{
		{100, 250, "USD", 0},                       // simple same-currency add
		{1 << 62, 1 << 62, "USD", 0},               // add overflow
		{1 << 62, -1, "USD", 0},                    // sub overflow (big - (-1))
		{0, 0, "USD", 0},
		{-1 << 63, -1, "USD", 0},                    // min amount
		{1<<63 - 1, 1, "USD", 0},                    // max amount
		{100, 100, "USD", 1 % len(codes)},           // currency-mismatch path
	}
	for _, c := range corpora {
		f.Add(c.a, c.b, c.codeA, c.codeBIdx)
	}

	f.Fuzz(func(t *testing.T, a, b int64, codeA string, codeBIdx int) {
		if len(codes) == 0 {
			t.Skip("no registered currencies")
		}
		ma, err := FromMinor(a, codeA)
		if err != nil {
			// Unknown currency is an expected fuzz outcome; nothing to exercise.
			return
		}
		mb, err := FromMinor(b, codes[((codeBIdx%len(codes))+len(codes))%len(codes)])
		if err != nil {
			return
		}

		// Invariant: Add/Sub must not panic. Overflow/mismatch become errors.
		if _, err := ma.Add(mb); err != nil && !isExpectedErr(err) {
			t.Fatalf("Add returned unexpected error: %v", err)
		}
		if _, err := ma.Sub(mb); err != nil && !isExpectedErr(err) {
			t.Fatalf("Sub returned unexpected error: %v", err)
		}
	})
}

// isExpectedErr reports whether err wraps one of the documented, non-fatal
// errors that Add/Sub may return (overflow or currency mismatch). Currency
// mismatch is wrapped with fmt.Errorf("%w: ..."), so errors.Is is required
// rather than identity. Anything else is a bug worth failing the iteration on.
func isExpectedErr(err error) bool {
	return errors.Is(err, ErrOverflow) || errors.Is(err, ErrCurrencyMismatch)
}
