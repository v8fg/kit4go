package log4go

import (
	"fmt"
	"strings"
)

// Filter is a record predicate. WebhookWriter uses it (see
// WebhookWriterOptions.Filter) to forward only records the predicate accepts;
// it is a plain func so ad-hoc closures work too. The constructors below cover
// the common field/keyword matches and can be composed with AllOf/AnyOf/NotMatch.
type Filter func(*Record) bool

// MatchField returns a Filter that matches records whose structured field key
// equals want. Comparison is exact first, then by string form (fmt.Sprint), so a
// numeric field 42 matches both want=42 and want="42".
func MatchField(key string, want any) Filter {
	return func(r *Record) bool {
		v, ok := r.FieldValue(key)
		return ok && fieldEqual(v, want)
	}
}

// MatchFieldIn matches when field key equals any of want (an OR over values).
func MatchFieldIn(key string, want ...any) Filter {
	return func(r *Record) bool {
		v, ok := r.FieldValue(key)
		if !ok {
			return false
		}
		for _, w := range want {
			if fieldEqual(v, w) {
				return true
			}
		}
		return false
	}
}

// MatchKeyword matches when substr appears in the message (case-insensitive).
func MatchKeyword(substr string) Filter {
	sub := strings.ToLower(substr)
	return func(r *Record) bool {
		return sub != "" && strings.Contains(strings.ToLower(r.msg), sub)
	}
}

// MatchKeywordIn matches when any of substrs appears in the message
// (case-insensitive).
func MatchKeywordIn(substrs ...string) Filter {
	subs := make([]string, 0, len(substrs))
	for _, s := range substrs {
		if s = strings.ToLower(s); s != "" {
			subs = append(subs, s)
		}
	}
	return func(r *Record) bool {
		m := strings.ToLower(r.msg)
		for _, sub := range subs {
			if strings.Contains(m, sub) {
				return true
			}
		}
		return false
	}
}

// AllOf matches when every filter matches (logical AND). Nil filters are
// skipped, so AllOf() with no args matches everything.
func AllOf(filters ...Filter) Filter {
	return func(r *Record) bool {
		for _, f := range filters {
			if f != nil && !f(r) {
				return false
			}
		}
		return true
	}
}

// AnyOf matches when at least one filter matches (logical OR). AnyOf() with no
// args matches nothing.
func AnyOf(filters ...Filter) Filter {
	return func(r *Record) bool {
		for _, f := range filters {
			if f != nil && f(r) {
				return true
			}
		}
		return false
	}
}

// NotMatch negates a filter.
func NotMatch(f Filter) Filter {
	return func(r *Record) bool {
		return f == nil || !f(r)
	}
}

// fieldEqual compares two field values: exact interface equality first, then by
// string form so callers can pass either the native value or its string form.
func fieldEqual(got, want any) bool {
	if got == want {
		return true
	}
	return fmt.Sprint(got) == fmt.Sprint(want)
}
