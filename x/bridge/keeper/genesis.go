package keeper

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/xitcoin-xtc/pos-chain/x/bridge/types"
)

func (k Keeper) InitGenesis(ctx sdk.Context, state types.GenesisState) {
	if state.RouteConfig != nil {
		config := *state.RouteConfig
		if config.SourceChainID == "" {
			config.SourceChainID = types.DefaultSourceChainID(ctx.ChainID())
		}
		if err := k.SetRouteConfig(ctx, config); err != nil {
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
			amount, err := types.ParseNonNegativeAmount(state.OutstandingAmount)
			if err != nil {
				panic("invalid bridge outstanding amount")
			}
			k.setOutstandingAmount(ctx, amount)
		}
		if state.OutboundNonce != 0 {
			encoded := make([]byte, 8)
			binary.BigEndian.PutUint64(encoded, state.OutboundNonce)
			ctx.KVStore(k.storeKey).Set(types.KeyOutboundNonce, encoded)
		}
		if state.ControlNonce != 0 {
			k.setControlNonce(ctx, state.ControlNonce)
		}
		for _, value := range state.ProcessedSourceDeposits {
			id, _, err := types.ParseAttestationID(value)
			if err != nil {
				panic(err)
			}
			k.MarkProcessed(ctx, id)
		}
		version := state.SignerSetVersion
		if version == 0 {
			version = 1
		}
		k.setSignerSetVersion(ctx, version)
		if state.DailyUsage != nil {
			used, err := types.ParseAmount(state.DailyUsage.Amount)
			if err != nil {
				panic("invalid bridge daily usage amount")
			}
			k.setDailyUsage(ctx, config.RouteID, state.DailyUsage.UTCDay, used.BigInt())
		}
		for _, record := range state.AttestationRecords {
			id, err := record.ID()
			if err != nil {
				panic(err)
			}
			if err := k.setAttestationRecord(ctx, id, record); err != nil {
				panic(err)
			}
		}
		for _, record := range state.OutboundTransfers {
			id, err := record.ID()
			if err != nil {
				panic(err)
			}
			if err := k.setOutboundTransfer(ctx, id, record); err != nil {
				panic(err)
			}
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
	genesis.ControlNonce = k.ControlNonce(ctx)
	for _, id := range k.ProcessedSourceDeposits(ctx) {
		genesis.ProcessedSourceDeposits = append(genesis.ProcessedSourceDeposits, "0x"+hex.EncodeToString(id[:]))
	}
	genesis.SignerSetVersion = k.SignerSetVersion(ctx)
	// The export context carries no block time, so the most recent bucket is
	// exported: imported on the same UTC day it keeps counting against the
	// daily limit; on a later day its key is never read.
	if day, used, found := k.LatestDailyUsage(ctx, config.RouteID); found {
		genesis.DailyUsage = &types.DailyUsage{UTCDay: day, Amount: used.String()}
	}
	if genesis.AttestationRecords, err = k.AttestationRecords(ctx); err != nil {
		panic(err)
	}
	if genesis.OutboundTransfers, err = k.OutboundTransfers(ctx); err != nil {
		panic(err)
	}
	return genesis
}
