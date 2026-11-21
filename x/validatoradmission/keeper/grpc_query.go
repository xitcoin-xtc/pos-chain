package keeper

import (
	"context"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/xitcoin-xtc/pos-chain/x/validatoradmission/types"
)

var _ types.QueryServer = Keeper{}

func (k Keeper) Params(goCtx context.Context, _ *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	return &types.QueryParamsResponse{
		Authority:             k.GetAuthority(ctx),
		MaxApprovedValidators: k.GetMaxApprovedValidators(ctx),
		MinimumSelfDelegation: k.GetMinimumSelfDelegation(ctx),
	}, nil
}

func (k Keeper) Validator(goCtx context.Context, req *types.QueryValidatorRequest) (*types.QueryValidatorResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("validator admission query request is required")
	}
	if _, err := sdk.ValAddressFromBech32(req.ValidatorAddress); err != nil {
		return nil, fmt.Errorf("invalid validator address: %w", err)
	}

	ctx := sdk.UnwrapSDKContext(goCtx)
	return &types.QueryValidatorResponse{
		Approved: k.IsApprovedValidator(ctx, req.ValidatorAddress),
	}, nil
}
