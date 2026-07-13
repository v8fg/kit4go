package tuple_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/v8fg/kit4go/tuple"
)

func TestNewPair(t *testing.T) {
	p := tuple.NewPair("hello", 42)
	require.Equal(t, "hello", p.First)
	require.Equal(t, 42, p.Second)
}

func TestPairValues(t *testing.T) {
	p := tuple.NewPair(1, "two")
	a, b := p.Values()
	require.Equal(t, 1, a)
	require.Equal(t, "two", b)
}

func TestNewTriple(t *testing.T) {
	tr := tuple.NewTriple("a", 2, true)
	require.Equal(t, "a", tr.First)
	require.Equal(t, 2, tr.Second)
	require.True(t, tr.Third)
}

func TestTripleValues(t *testing.T) {
	tr := tuple.NewTriple(10, 3.14, "pi")
	a, b, c := tr.Values()
	require.Equal(t, 10, a)
	require.Equal(t, 3.14, b)
	require.Equal(t, "pi", c)
}

func TestPairAsMapKey(t *testing.T) {
	m := map[tuple.Pair[string, int]]string{}
	m[tuple.NewPair("US", 80)] = "north"
	m[tuple.NewPair("BR", 40)] = "south"
	require.Equal(t, "north", m[tuple.NewPair("US", 80)])
	require.Equal(t, "south", m[tuple.NewPair("BR", 40)])
	require.NotEqual(t, tuple.NewPair("US", 80), tuple.NewPair("US", 81))
}

func TestZeroValue(t *testing.T) {
	var p tuple.Pair[int, string]
	require.Equal(t, 0, p.First)
	require.Equal(t, "", p.Second)
}
