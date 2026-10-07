package keeper

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"

	"github.com/xitcoin-xtc/pos-chain/x/bridge/types"
)

type msgServer struct {
	keeper Keeper
}

// NewMsgServer creates the bridge settlement message server.
func NewMsgServer(k Keeper) types.MsgServer {
	return msgServer{keeper: k}
}

// InitializeRouteConfig creates exactly one disabled, paused bridge route.
// It performs no token, bank, reserve, relayer, mint, burn, or transfer action.
func (s msgServer) InitializeRouteConfig(goCtx context.Context, req *types.MsgInitializeRouteConfig) (*types.MsgInitializeRouteConfigResponse, error) {
	if req == nil {
		return nil, errors.New("bridge: empty initial route configuration")
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	if err := sdk.ValidateAuthority(ctx, s.keeper.authority, req.Authority); err != nil {
		return nil, err
	}
	if err := req.ValidateBasic(); err != nil {
		return nil, err
	}
	if _, found, err := s.keeper.GetRouteConfig(ctx); err != nil {
		return nil, err
	} else if found {
		return nil, errors.New("bridge: initial route configuration already exists")
	}
	config := req.RouteConfig()
	if err := s.keeper.SetRouteConfig(ctx, config); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(RouteState{Paused: true})
	if err != nil {
		return nil, err
	}
	ctx.KVStore(s.keeper.storeKey).Set(routeStateKey, encoded)
	ctx.EventManager().EmitEvent(sdk.NewEvent("bridge_route_initialized", sdk.NewAttribute("route_id", config.RouteID), sdk.NewAttribute("enabled", "false"), sdk.NewAttribute("paused", "true")))
	return &types.MsgInitializeRouteConfigResponse{}, nil
}

// EmergencyPauseRoute applies a guardian-signed pause and changes no other
// route configuration or asset state.
func (s msgServer) EmergencyPauseRoute(goCtx context.Context, req *types.MsgEmergencyPauseRoute) (*types.MsgEmergencyPauseRouteResponse, error) {
	if req == nil {
		return nil, errors.New("bridge: empty emergency pause")
	}
	if err := req.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	config, found, err := s.keeper.GetRouteConfig(ctx)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrRouteDisabled
	}
	if err := s.keeper.PauseRoute(ctx, config, req.Action(), req.GuardianSignature); err != nil {
		return nil, err
	}
	ctx.EventManager().EmitEvent(sdk.NewEvent("bridge_route_paused", sdk.NewAttribute("route_id", config.RouteID), sdk.NewAttribute("nonce", fmt.Sprintf("%d", req.Nonce))))
	return &types.MsgEmergencyPauseRouteResponse{}, nil
}

// ResumeRoute applies a time-bounded 2-of-3 approval and clears only Paused.
func (s msgServer) ResumeRoute(goCtx context.Context, req *types.MsgResumeRoute) (*types.MsgResumeRouteResponse, error) {
	if req == nil {
		return nil, errors.New("bridge: empty route resume")
	}
	if err := req.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	config, found, err := s.keeper.GetRouteConfig(ctx)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrRouteDisabled
	}
	payloadHash, err := RouteStatePayloadHash(config, RouteState{Paused: false})
	if err != nil {
		return nil, err
	}
	action := req.Action(common.BytesToHash(payloadHash[:]).Hex())
	if err := s.keeper.ResumeRoute(ctx, config, action, req.Signatures); err != nil {
		return nil, err
	}
	ctx.EventManager().EmitEvent(sdk.NewEvent("bridge_route_resumed", sdk.NewAttribute("route_id", config.RouteID), sdk.NewAttribute("nonce", fmt.Sprintf("%d", req.Nonce))))
	return &types.MsgResumeRouteResponse{}, nil
}

// UpdateRouteConfig applies an exact configuration approved by two current
// signers. Administration never mints, burns, reserves, or transfers assets.
func (s msgServer) UpdateRouteConfig(goCtx context.Context, req *types.MsgUpdateRouteConfig) (*types.MsgUpdateRouteConfigResponse, error) {
	if req == nil {
		return nil, errors.New("bridge: empty route configuration update")
	}
	if err := req.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	current, found, err := s.keeper.GetRouteConfig(ctx)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrRouteDisabled
	}
	next := req.RouteConfig()
	payloadHash, err := types.RouteConfigPayloadHash(next)
	if err != nil {
		return nil, err
	}
	action := req.Action(common.BytesToHash(payloadHash[:]).Hex())
	if err := s.keeper.ApplyRouteConfigUpdate(ctx, current, action, next, req.Signatures); err != nil {
		return nil, err
	}
	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"bridge_route_config_updated",
		sdk.NewAttribute("route_id", next.RouteID),
		sdk.NewAttribute("enabled", fmt.Sprintf("%t", next.Enabled)),
		sdk.NewAttribute("nonce", fmt.Sprintf("%d", req.Nonce)),
	))
	return &types.MsgUpdateRouteConfigResponse{}, nil
}

// SubmitAttestation validates and settles one inbound Cronos transfer.
func (s msgServer) SubmitAttestation(goCtx context.Context, req *types.MsgSubmitAttestation) (*types.MsgSubmitAttestationResponse, error) {
	if req == nil {
		return nil, errors.New("bridge: empty attestation submission")
	}
	if err := req.ValidateBasic(); err != nil {
		return nil, err
	}

	ctx := sdk.UnwrapSDKContext(goCtx)
	config, found, err := s.keeper.GetRouteConfig(ctx)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrRouteDisabled
	}

	attestation := req.Attestation()
	if attestation.Direction != types.DirectionCronosToXitcoin {
		return nil, ErrSettlementDirection
	}
	id, err := s.keeper.AdmitAttestation(ctx, config, attestation, req.Signatures)
	if err != nil {
		return nil, err
	}
	if err := s.keeper.SettleInbound(ctx, config, attestation); err != nil {
		return nil, err
	}
	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"bridge_inbound_settled",
		sdk.NewAttribute("attestation_id", hex.EncodeToString(id[:])),
		sdk.NewAttribute("route_id", config.RouteID),
		sdk.NewAttribute("destination", attestation.Destination),
		sdk.NewAttribute("amount", attestation.Amount),
	))
	return &types.MsgSubmitAttestationResponse{AttestationId: hex.EncodeToString(id[:])}, nil
}

func (s msgServer) InitiateOutboundTransfer(goCtx context.Context, req *types.MsgInitiateOutboundTransfer) (*types.MsgInitiateOutboundTransferResponse, error) {
	if req == nil {
		return nil, errors.New("bridge: empty outbound transfer")
	}
	if err := req.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	config, found, err := s.keeper.GetRouteConfig(ctx)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrRouteDisabled
	}
	if req.RouteId != config.RouteID {
		return nil, ErrAttestationRouteMismatch
	}
	sender, err := sdk.AccAddressFromBech32(req.Sender)
	if err != nil {
		return nil, err
	}
	id, nonce, err := s.keeper.InitiateOutboundTransfer(ctx, config, sender, req.Destination, req.Amount)
	if err != nil {
		return nil, err
	}
	requestID := hex.EncodeToString(id[:])
	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"bridge_outbound_burned",
		sdk.NewAttribute("request_id", requestID),
		sdk.NewAttribute("route_id", config.RouteID),
		sdk.NewAttribute("sender", sender.String()),
		sdk.NewAttribute("destination", req.Destination),
		sdk.NewAttribute("amount", req.Amount),
		sdk.NewAttribute("nonce", fmt.Sprintf("%d", nonce)),
	))
	return &types.MsgInitiateOutboundTransferResponse{RequestId: requestID, Nonce: nonce}, nil
}
