package keeper

import (
	"errors"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/xitcoin-xtc/pos-chain/x/validatorincentives/types"
)

// SnapshotPeriod derives a new daily state exclusively from canonical keeper
// state. It records accounting state and never mints tokens.
func (k Keeper) SnapshotPeriod(
	ctx sdk.Context,
	stakingKeeper types.StakingKeeper,
	treasury Treasury,
) (types.PeriodState, error) {
	if ctx.BlockHeight() < 0 {
		return types.PeriodState{}, errors.New("block height cannot be negative")
	}

	eligibleBonded, err := stakingKeeper.TotalValidatorPower(ctx)
	if err != nil {
		return types.PeriodState{}, err
	}
	if eligibleBonded.IsNegative() {
		return types.PeriodState{}, errors.New("eligible bonded stake cannot be negative")
	}

	balance, err := treasury.Balance(ctx)
	if err != nil {
		return types.PeriodState{}, err
	}
	if balance.Denom != IncentiveDenom {
		return types.PeriodState{}, errors.New("unexpected incentive treasury denomination")
	}

	state, err := types.NewPeriodState(
		uint64(ctx.BlockHeight()),
		eligibleBonded,
		balance.Amount,
		k.GetParams(ctx),
	)
	if err != nil {
		return types.PeriodState{}, err
	}
	if err := k.SetPeriodState(ctx, state); err != nil {
		return types.PeriodState{}, err
	}
	return state, nil
}

// ProcessBlock creates the daily snapshot when required and releases the
// current block's deterministic share to the canonical fee collector.
func (k Keeper) ProcessBlock(
	ctx sdk.Context,
	stakingKeeper types.StakingKeeper,
	treasury Treasury,
	feeCollector string,
) error {
	if ctx.BlockHeight() < 0 {
		return errors.New("block height cannot be negative")
	}
	height := uint64(ctx.BlockHeight())

	state, found, err := k.GetPeriodState(ctx)
	if err != nil {
		return err
	}
	if !found || height >= state.EndBlock {
		state, err = k.SnapshotPeriod(ctx, stakingKeeper, treasury)
		if err != nil {
			return err
		}
	}
	if height < state.StartBlock || height >= state.EndBlock {
		return errors.New("block is outside the active calculation period")
	}

	periodProvision, err := types.ParseStoredAtomicAmount(
		state.PeriodProvisionAtomic,
	)
	if err != nil {
		return err
	}
	distributed, err := types.ParseStoredAtomicAmount(state.DistributedAtomic)
	if err != nil {
		return err
	}
	elapsed := height - state.StartBlock + 1
	target, err := types.CumulativeProvision(
		periodProvision,
		elapsed,
		state.EndBlock-state.StartBlock,
	)
	if err != nil {
		return err
	}
	amount := target.Sub(distributed)
	if amount.IsNegative() {
		return errors.New("stored distribution exceeds the cumulative target")
	}
	if amount.IsZero() {
		return nil
	}

	if err := treasury.SendToModule(ctx, feeCollector, amount); err != nil {
		return err
	}
	return k.RecordDistribution(ctx, amount)
}

// RecordDistribution advances daily and lifetime accounting after a successful
// module-to-module transfer.
func (k Keeper) RecordDistribution(ctx sdk.Context, amount sdkmath.Int) error {
	if !amount.IsPositive() {
		return errors.New("distribution amount must be positive")
	}

	state, found, err := k.GetPeriodState(ctx)
	if err != nil {
		return err
	}
	if !found {
		return errors.New("no calculation period has been configured")
	}
	remaining, err := state.RemainingProvision()
	if err != nil {
		return err
	}
	if amount.GT(remaining) {
		return errors.New("distribution exceeds the remaining period provision")
	}

	distributed, _ := types.ParseStoredAtomicAmount(state.DistributedAtomic)
	total, err := k.GetTotalDistributed(ctx)
	if err != nil {
		return err
	}
	state.DistributedAtomic = distributed.Add(amount).String()
	if err := k.SetPeriodState(ctx, state); err != nil {
		return err
	}
	return k.SetTotalDistributed(ctx, total.Add(amount))
}
