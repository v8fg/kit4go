package str

import (
	"strconv"
	"strings"
	"unicode"
)

// Quote returns a double-quoted Go string literal representing s. The
// returned string uses Go escape sequences (\t, \n, \xFF, \u0100) for
// control characters and non-printable characters as defined by
// IsPrint.
func Quote(s string) string {
	return strconv.Quote(s)
}

// Unquote interprets s as a single-quoted, double-quoted,
// or backquoted Go string literal, returning the string value
// that s quotes.  (If s is single-quoted, it would be a Go
// character literal; Unquote returns the corresponding
// one-character string.)
func Unquote(s string) (t string) {
	var err error
	if t, err = strconv.Unquote(s); err != nil {
		return s
	}
	return
}

// Title returns a copy of the string s with all Unicode letters mapped to
// their Unicode title case.
func Title(s string) string { return strings.ToTitle(s) }

// Lower returns s with all Unicode letters mapped to their lower case.
func Lower(s string) string { return strings.ToLower(s) }

// Upper returns s with all Unicode letters mapped to their upper case.
func Upper(s string) string { return strings.ToUpper(s) }

const defaultDelimiterSpace = ' '

// Camel converts all the delimiter separated words in a String into camel,
// that is each word is made up of a title case character and then a series of
// lowercase characters.
//
// The delimiters represent a set of characters understood to separate words.
// The first non-delimiter character after a delimiter will be capitalized. The first String
// character may or may not be capitalized, and it's determined by the user input for capitalizeFirstLetter
// variable.
//
// The delimiters`s type shall rune, like: ' ', '_', '@', etc.
func Camel(s string, capitalizeFirstLetter bool, delimiters ...rune) string {
	if IsEmpty(s) {
		return s
	}

	delimiterSet := make(map[rune]struct{}, len(delimiters)+1)
	delimiterSet[defaultDelimiterSpace] = struct{}{} // default delimiter, used to remove space

	for i := 0; i < len(delimiters); i++ {
		delimiterSet[delimiters[i]] = struct{}{}
	}

	sb := strings.Builder{}
	capitalizeNext := capitalizeFirstLetter
	outOffset := 0
	for _, v := range s {
		if _, ok := delimiterSet[v]; ok {
			capitalizeNext = outOffset != 0
			continue
		}

		if capitalizeNext || outOffset == 0 && capitalizeFirstLetter {
			sb.WriteString(strings.ToTitle(string(v)))
			capitalizeNext = false
		} else {
			sb.WriteString(string(v))
		}
		outOffset++
	}
	return sb.String()
}

// SnakeToCamel converts the snake case to camel case.
// Example: "snake_case_to_camel_case" -> "snakeCaseToCamelCase"
func SnakeToCamel(s string, capitalizeFirstLetter bool) string {
	return Camel(s, capitalizeFirstLetter, '_')
}

// CamelToSnake converts a given camel format string to snake case.
func CamelToSnake(s string) string {
	return CamelToSnakeWithDelimiter(s, "_")
}

// CamelToSnakeWithDelimiter converts a given camel format string to snake case with given delimiter.
func CamelToSnakeWithDelimiter(s, delimiter string) string {
	if IsEmpty(s) {
		return s
	}
	if len(strings.TrimSpace(delimiter)) == 0 {
		delimiter = "_"
	}

	var words []string
	rs := []rune(s)
	sizeRs := len(rs)
	lastPos := 0

	for i := 0; i < sizeRs; i++ {
		if i > 0 && unicode.IsUpper(rs[i]) {
			if initialism := initialismExtract(s[lastPos:]); len(initialism) != 0 {
				words = append(words, strings.ToLower(initialism))
				i += len(initialism) - 1
				lastPos = i
				continue
			}

			words = append(words, strings.ToLower(s[lastPos:i]))
			lastPos = i
		}
	}
	if len(s[lastPos:]) != 0 {
		words = append(words, strings.ToLower(s[lastPos:]))
	}
	return strings.Join(words, delimiter)
}

// SnakeToCamelWithInitialismList converts snake to camel with given initializes, if given initializes null,
// use the default initializes.
func SnakeToCamelWithInitialismList(s string, capitalizeFirstLetter bool, initializes ...string) string {
	if len(initializes) == 0 {
		return SnakeToCamelWithInitializes(s, capitalizeFirstLetter, commonInitializes)
	}

	inputInitializes := make(map[string]bool)
	for _, initialism := range initializes {
		inputInitializes[initialism] = true
	}
	return SnakeToCamelWithInitializes(s, capitalizeFirstLetter, inputInitializes)
}

// SnakeToCamelWithDefaultInitializes converts snake to camel with the global initializes, initialism will be treated as one word.
func SnakeToCamelWithDefaultInitializes(s string, capitalizeFirstLetter bool) string {
	return SnakeToCamelWithInitializes(s, capitalizeFirstLetter, commonInitializes)
}

// SnakeToCamelWithInitializes converts snake to camel with the default initializes, initialism will be treated as one word.
func SnakeToCamelWithInitializes(s string, capitalizeFirstLetter bool, initializes map[string]bool) string {
	if IsEmpty(s) {
		return s
	}

	if len(initializes) == 0 {
		initializes = commonInitializes
	}

	sb := strings.Builder{}
	words := strings.Split(s, "_")
	for i, word := range words {
		if upper := strings.ToUpper(word); initializes[upper] {
			sb.WriteString(upper)
			continue
		}

		if (capitalizeFirstLetter || i > 0) && len(word) > 0 {
			w := []rune(word)
			w[0] = unicode.ToUpper(w[0])
			sb.WriteString(string(w))
		} else {
			sb.WriteString(strings.ToLower(word))
		}
	}
	return sb.String()
}

func initialismExtract(s string) (initialism string) {
	for i := 1; i <= maxLengthOfInitializes; i++ {
		if len(s) > i-1 && commonInitializes[s[:i]] {
			initialism = s[:i]
		}
	}
	return initialism
}

// current the maximum length of the commonInitializes elements
const maxLengthOfInitializes = 5

// some initializes, ref https://github.com/golang/lint/blob/master/lint.go
// commonInitializes is a set of common initializes.
// Only add entries that are highly unlikely to be non-initializes.
// For instance, "ID" is fine (Freudian code is rare), but "AND" is not.
var commonInitializes = map[string]bool{
	"ACL":   true,
	"API":   true,
	"ASCII": true,
	"CPU":   true,
	"CSS":   true,
	"DNS":   true,
	"EOF":   true,
	"GUID":  true,
	"HTML":  true,
	"HTTP":  true,
	"HTTPS": true,
	"ID":    true,
	"IP":    true,
	"JSON":  true,
	"LHS":   true,
	"QPS":   true,
	"RAM":   true,
	"RHS":   true,
	"RPC":   true,
	"SLA":   true,
	"SMTP":  true,
	"SQL":   true,
	"SSH":   true,
	"TCP":   true,
	"TLS":   true,
	"TTL":   true,
	"UDP":   true,
	"UI":    true,
	"UID":   true,
	"UUID":  true,
	"URI":   true,
	"URL":   true,
	"UTF8":  true,
	"VM":    true,
	"XML":   true,
	"XMPP":  true,
	"XSRF":  true,
	"XSS":   true,
}
