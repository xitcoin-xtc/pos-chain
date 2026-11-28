package types

import (
	"errors"
	"math/big"

	sdkmath "cosmossdk.io/math"
)

const (
	BasisPointDenominator uint32 = 10_000
	DefaultTreasuryReleaseRateBasisPoints uint32 = 1_000
	DefaultBlocksPerYear uint64 = 6_311_520
	DefaultCalculationPeriodBlocks uint64 = 17_280
)

func AnnualizedRewardCapacity(
	treasuryBalance sdkmath.Int,
	releaseRateBasisPoints uint32,
) (sdkmath.Int, error) {
	if treasuryBalance.IsNegative() {
		return sdkmath.Int{}, errors.New("treasury balance cannot be negative")
	}
	if releaseRateBasisPoints > BasisPointDenominator {
		return sdkmath.Int{}, errors.New("treasury release rate exceeds 100 percent")
	}

	return mulDivFloor(
		treasuryBalance,
		new(big.Int).SetUint64(uint64(releaseRateBasisPoints)),
		new(big.Int).SetUint64(uint64(BasisPointDenominator)),
	)
}

func PeriodProvision(
	annualizedCapacity sdkmath.Int,
	periodBlocks uint64,
	blocksPerYear uint64,
) (sdkmath.Int, error) {
	if annualizedCapacity.IsNegative() {
		return sdkmath.Int{}, errors.New("annualized capacity cannot be negative")
	}
	if periodBlocks == 0 {
		return sdkmath.Int{}, errors.New("calculation period must contain at least one block")
	}
	if blocksPerYear == 0 {
		return sdkmath.Int{}, errors.New("blocks per year must be greater than zero")
	}
	if periodBlocks > blocksPerYear {
		return sdkmath.Int{}, errors.New("calculation period cannot exceed one year")
	}

	return mulDivFloor(
		annualizedCapacity,
		new(big.Int).SetUint64(periodBlocks),
		new(big.Int).SetUint64(blocksPerYear),
	)
}

func DerivedAPYBasisPoints(
	annualizedCapacity sdkmath.Int,
	eligibleBonded sdkmath.Int,
) (sdkmath.Int, error) {
	if annualizedCapacity.IsNegative() {
		return sdkmath.Int{}, errors.New("annualized capacity cannot be negative")
	}
	if eligibleBonded.IsNegative() {
		return sdkmath.Int{}, errors.New("eligible bonded stake cannot be negative")
	}
	if eligibleBonded.IsZero() {
		return sdkmath.ZeroInt(), nil
	}

	return mulDivFloor(
		annualizedCapacity,
		new(big.Int).SetUint64(uint64(BasisPointDenominator)),
		eligibleBonded.BigInt(),
	)
}

func CumulativeProvision(
	periodProvision sdkmath.Int,
	elapsedBlocks uint64,
	periodBlocks uint64,
) (sdkmath.Int, error) {
	if periodProvision.IsNegative() {
		return sdkmath.Int{}, errors.New("period provision cannot be negative")
	}
	if periodBlocks == 0 {
		return sdkmath.Int{}, errors.New("calculation period must contain at least one block")
	}
	if elapsedBlocks > periodBlocks {
		return sdkmath.Int{}, errors.New("elapsed blocks exceed the calculation period")
	}

	return mulDivFloor(
		periodProvision,
		new(big.Int).SetUint64(elapsedBlocks),
		new(big.Int).SetUint64(periodBlocks),
	)
}

// mulDivFloor returns floor(value * numerator / denominator) for a
// non-negative value and positive denominator. The product is computed
// exactly with big.Int, so no intermediate value can overflow: the result is
// the same as sdkmath.Int's Mul followed by Quo whenever those do not panic.
// Only a result that does not fit in an sdkmath.Int (256 bits) is an error.
// AnnualizedRewardCapacity, PeriodProvision and CumulativeProvision multiply
// by a ratio of at most one, so their result always fits.
func mulDivFloor(value sdkmath.Int, numerator, denominator *big.Int) (sdkmath.Int, error) {
	product := new(big.Int).Mul(value.BigInt(), numerator)
	quotient := product.Quo(product, denominator)
	if quotient.BitLen() > sdkmath.MaxBitLen {
		return sdkmath.Int{}, errors.New("result exceeds the 256-bit amount range")
	}
	return sdkmath.NewIntFromBigInt(quotient), nil
}
