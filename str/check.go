package str

import (
	"strings"
)

// EqualIgnoreCase reports whether s and t, interpreted as UTF-8 strings,
// are equal under simple Unicode case-folding, which is a more general
// form of case-insensitivity.
func EqualIgnoreCase(s, t string) bool {
	return strings.EqualFold(s, t)
}

// IsEmpty reports whether s is empty.
func IsEmpty(s string) bool {
	return len(s) == 0
}

// IsBlank reports whether s is empty or only contains the space character.
func IsBlank(s string) bool {
	return len(s) == 0 || len(strings.TrimSpace(s)) == 0
}

// CharIsNumber reports whether the char is an ASCII number.
func CharIsNumber(c byte) bool {
	return c >= '0' && c <= '9'
}

// CharIsAlphabet reports whether the char is an ASCII letter.
func CharIsAlphabet(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z'
}

// ContainsAny reports whether the string s contains any one sub string.
func ContainsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// ContainsAll reports whether the string s contains all the sub string.
func ContainsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}

// StartWithAny reports whether the string has the prefix string in the set.
func StartWithAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.HasPrefix(s, sub) {
			return true
		}
	}
	return false
}

// EndWithAny reports whether the string has the suffix string in the set.
func EndWithAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.HasSuffix(s, sub) {
			return true
		}
	}
	return false
}
