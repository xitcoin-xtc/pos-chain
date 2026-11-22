package validatoradmission

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/xitcoin-xtc/pos-chain/x/validatoradmission/keeper"
)

// stakingMsgServer applies the admission policy before delegating to the
// staking message server. It is used for message servers that are called
// directly instead of through a transaction, such as the staking precompile.
type stakingMsgServer struct {
	stakingtypes.MsgServer
	keeper keeper.Keeper
}

// NewStakingMsgServer wraps a staking message server with the admission policy.
func NewStakingMsgServer(admissionKeeper keeper.Keeper, inner stakingtypes.MsgServer) stakingtypes.MsgServer {
	return stakingMsgServer{MsgServer: inner, keeper: admissionKeeper}
}

func (s stakingMsgServer) CreateValidator(
	goCtx context.Context,
	msg *stakingtypes.MsgCreateValidator,
) (*stakingtypes.MsgCreateValidatorResponse, error) {
	if err := ValidateCreateValidator(sdk.UnwrapSDKContext(goCtx), s.keeper, msg); err != nil {
		return nil, err
	}
	return s.MsgServer.CreateValidator(goCtx, msg)
}

// slashingMsgServer applies the admission policy before delegating to the
// slashing message server, for direct callers such as the slashing precompile.
type slashingMsgServer struct {
	slashingtypes.MsgServer
	keeper keeper.Keeper
}

// NewSlashingMsgServer wraps a slashing message server with the admission policy.
func NewSlashingMsgServer(admissionKeeper keeper.Keeper, inner slashingtypes.MsgServer) slashingtypes.MsgServer {
	return slashingMsgServer{MsgServer: inner, keeper: admissionKeeper}
}

func (s slashingMsgServer) Unjail(
	goCtx context.Context,
	msg *slashingtypes.MsgUnjail,
) (*slashingtypes.MsgUnjailResponse, error) {
	if err := ValidateUnjail(sdk.UnwrapSDKContext(goCtx), s.keeper, msg.ValidatorAddr); err != nil {
		return nil, err
	}
	return s.MsgServer.Unjail(goCtx, msg)
}
