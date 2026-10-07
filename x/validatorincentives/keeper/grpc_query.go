package keeper

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/xitcoin-xtc/pos-chain/x/validatorincentives/types"
)

var _ types.QueryServer = queryServer{}

type queryServer struct {
	keeper   Keeper
	treasury Treasury
}

func NewQueryServer(
	keeper Keeper,
	treasury Treasury,
) types.QueryServer {
	return queryServer{
		keeper:   keeper,
		treasury: treasury,
	}
}

func (s queryServer) Params(
	goCtx context.Context,
	_ *types.QueryParamsRequest,
) (*types.QueryParamsResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	params := s.keeper.GetParams(ctx)

	return &types.QueryParamsResponse{
		Authority:                      s.keeper.GetAuthority(ctx),
		TreasuryReleaseRateBasisPoints: params.TreasuryReleaseRateBasisPoints,
		BlocksPerYear:                  params.BlocksPerYear,
		CalculationPeriodBlocks:        params.CalculationPeriodBlocks,
	}, nil
}

func (s queryServer) Period(
	goCtx context.Context,
	_ *types.QueryPeriodRequest,
) (*types.QueryPeriodResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	totalDistributed, err := s.keeper.GetTotalDistributed(ctx)
	if err != nil {
		return nil, err
	}

	response := &types.QueryPeriodResponse{
		TotalDistributedAtomic: totalDistributed.String(),
	}

	state, found, err := s.keeper.GetPeriodState(ctx)
	if err != nil {
		return nil, err
	}
	if !found {
		return response, nil
	}

	height := ctx.BlockHeight()
	response.Active = height >= 0 &&
		uint64(height) >= state.StartBlock &&
		uint64(height) < state.EndBlock
	response.StartBlock = state.StartBlock
	response.EndBlock = state.EndBlock
	response.TreasuryReleaseRateBasisPoints =
		state.TreasuryReleaseRateBasisPoints
	response.TreasuryBalanceAtomic =
		state.TreasuryBalanceAtomic
	response.EligibleBondedAtomic =
		state.EligibleBondedAtomic
	response.AnnualizedCapacityAtomic =
		state.AnnualizedCapacityAtomic
	response.DerivedApyBasisPoints =
		state.DerivedAPYBasisPoints
	response.PeriodProvisionAtomic =
		state.PeriodProvisionAtomic
	response.DistributedAtomic =
		state.DistributedAtomic

	return response, nil
}

func (s queryServer) Treasury(
	goCtx context.Context,
	_ *types.QueryTreasuryRequest,
) (*types.QueryTreasuryResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	balance, err := s.treasury.Balance(ctx)
	if err != nil {
		return nil, err
	}

	return &types.QueryTreasuryResponse{
		Denom:         balance.Denom,
		BalanceAtomic: balance.Amount.String(),
	}, nil
}
