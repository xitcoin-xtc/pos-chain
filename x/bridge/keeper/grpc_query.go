package keeper

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/xitcoin-xtc/pos-chain/x/bridge/types"

	sdkmath "cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// queryServer implements the bridge Query service over the keeper state. It
// is a separate type because the Query service and the keeper both define a
// DailyUsage method.
type queryServer struct {
	keeper Keeper
}

var _ types.QueryServer = queryServer{}

// NewQueryServer returns the bridge Query service.
func NewQueryServer(k Keeper) types.QueryServer {
	return queryServer{keeper: k}
}

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

func (q queryServer) AttestationStatus(goCtx context.Context, req *types.QueryAttestationStatusRequest) (*types.QueryAttestationStatusResponse, error) {
	return q.keeper.AttestationStatus(goCtx, req)
}

var errEmptyRequest = status.Error(codes.InvalidArgument, "empty bridge query request")

func (q queryServer) Params(goCtx context.Context, req *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	if req == nil {
		return nil, errEmptyRequest
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	params := &types.Params{
		Authority:           q.keeper.authority,
		NativeDenom:         q.keeper.nativeDenom,
		RequiredApprovals:   types.RequiredApprovals,
		SignerSetSize:       types.MaxBridgeSigners,
		ChainId:             ctx.ChainID(),
		SourceDepositDomain: types.SourceDepositDomain,
	}
	if !q.keeper.maxSupply.IsNil() {
		params.MaxSupply = q.keeper.maxSupply.String()
	}
	// A chain ID the domains reject leaves them empty: no approval can be
	// verified on such a chain either.
	params.AttestationDomain, _ = types.AttestationDomain(ctx.ChainID())
	params.ControlDomain, _ = types.ControlDomain(ctx.ChainID())
	params.GuardianPauseDomain, _ = types.GuardianPauseDomain(ctx.ChainID())
	return &types.QueryParamsResponse{Params: params}, nil
}

// configuredRoute returns the stored route, or NotFound when none is
// configured.
func (q queryServer) configuredRoute(ctx sdk.Context) (types.RouteConfig, error) {
	config, found, err := q.keeper.GetRouteConfig(ctx)
	if err != nil {
		return types.RouteConfig{}, status.Error(codes.Internal, err.Error())
	}
	if !found {
		return types.RouteConfig{}, status.Error(codes.NotFound, "no bridge route is configured")
	}
	return config, nil
}

func (q queryServer) Route(goCtx context.Context, req *types.QueryRouteRequest) (*types.QueryRouteResponse, error) {
	if req == nil {
		return nil, errEmptyRequest
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	config, err := q.configuredRoute(ctx)
	if err != nil {
		return nil, err
	}
	if req.RouteId != config.RouteID {
		return nil, status.Errorf(codes.NotFound, "bridge route %q is not configured", req.RouteId)
	}
	state, err := q.keeper.GetRouteState(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryRouteResponse{
		Route: &types.Route{
			RouteId:              config.RouteID,
			SourceChainId:        config.EffectiveSourceChainID(ctx.ChainID()),
			BridgeSigners:        config.BridgeSigners,
			Guardian:             config.Guardian,
			MaxTransferAmount:    config.MaxTransferAmount,
			DailyLimit:           config.DailyLimit,
			MaxOutstandingAmount: config.MaxOutstandingAmount,
			Enabled:              config.Enabled,
			Paused:               state.Paused,
			SignerSetVersion:     q.keeper.SignerSetVersion(ctx),
			ControlNonce:         q.keeper.ControlNonce(ctx),
			OutboundNonce:        q.keeper.OutboundNonce(ctx),
		},
		Height: ctx.BlockHeight(),
	}, nil
}

// remaining returns limit - used, or zero when used exceeds the limit.
func remaining(limit, used sdkmath.Int) sdkmath.Int {
	if used.GTE(limit) {
		return sdkmath.ZeroInt()
	}
	return limit.Sub(used)
}

func (q queryServer) Outstanding(goCtx context.Context, req *types.QueryOutstandingRequest) (*types.QueryOutstandingResponse, error) {
	if req == nil {
		return nil, errEmptyRequest
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	config, err := q.configuredRoute(ctx)
	if err != nil {
		return nil, err
	}
	limits, err := config.Limits()
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	outstanding := q.keeper.OutstandingAmount(ctx)
	return &types.QueryOutstandingResponse{
		RouteId:              config.RouteID,
		OutstandingAmount:    outstanding.String(),
		MaxOutstandingAmount: config.MaxOutstandingAmount,
		RemainingAmount:      remaining(limits.MaxOutstandingAmount, outstanding).String(),
		Height:               ctx.BlockHeight(),
	}, nil
}

func (q queryServer) DailyUsage(goCtx context.Context, req *types.QueryDailyUsageRequest) (*types.QueryDailyUsageResponse, error) {
	if req == nil {
		return nil, errEmptyRequest
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	config, err := q.configuredRoute(ctx)
	if err != nil {
		return nil, err
	}
	limits, err := config.Limits()
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	blockTime := ctx.BlockTime().UTC()
	if blockTime.Unix() <= 0 {
		return nil, status.Error(codes.FailedPrecondition, "the queried block has no block time")
	}
	used := sdkmath.NewIntFromBigInt(q.keeper.DailyUsage(ctx, config.RouteID))
	return &types.QueryDailyUsageResponse{
		RouteId:         config.RouteID,
		UtcDay:          UTCDay(blockTime.Unix()),
		UtcDate:         blockTime.Format("2006-01-02"),
		UsedAmount:      used.String(),
		DailyLimit:      config.DailyLimit,
		RemainingAmount: remaining(limits.DailyLimit, used).String(),
		BlockTimeUnix:   blockTime.Unix(),
		Height:          ctx.BlockHeight(),
	}, nil
}

func (q queryServer) Attestation(goCtx context.Context, req *types.QueryAttestationRequest) (*types.QueryAttestationResponse, error) {
	if req == nil {
		return nil, errEmptyRequest
	}
	id, _, err := types.ParseAttestationID(req.AttestationId)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	return q.attestation(sdk.UnwrapSDKContext(goCtx), id)
}

func (q queryServer) SourceDeposit(goCtx context.Context, req *types.QuerySourceDepositRequest) (*types.QueryAttestationResponse, error) {
	if req == nil {
		return nil, errEmptyRequest
	}
	id, err := types.SourceDepositKey(req.SourceChainId, req.SourceRef)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	return q.attestation(sdk.UnwrapSDKContext(goCtx), id)
}

// attestation distinguishes an absent replay key, a processed one with its
// record and a processed one without a record (consumed before records were
// kept). A processed key is never reported as absent.
func (q queryServer) attestation(ctx sdk.Context, id [32]byte) (*types.QueryAttestationResponse, error) {
	response := &types.QueryAttestationResponse{
		AttestationId: types.HexID(id),
		Status:        types.AttestationState_ATTESTATION_STATE_ABSENT,
		Height:        ctx.BlockHeight(),
	}
	if !q.keeper.IsProcessed(ctx, id) {
		return response, nil
	}
	response.Processed = true
	record, found, err := q.keeper.GetAttestationRecord(ctx, id)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if !found {
		response.Status = types.AttestationState_ATTESTATION_STATE_PROCESSED_WITHOUT_RECORD
		return response, nil
	}
	response.Status = types.AttestationState_ATTESTATION_STATE_PROCESSED
	response.Record = &record
	return response, nil
}

func (q queryServer) OutboundTransfer(goCtx context.Context, req *types.QueryOutboundTransferRequest) (*types.QueryOutboundTransferResponse, error) {
	if req == nil {
		return nil, errEmptyRequest
	}
	id, canonical, err := types.ParseRequestID(req.RequestId)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	record, found, err := q.keeper.GetOutboundTransfer(ctx, id)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if !found {
		return nil, status.Errorf(codes.NotFound, "no outbound transfer is recorded for request ID %s", canonical)
	}
	return &types.QueryOutboundTransferResponse{Transfer: &record, Height: ctx.BlockHeight()}, nil
}
