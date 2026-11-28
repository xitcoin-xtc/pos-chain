package types

import (
	"math"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	sdkmath "cosmossdk.io/math"
)

// The policy math used to multiply in 256-bit sdkmath.Int before dividing,
// so amounts above about 2^192 (2^242 for the release rate) panicked with
// "integer overflow" although the result fits. Found by FuzzPolicyMath.
func TestPolicyMathHasNoOverflowPanic(t *testing.T) {
	maximum := sdkmath.NewIntFromBigInt(new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1)))
	third := new(big.Int).Quo(maximum.BigInt(), big.NewInt(3))

	var (
		got sdkmath.Int
		err error
	)
	require.NotPanics(t, func() { got, err = AnnualizedRewardCapacity(maximum, BasisPointDenominator) })
	require.NoError(t, err)
	require.True(t, got.Equal(maximum))

	require.NotPanics(t, func() { got, err = PeriodProvision(maximum, 1, 3) })
	require.NoError(t, err)
	require.Equal(t, third.String(), got.String())

	require.NotPanics(t, func() { got, err = CumulativeProvision(maximum, math.MaxUint64-1, math.MaxUint64) })
	require.NoError(t, err)
	require.True(t, got.LT(maximum))

	// The APY of the largest capacity against one atomic unit of stake does
	// not fit in 256 bits: an error, not a panic.
	require.NotPanics(t, func() { _, err = DerivedAPYBasisPoints(maximum, sdkmath.OneInt()) })
	require.Error(t, err)

	var state PeriodState
	require.NotPanics(t, func() {
		state, err = NewPeriodState(1, maximum, maximum, Params{BasisPointDenominator, math.MaxUint64, math.MaxUint64 - 1})
	})
	require.NoError(t, err)
	require.NoError(t, state.Validate())

	// Below the overflow threshold the results are unchanged.
	got, err = AnnualizedRewardCapacity(sdkmath.NewInt(20_000_000), DefaultTreasuryReleaseRateBasisPoints)
	require.NoError(t, err)
	require.Equal(t, "2000000", got.String())
}
