package trie

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInsertAndGet(t *testing.T) {
	tr := New[int]()
	tr.Insert("a/b/c", 42)
	v, ok := tr.Get("a/b/c")
	require.True(t, ok)
	require.Equal(t, 42, v)
}

func TestGetMiss(t *testing.T) {
	tr := New[string]()
	tr.Insert("a/b", "v")
	_, ok := tr.Get("missing")
	require.False(t, ok)
	_, ok = tr.Get("a")
	require.False(t, ok) // "a" is a prefix node but has no value
}

func TestHas(t *testing.T) {
	tr := New[bool]()
	tr.Insert("x/y", true)
	require.True(t, tr.Has("x/y"))
	require.False(t, tr.Has("x"))
}

func TestLongestPrefix(t *testing.T) {
	tr := New[string]()
	tr.Insert("a", "root-a")
	tr.Insert("a/b", "ab")
	tr.Insert("a/b/c", "abc")

	v, key, ok := tr.LongestPrefix("a/b/c/d/e")
	require.True(t, ok)
	require.Equal(t, "abc", v)
	require.Equal(t, "a/b/c", key)

	v, key, ok = tr.LongestPrefix("a/b")
	require.True(t, ok)
	require.Equal(t, "ab", v)
	require.Equal(t, "a/b", key)

	v, key, ok = tr.LongestPrefix("a/x/y")
	require.True(t, ok)
	require.Equal(t, "root-a", v)
	require.Equal(t, "a", key)

	_, _, ok = tr.LongestPrefix("z/none")
	require.False(t, ok)
}

func TestLongestPrefixEmpty(t *testing.T) {
	tr := New[int]()
	_, _, ok := tr.LongestPrefix("anything")
	require.False(t, ok)
}

func TestDelete(t *testing.T) {
	tr := New[int]()
	tr.Insert("a/b/c", 1)
	tr.Insert("a/b", 2)

	require.True(t, tr.Delete("a/b/c"))
	require.False(t, tr.Has("a/b/c"))
	require.True(t, tr.Has("a/b")) // parent survives

	require.True(t, tr.Delete("a/b"))
	require.False(t, tr.Has("a/b"))
	require.Equal(t, 0, tr.Len()) // pruned to empty
}

func TestDeleteNonExistent(t *testing.T) {
	tr := New[int]()
	require.False(t, tr.Delete("nope"))
	tr.Insert("a", 1)
	require.False(t, tr.Delete("a/b"))
}

func TestDeleteRootEmptyKey(t *testing.T) {
	tr := New[string]()
	tr.Insert("", "root")
	require.True(t, tr.Delete(""))
	require.False(t, tr.Has(""))
}

func TestKeysWithPrefix(t *testing.T) {
	tr := New[int]()
	tr.Insert("api/v1/users", 1)
	tr.Insert("api/v1/orders", 2)
	tr.Insert("api/v2/users", 3)
	tr.Insert("web/home", 4)

	keys := tr.KeysWithPrefix("api/v1")
	require.Len(t, keys, 2)
	require.Contains(t, keys, "api/v1/users")
	require.Contains(t, keys, "api/v1/orders")

	keys = tr.KeysWithPrefix("api")
	require.Len(t, keys, 3)

	keys = tr.KeysWithPrefix("none")
	require.Empty(t, keys)
}

func TestLen(t *testing.T) {
	tr := New[int]()
	require.Equal(t, 0, tr.Len())
	tr.Insert("a", 1)
	tr.Insert("b", 2)
	tr.Insert("a/b", 3)
	require.Equal(t, 3, tr.Len())
	tr.Delete("a")
	require.Equal(t, 2, tr.Len())
}

func TestOverwrite(t *testing.T) {
	tr := New[string]()
	tr.Insert("k", "old")
	tr.Insert("k", "new")
	v, _ := tr.Get("k")
	require.Equal(t, "new", v)
	require.Equal(t, 1, tr.Len())
}

func TestConcurrency(t *testing.T) {
	tr := New[int]()
	var wg sync.WaitGroup
	const g = 16
	wg.Add(g * 2)
	for i := 0; i < g; i++ {
		i := i
		go func() {
			defer wg.Done()
			tr.Insert("k"+itoa(i), i)
		}()
		go func() {
			defer wg.Done()
			_, _, _ = tr.LongestPrefix("k" + itoa(i) + "/x")
			_ = tr.Len()
		}()
	}
	wg.Wait()
	require.Equal(t, g, tr.Len())
}

// itoa is a tiny int-to-string to avoid importing strconv in the test loop.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
