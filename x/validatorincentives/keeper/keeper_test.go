package keeper

import (
	"bytes"
	"context"
	"errors"
	"testing"

	sdkmath "cosmossdk.io/math"
	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	sdktestutil "github.com/cosmos/cosmos-sdk/testutil"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/xitcoin-xtc/pos-chain/x/validatorincentives/types"
)

func keeperTestContext(t *testing.T) (sdk.Context, Keeper) {
	t.Helper()
	key := storetypes.NewKVStoreKey(types.StoreKey)
	ctx := sdktestutil.DefaultContext(
		key,
		storetypes.NewTransientStoreKey("validator_incentives_test"),
	)
	return ctx, NewKeeper(key)
}

type testAccountKeeper struct{ address sdk.AccAddress }

func (k testAccountKeeper) GetModuleAddress(string) sdk.AccAddress { return k.address }

type testBankKeeper struct {
	balance sdk.Coin
	sent sdk.Coins
	sendErr error
}

func (k *testBankKeeper) GetBalance(context.Context, sdk.AccAddress, string) sdk.Coin {
	return k.balance
}

func (k *testBankKeeper) SendCoinsFromModuleToAccount(
	context.Context, string, sdk.AccAddress, sdk.Coins,
) error { return errors.New("account transfer is not used") }

func (k *testBankKeeper) SendCoinsFromModuleToModule(
	_ context.Context, _ string, _ string, amount sdk.Coins,
) error {
	if k.sendErr != nil { return k.sendErr }
	k.sent = k.sent.Add(amount...)
	k.balance = sdk.NewCoin(IncentiveDenom, k.balance.Amount.Sub(amount.AmountOf(IncentiveDenom)))
	return nil
}

type testStakingKeeper struct {
	bonded sdkmath.Int
	err error
}

func (k testStakingKeeper) TotalValidatorPower(context.Context) (sdkmath.Int, error) {
	return k.bonded, k.err
}

func testTreasury(amount int64) (Treasury, *testBankKeeper) {
	bank := &testBankKeeper{balance: sdk.NewInt64Coin(IncentiveDenom, amount)}
	account := testAccountKeeper{address: sdk.AccAddress(bytes.Repeat([]byte{1}, 20))}
	return NewTreasury(account, bank), bank
}

func TestProcessBlockUsesDailySnapshotAndCumulativeRelease(t *testing.T) {
	ctx, k := keeperTestContext(t)
	params := types.Params{
		TreasuryReleaseRateBasisPoints: 1_000,
		BlocksPerYear: 10,
		CalculationPeriodBlocks: 2,
	}
	require.NoError(t, k.SetParams(ctx, params))
	treasury, bank := testTreasury(1_000)
	staking := testStakingKeeper{bonded: sdkmath.NewInt(1_000)}

	ctx = ctx.WithBlockHeight(10)
	require.NoError(t, k.ProcessBlock(ctx, staking, treasury, "fee_collector"))
	require.Equal(t, sdkmath.NewInt(10), bank.sent.AmountOf(IncentiveDenom))

	ctx = ctx.WithBlockHeight(11)
	require.NoError(t, k.ProcessBlock(ctx, staking, treasury, "fee_collector"))
	require.Equal(t, sdkmath.NewInt(20), bank.sent.AmountOf(IncentiveDenom))

	state, found, err := k.GetPeriodState(ctx)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "20", state.DistributedAtomic)
	require.Equal(t, "1000", state.DerivedAPYBasisPoints)

	ctx = ctx.WithBlockHeight(12)
	require.NoError(t, k.ProcessBlock(ctx, staking, treasury, "fee_collector"))
	state, _, err = k.GetPeriodState(ctx)
	require.NoError(t, err)
	require.Equal(t, "980", state.TreasuryBalanceAtomic)
	require.Equal(t, "98", state.AnnualizedCapacityAtomic)
}

func TestProcessBlockDefersNewFundingUntilNextSnapshot(t *testing.T) {
	ctx, k := keeperTestContext(t)
	require.NoError(t, k.SetParams(ctx, types.Params{1_000, 10, 2}))
	treasury, bank := testTreasury(1_000)
	staking := testStakingKeeper{bonded: sdkmath.NewInt(1_000)}

	ctx = ctx.WithBlockHeight(20)
	require.NoError(t, k.ProcessBlock(ctx, staking, treasury, "fee_collector"))
	bank.balance = sdk.NewInt64Coin(IncentiveDenom, 1_990)
	ctx = ctx.WithBlockHeight(21)
	require.NoError(t, k.ProcessBlock(ctx, staking, treasury, "fee_collector"))
	state, _, err := k.GetPeriodState(ctx)
	require.NoError(t, err)
	require.Equal(t, "1000", state.TreasuryBalanceAtomic)

	ctx = ctx.WithBlockHeight(22)
	require.NoError(t, k.ProcessBlock(ctx, staking, treasury, "fee_collector"))
	state, _, err = k.GetPeriodState(ctx)
	require.NoError(t, err)
	require.Equal(t, "1980", state.TreasuryBalanceAtomic)
}

func TestZeroEligibleStakeProducesNoDistribution(t *testing.T) {
	ctx, k := keeperTestContext(t)
	treasury, bank := testTreasury(1_000)
	ctx = ctx.WithBlockHeight(1)
	require.NoError(t, k.ProcessBlock(
		ctx,
		testStakingKeeper{bonded: sdkmath.ZeroInt()},
		treasury,
		"fee_collector",
	))
	require.True(t, bank.sent.Empty())
}

func TestFailedTransferDoesNotAdvanceAccounting(t *testing.T) {
	ctx, k := keeperTestContext(t)
	require.NoError(t, k.SetParams(ctx, types.Params{1_000, 10, 2}))
	treasury, bank := testTreasury(1_000)
	bank.sendErr = errors.New("rejected")
	ctx = ctx.WithBlockHeight(1)
	require.ErrorContains(t, k.ProcessBlock(
		ctx,
		testStakingKeeper{bonded: sdkmath.NewInt(1_000)},
		treasury,
		"fee_collector",
	), "rejected")
	state, _, err := k.GetPeriodState(ctx)
	require.NoError(t, err)
	require.Equal(t, "0", state.DistributedAtomic)
}

func TestMigrationResetsObsoleteState(t *testing.T) {
	ctx, k := keeperTestContext(t)
	ctx.KVStore(k.storeKey).Set(types.PeriodStateKey, []byte("obsolete"))
	require.NoError(t, k.Migrate1to2(ctx))
	require.Nil(t, ctx.KVStore(k.storeKey).Get(types.PeriodStateKey))
	require.Equal(t, types.DefaultParams(), k.GetParams(ctx))
}
