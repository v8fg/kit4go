package random

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNumericCode_LengthAndDigits(t *testing.T) {
	for _, n := range []int{1, 4, 6, 8, 16} {
		code := NumericCode(n)
		require.Len(t, code, n, "length %d", n)
		for i := 0; i < len(code); i++ {
			require.GreaterOrEqual(t, code[i], byte('0'))
			require.LessOrEqual(t, code[i], byte('9'))
		}
	}
}

func TestNumericCode_NonPositive(t *testing.T) {
	require.Equal(t, "", NumericCode(0))
	require.Equal(t, "", NumericCode(-3))
}

func TestNumericCode_DistributionAndUniqueness(t *testing.T) {
	const n = 6
	const samples = 5000
	seen := make(map[string]struct{}, samples)
	digitCount := [10]int{}
	for i := 0; i < samples; i++ {
		code := NumericCode(n)
		require.Len(t, code, n)
		seen[code] = struct{}{}
		for j := 0; j < n; j++ {
			digitCount[code[j]-'0']++
		}
	}
	// Across 5000×6 = 30000 digits, each of the 10 digits should appear ~3000
	// times; assert each is within 25% (generous, no false flakes).
	for d, c := range digitCount {
		require.InDelta(t, 3000, c, 750, "digit %d distribution off", d)
	}
	// Near-certain uniqueness for 6-digit codes over 5000 draws.
	require.Greater(t, len(seen), samples*99/100)
}
