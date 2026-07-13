package bytespool_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/v8fg/kit4go/bytespool"
)

func TestGetPut(t *testing.T) {
	b := bytespool.Get(256)
	require.NotNil(t, b)
	require.Equal(t, 0, b.Len())
	b.WriteString("hello")
	require.Equal(t, "hello", b.String())
	bytespool.Put(b)
}

func TestPutNil(t *testing.T) {
	require.NotPanics(t, func() { bytespool.Put(nil) })
}

func TestGetReset(t *testing.T) {
	b := bytespool.Get(64)
	b.WriteString("data")
	bytespool.Put(b)
	b2 := bytespool.Get(64)
	require.Equal(t, 0, b2.Len(), "pooled buffer must be reset")
}

func TestWithBuffer(t *testing.T) {
	var result string
	bytespool.WithBuffer(128, func(b *bytes.Buffer) {
		b.WriteString("inside")
		result = b.String()
	})
	require.Equal(t, "inside", result)
}

func TestSizeClasses(t *testing.T) {
	// Small and large requests get different-capacity buffers.
	small := bytespool.Get(32)
	large := bytespool.Get(32768)
	require.GreaterOrEqual(t, small.Cap(), 32)
	require.GreaterOrEqual(t, large.Cap(), 32768)
	bytespool.Put(small)
	bytespool.Put(large)
}

func TestPutOversized(t *testing.T) {
	// Buffers > maxSize*2 are discarded (not retained).
	b := bytespool.Get(64)
	for range 200000 {
		b.WriteByte('x')
	}
	require.NotPanics(t, func() { bytespool.Put(b) }) // discarded, no panic
}

func TestGetLargeRequest(t *testing.T) {
	b := bytespool.Get(100000) // > maxSize (65536)
	require.NotNil(t, b)
	require.Equal(t, 0, b.Len())
	bytespool.Put(b)
}

func TestClassIndexClamp(t *testing.T) {
	// Requests near the boundary should map to the last class.
	b := bytespool.Get(65536) // exactly maxSize
	require.NotNil(t, b)
	bytespool.Put(b)
}
