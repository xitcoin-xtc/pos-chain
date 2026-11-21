package keeper

import (
	"encoding/binary"
	"encoding/json"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/xitcoin-xtc/pos-chain/x/bridge/types"
)

func (k Keeper) InitGenesis(ctx sdk.Context, state types.GenesisState) {
	if state.RouteConfig != nil {
		if err := k.SetRouteConfig(ctx, *state.RouteConfig); err != nil {
			panic(err)
		}
		if state.Paused {
			encoded, err := json.Marshal(RouteState{Paused: true})
			if err != nil {
				panic(err)
			}
			ctx.KVStore(k.storeKey).Set(routeStateKey, encoded)
		}
		if state.OutstandingAmount != "" {
			amount, ok := sdkmath.NewIntFromString(state.OutstandingAmount)
			if !ok {
				panic("invalid bridge outstanding amount")
			}
			k.setOutstandingAmount(ctx, amount)
		}
		if state.OutboundNonce != 0 {
			encoded := make([]byte, 8)
			binary.BigEndian.PutUint64(encoded, state.OutboundNonce)
			ctx.KVStore(k.storeKey).Set(types.KeyOutboundNonce, encoded)
		}
	}
}

func (k Keeper) ExportGenesis(ctx sdk.Context) types.GenesisState {
	config, found, err := k.GetRouteConfig(ctx)
	if err != nil {
		panic(err)
	}
	if !found {
		return types.DefaultGenesisState()
	}
	state, err := k.GetRouteState(ctx)
	if err != nil {
		panic(err)
	}
	genesis := types.GenesisState{
		RouteConfig:       &config,
		Paused:            state.Paused,
		OutstandingAmount: k.OutstandingAmount(ctx).String(),
	}
	value := ctx.KVStore(k.storeKey).Get(types.KeyOutboundNonce)
	if len(value) == 8 {
		genesis.OutboundNonce = binary.BigEndian.Uint64(value)
	}
	return genesis
}
