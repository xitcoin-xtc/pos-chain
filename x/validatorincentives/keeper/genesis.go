package keeper

import (
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/xitcoin-xtc/pos-chain/x/validatorincentives/types"
)

func (k Keeper) InitGenesis(
	ctx sdk.Context,
	state types.GenesisState,
) error {
	if err := state.Validate(); err != nil {
		return err
	}
	if err := k.SetParams(ctx, state.Params); err != nil {
		return err
	}
	if err := k.SetTotalDistributed(
		ctx,
		sdkmath.ZeroInt(),
	); err != nil {
		return err
	}
	if state.Authority != "" {
		k.SetAuthority(ctx, state.Authority)
	}
	return nil
}

func (k Keeper) ExportGenesis(
	ctx sdk.Context,
) types.GenesisState {
	return types.GenesisState{
		Authority: k.GetAuthority(ctx),
		Params:    k.GetParams(ctx),
	}
}
