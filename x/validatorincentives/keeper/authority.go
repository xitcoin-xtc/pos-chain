package keeper

import (
	"errors"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/xitcoin-xtc/pos-chain/x/validatorincentives/types"
)

// RequireAuthority rejects unset, empty or non-matching callers. The stored
// authority is expected to be the governance or security authority configured
// by the application.
func (k Keeper) RequireAuthority(
	ctx sdk.Context,
	caller string,
) error {
	authority := k.GetAuthority(ctx)
	if authority == "" {
		return errors.New("validator incentive authority is not configured")
	}
	if caller == "" {
		return errors.New("validator incentive caller is empty")
	}
	if caller != authority {
		return errors.New("validator incentive caller is not authorized")
	}
	return nil
}

func (k Keeper) UpdateParamsAuthorized(
	ctx sdk.Context,
	caller string,
	next types.Params,
) error {
	if err := k.RequireAuthority(ctx, caller); err != nil {
		return err
	}
	return k.UpdateParams(ctx, next)
}
