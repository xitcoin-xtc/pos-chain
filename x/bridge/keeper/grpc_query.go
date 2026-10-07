package keeper

import (
	"context"
	"errors"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/xitcoin-xtc/pos-chain/x/bridge/types"
)

var _ types.QueryServer = Keeper{}

func (k Keeper) AttestationStatus(goCtx context.Context, req *types.QueryAttestationStatusRequest) (*types.QueryAttestationStatusResponse, error) {
	if req == nil {
		return nil, errors.New("bridge attestation status request is required")
	}
	id, canonical, err := types.ParseAttestationID(req.AttestationId)
	if err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	return &types.QueryAttestationStatusResponse{
		AttestationId: canonical,
		Processed:     k.IsProcessed(ctx, id),
	}, nil
}
