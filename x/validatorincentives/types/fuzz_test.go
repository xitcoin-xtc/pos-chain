package types

// Attack simulations of the validator incentive policy math. `go test`
// replays the seed corpus; `go test -fuzz` explores further (see
// .github/workflows/fuzz.yml and docs/validator-incentives-attack-simulations.md).
//
// Every function is checked over its whole input domain (amounts in
// [0, 2^256-1], block counts in [0, 2^64-1]) against a reference written with
// unbounded big.Int arithmetic: no panic, the exact floor result whenever the
// result fits in 256 bits, and an error otherwise.

import (
	"encoding/json"
	"math"
	"math/big"
	"testing"

	sdkmath "cosmossdk.io/math"
)

var (
	fuzzMaxUint256 = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	// fuzzMaximumSupply is evmd/config.MaximumSupplyAtomic, the largest
	// treasury balance a real chain can hold.
	fuzzMaximumSupply, _ = new(big.Int).SetString("5250000000000000000000000000", 10)
)

// fuzzAmount reads an unsigned 256-bit amount from at most 32 bytes.
func fuzzAmount(raw []byte) *big.Int {
	if len(raw) > 32 {
		raw = raw[len(raw)-32:]
	}
	return new(big.Int).SetBytes(raw)
}

// call runs fn and fails the test if it panics.
func call[T any](t *testing.T, name string, fn func() (T, error)) (result T, err error) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("%s panicked: %v", name, recovered)
		}
	}()
	return fn()
}

// referenceMulDivFloor is the reference floor(value * numerator / denominator) and
// whether it fits in an sdkmath.Int.
func referenceMulDivFloor(value, numerator, denominator *big.Int) (*big.Int, bool) {
	product := new(big.Int).Mul(value, numerator)
	quotient := product.Quo(product, denominator)
	return quotient, quotient.BitLen() <= sdkmath.MaxBitLen
}

func requireEqualInt(t *testing.T, name string, got sdkmath.Int, want *big.Int) {
	t.Helper()
	if got.IsNil() || got.BigInt().Cmp(want) != 0 {
		t.Fatalf("%s = %v, want %s", name, got, want)
	}
}

func bigUint(value uint64) *big.Int { return new(big.Int).SetUint64(value) }

var fuzzPolicySeeds = []struct {
	treasury                             []byte
	rate                                 uint32
	periodBlocks, blocksPerYear, elapsed uint64
	bonded                               []byte
	start                                uint64
}{
	// Launch reference values (types/policy_test.go).
	{big.NewInt(20_000_000).Bytes(), DefaultTreasuryReleaseRateBasisPoints, DefaultCalculationPeriodBlocks, DefaultBlocksPerYear, 1, big.NewInt(20_000_000).Bytes(), 1},
	// The whole maximum supply in the treasury, 100% release, one block.
	{fuzzMaximumSupply.Bytes(), BasisPointDenominator, 1, 1, 1, []byte{1}, 7},
	// Daily period, last block, maximum supply against one atomic unit of stake.
	{fuzzMaximumSupply.Bytes(), BasisPointDenominator, DefaultCalculationPeriodBlocks, DefaultBlocksPerYear, DefaultCalculationPeriodBlocks, []byte{1}, 100},
	// Full domain: these overflowed the 256-bit intermediate products.
	{fuzzMaxUint256.Bytes(), BasisPointDenominator, math.MaxUint64, math.MaxUint64, math.MaxUint64 - 1, []byte{1}, 0},
	{fuzzMaxUint256.Bytes(), 1, 2, math.MaxUint64, 2, fuzzMaxUint256.Bytes(), math.MaxUint64 - 2},
	{new(big.Int).Lsh(big.NewInt(1), 243).Bytes(), BasisPointDenominator, 3, 4, 2, []byte{3}, 1},
	{new(big.Int).Lsh(big.NewInt(1), 200).Bytes(), 9_999, 1 << 60, 1 << 62, 1 << 59, []byte{0}, 1 << 40},
	// Invalid parameters must be errors, never panics.
	{[]byte{1}, BasisPointDenominator + 1, 0, 0, 1, nil, math.MaxUint64},
	{[]byte{5}, 0, 10, 9, 11, []byte{}, 5},
	{nil, 0, 1, 1, 0, nil, 0},
}

func FuzzPolicyMath(f *testing.F) {
	for _, seed := range fuzzPolicySeeds {
		f.Add(seed.treasury, seed.rate, seed.periodBlocks, seed.blocksPerYear, seed.elapsed, seed.bonded, seed.start)
	}
	f.Fuzz(func(t *testing.T, treasuryRaw []byte, rate uint32, periodBlocks, blocksPerYear, elapsed uint64, bondedRaw []byte, start uint64) {
		treasury, bonded := fuzzAmount(treasuryRaw), fuzzAmount(bondedRaw)
		checkAnnualized(t, treasury, rate)
		checkPeriodProvision(t, treasury, periodBlocks, blocksPerYear)
		checkDerivedAPY(t, treasury, bonded)
		checkCumulative(t, treasury, elapsed, periodBlocks)
		checkPeriodState(t, start, bonded, treasury, Params{
			TreasuryReleaseRateBasisPoints: rate,
			BlocksPerYear:                  blocksPerYear,
			CalculationPeriodBlocks:        periodBlocks,
		})
		checkNegativeInputsAreErrors(t, treasury, bonded, rate, periodBlocks, blocksPerYear, elapsed)
	})
}

// AnnualizedRewardCapacity is floor(balance * rate / 10000) for a rate of at
// most 100%, so it never exceeds the balance.
func checkAnnualized(t *testing.T, treasury *big.Int, rate uint32) {
	t.Helper()
	got, err := call(t, "AnnualizedRewardCapacity", func() (sdkmath.Int, error) {
		return AnnualizedRewardCapacity(sdkmath.NewIntFromBigInt(treasury), rate)
	})
	if rate > BasisPointDenominator {
		if err == nil {
			t.Fatalf("release rate %d accepted", rate)
		}
		return
	}
	if err != nil {
		t.Fatalf("AnnualizedRewardCapacity(%s, %d): %v", treasury, rate, err)
	}
	want, _ := referenceMulDivFloor(treasury, bigUint(uint64(rate)), bigUint(uint64(BasisPointDenominator)))
	requireEqualInt(t, "annualized capacity", got, want)
	if got.BigInt().Cmp(treasury) > 0 {
		t.Fatalf("annualized capacity %s exceeds the treasury %s", got, treasury)
	}
}

// PeriodProvision is floor(capacity * period / year) and never exceeds the
// annualized capacity.
func checkPeriodProvision(t *testing.T, capacity *big.Int, periodBlocks, blocksPerYear uint64) {
	t.Helper()
	got, err := call(t, "PeriodProvision", func() (sdkmath.Int, error) {
		return PeriodProvision(sdkmath.NewIntFromBigInt(capacity), periodBlocks, blocksPerYear)
	})
	if periodBlocks == 0 || blocksPerYear == 0 || periodBlocks > blocksPerYear {
		if err == nil {
			t.Fatalf("PeriodProvision accepted period %d, year %d", periodBlocks, blocksPerYear)
		}
		return
	}
	if err != nil {
		t.Fatalf("PeriodProvision(%s, %d, %d): %v", capacity, periodBlocks, blocksPerYear, err)
	}
	want, _ := referenceMulDivFloor(capacity, bigUint(periodBlocks), bigUint(blocksPerYear))
	requireEqualInt(t, "period provision", got, want)
	if got.BigInt().Cmp(capacity) > 0 {
		t.Fatalf("period provision %s exceeds the annualized capacity %s", got, capacity)
	}
}

// DerivedAPYBasisPoints is floor(capacity * 10000 / bonded), zero without
// bonded stake, and an error (not a panic) when it cannot be represented.
func checkDerivedAPY(t *testing.T, capacity, bonded *big.Int) {
	t.Helper()
	got, err := call(t, "DerivedAPYBasisPoints", func() (sdkmath.Int, error) {
		return DerivedAPYBasisPoints(sdkmath.NewIntFromBigInt(capacity), sdkmath.NewIntFromBigInt(bonded))
	})
	if bonded.Sign() == 0 {
		if err != nil {
			t.Fatalf("DerivedAPYBasisPoints without bonded stake: %v", err)
		}
		requireEqualInt(t, "derived APY", got, new(big.Int))
		return
	}
	want, fits := referenceMulDivFloor(capacity, bigUint(uint64(BasisPointDenominator)), bonded)
	if !fits {
		if err == nil {
			t.Fatalf("DerivedAPYBasisPoints(%s, %s) = %s, want an error (result does not fit)", capacity, bonded, got)
		}
		return
	}
	if err != nil {
		t.Fatalf("DerivedAPYBasisPoints(%s, %s): %v", capacity, bonded, err)
	}
	requireEqualInt(t, "derived APY", got, want)
}

// CumulativeProvision is monotonic in the elapsed blocks, equals the period
// provision at the end of the period, never exceeds it, and releases at most
// ceil(provision / period) per block.
func checkCumulative(t *testing.T, provision *big.Int, elapsed, periodBlocks uint64) {
	t.Helper()
	cumulative := func(e uint64) (sdkmath.Int, error) {
		return call(t, "CumulativeProvision", func() (sdkmath.Int, error) {
			return CumulativeProvision(sdkmath.NewIntFromBigInt(provision), e, periodBlocks)
		})
	}
	got, err := cumulative(elapsed)
	if periodBlocks == 0 || elapsed > periodBlocks {
		if err == nil {
			t.Fatalf("CumulativeProvision accepted elapsed %d of period %d", elapsed, periodBlocks)
		}
		return
	}
	if err != nil {
		t.Fatalf("CumulativeProvision(%s, %d, %d): %v", provision, elapsed, periodBlocks, err)
	}
	want, _ := referenceMulDivFloor(provision, bigUint(elapsed), bigUint(periodBlocks))
	requireEqualInt(t, "cumulative provision", got, want)
	if got.BigInt().Cmp(provision) > 0 {
		t.Fatalf("cumulative provision %s exceeds the period provision %s", got, provision)
	}

	end, err := cumulative(periodBlocks)
	if err != nil {
		t.Fatalf("CumulativeProvision at the period end: %v", err)
	}
	requireEqualInt(t, "cumulative provision at the period end", end, provision)

	perBlockCap := new(big.Int).Add(provision, bigUint(periodBlocks-1))
	perBlockCap.Quo(perBlockCap, bigUint(periodBlocks))
	if elapsed < periodBlocks {
		next, err := cumulative(elapsed + 1)
		if err != nil {
			t.Fatalf("CumulativeProvision(%d): %v", elapsed+1, err)
		}
		step := new(big.Int).Sub(next.BigInt(), got.BigInt())
		if step.Sign() < 0 {
			t.Fatalf("cumulative provision decreased from %s to %s at block %d", got, next, elapsed+1)
		}
		if step.Cmp(perBlockCap) > 0 {
			t.Fatalf("block %d released %s, more than ceil(%s / %d)", elapsed+1, step, provision, periodBlocks)
		}
	}
	if elapsed > 0 {
		previous, err := cumulative(elapsed - 1)
		if err != nil {
			t.Fatalf("CumulativeProvision(%d): %v", elapsed-1, err)
		}
		if previous.GT(got) {
			t.Fatalf("cumulative provision decreased from %s to %s at block %d", previous, got, elapsed)
		}
	}
}

// NewPeriodState accepts every valid parameter set without panicking and
// produces a state that validates, whose provision is backed by the treasury
// snapshot, and which survives the JSON encoding of the store.
func checkPeriodState(t *testing.T, start uint64, bonded, treasury *big.Int, params Params) {
	t.Helper()
	state, err := call(t, "NewPeriodState", func() (PeriodState, error) {
		return NewPeriodState(start, sdkmath.NewIntFromBigInt(bonded), sdkmath.NewIntFromBigInt(treasury), params)
	})
	if params.Validate() != nil || start > math.MaxUint64-params.CalculationPeriodBlocks {
		if err == nil {
			t.Fatalf("NewPeriodState accepted params %+v at start %d", params, start)
		}
		return
	}
	annualized, _ := referenceMulDivFloor(treasury, bigUint(uint64(params.TreasuryReleaseRateBasisPoints)), bigUint(uint64(BasisPointDenominator)))
	if bonded.Sign() > 0 {
		if _, fits := referenceMulDivFloor(annualized, bigUint(uint64(BasisPointDenominator)), bonded); !fits {
			if err == nil {
				t.Fatalf("NewPeriodState stored an APY that does not fit in 256 bits")
			}
			return
		}
	}
	if err != nil {
		t.Fatalf("NewPeriodState(%d, %s, %s, %+v): %v", start, bonded, treasury, params, err)
	}
	if err := state.Validate(); err != nil {
		t.Fatalf("new period state does not validate: %v", err)
	}
	if state.StartBlock != start || state.EndBlock != start+params.CalculationPeriodBlocks {
		t.Fatalf("period [%d, %d), want [%d, %d)", state.StartBlock, state.EndBlock, start, start+params.CalculationPeriodBlocks)
	}
	provision, _ := new(big.Int).SetString(state.PeriodProvisionAtomic, 10)
	wantProvision := new(big.Int)
	if bonded.Sign() > 0 {
		wantProvision, _ = referenceMulDivFloor(annualized, bigUint(params.CalculationPeriodBlocks), bigUint(params.BlocksPerYear))
	}
	if provision == nil || provision.Cmp(wantProvision) != 0 {
		t.Fatalf("period provision %s, want %s", state.PeriodProvisionAtomic, wantProvision)
	}
	if provision.Cmp(treasury) > 0 {
		t.Fatalf("period provision %s exceeds the treasury snapshot %s", provision, treasury)
	}
	if state.DistributedAtomic != "0" || state.TreasuryBalanceAtomic != treasury.String() {
		t.Fatalf("unexpected new period state %+v", state)
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	var decoded PeriodState
	if err := json.Unmarshal(encoded, &decoded); err != nil || decoded != state {
		t.Fatalf("period state does not round-trip: %v", err)
	}
	if remaining, err := decoded.RemainingProvision(); err != nil || remaining.BigInt().Cmp(provision) != 0 {
		t.Fatalf("remaining provision %v, err %v, want %s", remaining, err, provision)
	}
}

// Negative amounts are rejected with an error, never a panic.
func checkNegativeInputsAreErrors(t *testing.T, treasury, bonded *big.Int, rate uint32, periodBlocks, blocksPerYear, elapsed uint64) {
	t.Helper()
	if treasury.Sign() == 0 {
		return
	}
	negative := sdkmath.NewIntFromBigInt(new(big.Int).Neg(treasury))
	positive := sdkmath.OneInt()
	if bonded.Sign() > 0 {
		positive = sdkmath.NewIntFromBigInt(bonded)
	}
	for name, fn := range map[string]func() (sdkmath.Int, error){
		"AnnualizedRewardCapacity": func() (sdkmath.Int, error) { return AnnualizedRewardCapacity(negative, rate) },
		"PeriodProvision":          func() (sdkmath.Int, error) { return PeriodProvision(negative, periodBlocks, blocksPerYear) },
		"DerivedAPYBasisPoints capacity": func() (sdkmath.Int, error) {
			return DerivedAPYBasisPoints(negative, positive)
		},
		"DerivedAPYBasisPoints stake": func() (sdkmath.Int, error) {
			return DerivedAPYBasisPoints(positive, negative)
		},
		"CumulativeProvision": func() (sdkmath.Int, error) { return CumulativeProvision(negative, elapsed, periodBlocks) },
	} {
		if _, err := call(t, name, fn); err == nil {
			t.Fatalf("%s accepted the negative amount %s", name, negative)
		}
	}
	_, err := call(t, "NewPeriodState", func() (PeriodState, error) {
		return NewPeriodState(1, negative, positive, DefaultParams())
	})
	if err == nil {
		t.Fatal("NewPeriodState accepted negative bonded stake")
	}
	_, err = call(t, "NewPeriodState", func() (PeriodState, error) {
		return NewPeriodState(1, positive, negative, DefaultParams())
	})
	if err == nil {
		t.Fatal("NewPeriodState accepted a negative treasury balance")
	}
}
