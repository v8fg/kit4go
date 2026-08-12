// Package str provides string utilities: zero-copy StringToBytes /
// BytesToString, case and content checks (IsEmpty, IsBlank,
// ContainsAny/All, CheckString), and case conversions (CamelToSnake,
// SnakeToCamel, Lower/Upper/Title, Quote). Pure standard library.
package str

import "unsafe"

// StringToBytes converts a string to a byte slice without copying. The result
// aliases the string's immutable backing array — DO NOT mutate the returned
// slice; doing so is undefined behavior.
func StringToBytes(s string) []byte {
	return unsafe.Slice(unsafe.StringData(s), len(s))
}

// BytesToString converts a byte slice to a string without copying. The input
// slice must not be mutated after this call (the string shares its backing
// array).
func BytesToString(b []byte) string {
	return unsafe.String(unsafe.SliceData(b), len(b))
}
