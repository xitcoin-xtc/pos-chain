package keeper

import (
	"context"
	"errors"

	"github.com/xitcoin-xtc/pos-chain/x/validatoradmission/types"

	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

type stakingKeeper interface {
	GetValidator(context.Context, sdk.ValAddress) (stakingtypes.Validator, error)
	Jail(context.Context, sdk.ConsAddress) error
}

type msgServer struct {
	keeper        Keeper
	stakingKeeper stakingKeeper
}

// NewMsgServer creates the on-chain Xitcoin Validator Admission message server.
func NewMsgServer(k Keeper, sk stakingKeeper) types.MsgServer {
	return msgServer{
		keeper:        k,
		stakingKeeper: sk,
	}
}

func (s msgServer) validateAuthority(ctx sdk.Context, authority string) error {
	return sdk.ValidateAuthority(ctx, s.keeper.GetAuthority(ctx), authority)
}

// ApproveValidator authorizes a validator address before it can create a validator.
func (s msgServer) ApproveValidator(goCtx context.Context, req *types.MsgApproveValidator) (*types.MsgApproveValidatorResponse, error) {
	if req == nil {
		return nil, errors.New("Validator Admission: empty approval request")
	}

	ctx := sdk.UnwrapSDKContext(goCtx)
	if err := s.validateAuthority(ctx, req.Authority); err != nil {
		return nil, err
	}
	if err := req.ValidateBasic(); err != nil {
		return nil, err
	}

	if s.keeper.IsApprovedValidator(ctx, req.ValidatorAddress) {
		return &types.MsgApproveValidatorResponse{}, nil
	}
	if s.keeper.ApprovedValidatorCount(ctx) >= s.keeper.GetMaxApprovedValidators(ctx) {
		return nil, errors.New("Validator Admission: approved validator capacity reached")
	}

	s.keeper.SetApprovedValidator(ctx, req.ValidatorAddress, true)
	return &types.MsgApproveValidatorResponse{}, nil
}

// RevokeValidator removes authorization and jails an existing validator.
func (s msgServer) RevokeValidator(goCtx context.Context, req *types.MsgRevokeValidator) (*types.MsgRevokeValidatorResponse, error) {
	if req == nil {
		return nil, errors.New("Validator Admission: empty revocation request")
	}

	ctx := sdk.UnwrapSDKContext(goCtx)
	if err := s.validateAuthority(ctx, req.Authority); err != nil {
		return nil, err
	}
	if err := req.ValidateBasic(); err != nil {
		return nil, err
	}

	validatorAddress, err := sdk.ValAddressFromBech32(req.ValidatorAddress)
	if err != nil {
		return nil, err
	}

	s.keeper.SetApprovedValidator(ctx, req.ValidatorAddress, false)

	validator, err := s.stakingKeeper.GetValidator(ctx, validatorAddress)
	if errors.Is(err, stakingtypes.ErrNoValidatorFound) {
		return &types.MsgRevokeValidatorResponse{}, nil
	}
	if err != nil {
		return nil, err
	}

	if validator.IsJailed() {
		return &types.MsgRevokeValidatorResponse{}, nil
	}

	consensusAddress, err := validator.GetConsAddr()
	if err != nil {
		return nil, err
	}
	if err := s.stakingKeeper.Jail(ctx, sdk.ConsAddress(consensusAddress)); err != nil {
		return nil, err
	}

	return &types.MsgRevokeValidatorResponse{}, nil
}

// UpdateParams updates policy parameters under the configured Xitcoin authority.
func (s msgServer) UpdateParams(goCtx context.Context, req *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if req == nil {
		return nil, errors.New("Validator Admission: empty policy update request")
	}

	ctx := sdk.UnwrapSDKContext(goCtx)
	if err := s.validateAuthority(ctx, req.Authority); err != nil {
		return nil, err
	}
	if err := req.ValidateBasic(); err != nil {
		return nil, err
	}
	if req.MaxApprovedValidators < s.keeper.ApprovedValidatorCount(ctx) {
		return nil, errors.New("Validator Admission: capacity cannot be below approved validator count")
	}

	s.keeper.SetMaxApprovedValidators(ctx, req.MaxApprovedValidators)
	s.keeper.SetMinimumSelfDelegation(ctx, req.MinimumSelfDelegation)
	return &types.MsgUpdateParamsResponse{}, nil
}
