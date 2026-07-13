package mathx_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/v8fg/kit4go/mathx"
)

func TestSum(t *testing.T) {
	require.Equal(t, 6, mathx.Sum(1, 2, 3))
	require.Equal(t, 0, mathx.Sum[int]()) // empty → zero
	require.Equal(t, 6.5, mathx.Sum(1.5, 2.0, 3.0))
	require.Equal(t, uint(10), mathx.Sum[uint](3, 7))
}

func TestProduct(t *testing.T) {
	require.Equal(t, 24, mathx.Product(1, 2, 3, 4))
	require.Equal(t, 1, mathx.Product[int]()) // empty → identity
	require.Equal(t, 6.0, mathx.Product(1.5, 4.0))
}

func TestClamp(t *testing.T) {
	require.Equal(t, 5, mathx.Clamp(5, 0, 10))
	require.Equal(t, 0, mathx.Clamp(-3, 0, 10))
	require.Equal(t, 10, mathx.Clamp(15, 0, 10))
	require.Equal(t, 5, mathx.Clamp(5, 5, 5)) // single point
	require.Panics(t, func() { mathx.Clamp(1, 10, 0) })
}

func TestMap(t *testing.T) {
	require.Equal(t, 128.0, mathx.Map(0.5, 0.0, 1.0, 0.0, 256.0))
	require.Equal(t, 0.0, mathx.Map(0.0, 0.0, 1.0, 0.0, 256.0))
	require.Equal(t, 256.0, mathx.Map(1.0, 0.0, 1.0, 0.0, 256.0))
	require.Equal(t, 0.0, mathx.Map(5.0, 5.0, 5.0, 0.0, 100.0)) // degenerate
}

func TestLerp(t *testing.T) {
	require.Equal(t, 5.0, mathx.Lerp(0.0, 10.0, 0.5))
	require.Equal(t, 0.0, mathx.Lerp(0.0, 10.0, 0.0))
	require.Equal(t, 10.0, mathx.Lerp(0.0, 10.0, 1.0))
	require.Equal(t, 15.0, mathx.Lerp(0.0, 10.0, 1.5)) // extrapolate
}

func TestSumFloat32(t *testing.T) {
	require.Equal(t, float32(3.5), mathx.Sum(float32(1.5), float32(2.0)))
	_ = math.NaN
}
