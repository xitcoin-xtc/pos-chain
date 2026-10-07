package keeper

import (
	"context"
	"errors"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/xitcoin-xtc/pos-chain/x/validatorincentives/types"
)

var _ types.MsgServer = msgServer{}

type msgServer struct {
	keeper Keeper
}

func NewMsgServerImpl(keeper Keeper) types.MsgServer {
	return msgServer{keeper: keeper}
}

func (s msgServer) UpdateParams(
	goCtx context.Context,
	message *types.MsgUpdateParams,
) (*types.MsgUpdateParamsResponse, error) {
	if message == nil {
		return nil, errors.New("update params message is nil")
	}

	ctx := sdk.UnwrapSDKContext(goCtx)
	command := types.UpdateParamsCommand{
		Authority: message.Authority,
		Params: types.Params{
			TreasuryReleaseRateBasisPoints: message.TreasuryReleaseRateBasisPoints,
			BlocksPerYear:                  message.BlocksPerYear,
			CalculationPeriodBlocks:        message.CalculationPeriodBlocks,
		},
	}
	if err := s.keeper.HandleUpdateParamsCommand(
		ctx,
		command,
	); err != nil {
		return nil, err
	}
	return &types.MsgUpdateParamsResponse{}, nil
}
