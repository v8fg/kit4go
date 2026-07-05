package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestInt64_ParseErrorAndMissing covers the Int64 parse-error branch (returns
// default) and the missing-key branch.
func TestInt64_ParseErrorAndMissing(t *testing.T) {
	store := New(MapSource{
		"good": "9000000000",
		"bad":  "not-an-int64",
	})
	require.Equal(t, int64(9000000000), store.Int64("good", -1))
	// Parse error -> default.
	require.Equal(t, int64(-1), store.Int64("bad", -1))
	// Missing -> default.
	require.Equal(t, int64(42), store.Int64("missing", 42))
}

// TestStringSlice_EmptyAfterTrim exercises the branch where every field trims to
// empty (e.g. only separators/whitespace) -> returns def rather than an empty
// slice.
func TestStringSlice_EmptyAfterTrim(t *testing.T) {
	store := New(MapSource{
		"allsep": ",,,",
		"ws":     " ,  , ",
	})
	// All-separators -> def.
	require.Equal(t, []string{"d"}, store.StringSlice("allsep", ",", []string{"d"}))
	// All-whitespace fields -> def.
	require.Equal(t, []string{"x"}, store.StringSlice("ws", ",", []string{"x"}))
	// Missing -> def.
	require.Nil(t, store.StringSlice("missing", ",", nil))
}

// TestIntSlice_EmptyAndErrors exercises both the "all fields empty/unparseable"
// paths: an all-empty-fields value (yields an empty out slice -> def) and a
// value with a parse error (-> def).
func TestIntSlice_EmptyAndErrors(t *testing.T) {
	store := New(MapSource{
		"emptyfields": ",,,",
		"bad":         "1,abc,3",
		"good":        "1,2,3",
	})
	// All fields empty -> out is empty -> def.
	require.Equal(t, []int{9}, store.IntSlice("emptyfields", ",", []int{9}))
	// Parse failure on any field -> def.
	require.Equal(t, []int{9}, store.IntSlice("bad", ",", []int{9}))
	// Missing -> def.
	require.Equal(t, []int{7}, store.IntSlice("missing", ",", []int{7}))
	// Sanity: the happy path still returns parsed values.
	require.Equal(t, []int{1, 2, 3}, store.IntSlice("good", ",", nil))
}

// TestErrMissing_Error exercises the errMissing.Error() method (0% coverage).
func TestErrMissing_Error(t *testing.T) {
	require.Equal(t, "config: key missing", ErrMissing.Error())
}

// TestInt64_HappyAndTrim is a small positive-path guard so the Int64 happy path
// is explicitly asserted (the existing TestTypedGetters already covers it, but
// this keeps the coverage file self-documenting).
func TestInt64_HappyAndTrim(t *testing.T) {
	store := New(MapSource{"k": "  123456789012  "})
	require.Equal(t, int64(123456789012), store.Int64("k", 0))
}

// TestDuration_ParseError covers the Duration parse-error default branch
// explicitly with an unparseable value.
func TestDuration_ParseError(t *testing.T) {
	store := New(MapSource{"bad": "not-a-duration"})
	require.Equal(t, 3*time.Second, store.Duration("bad", 3*time.Second))
}
