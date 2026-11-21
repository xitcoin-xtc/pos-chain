package keeper

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"

	"github.com/xitcoin-xtc/pos-chain/x/bridge/types"
)

var ErrRouteNotPaused = errors.New("bridge route is not paused")

// RouteStatePayloadHash binds a governance signature to its exact intended
// state and current configuration.
func RouteStatePayloadHash(config types.RouteConfig, state RouteState) ([32]byte, error) {
	if err := config.Validate(); err != nil {
		return [32]byte{}, err
	}
	payload := struct {
		Config types.RouteConfig `json:"config"`
		State  RouteState        `json:"state"`
	}{Config: config, State: state}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(encoded), nil
}

// ResumeRoute requires two current bridge signers. The guardian has no path to
// this method. This method changes only Paused from true to false.
func (k Keeper) ResumeRoute(ctx sdk.Context, config types.RouteConfig, action types.ControlAction, signatures [][]byte) error {
	if err := config.Validate(); err != nil {
		return err
	}
	if action.Action != types.ActionResumeRoute || action.RouteID != config.RouteID {
		return ErrControlPayloadMismatch
	}
	if err := action.Validate(); err != nil {
		return err
	}
	now := ctx.BlockTime().Unix()
	if now < action.NotBeforeUnix || now >= action.ExpiresUnix {
		return ErrControlActionNotActive
	}
	state, err := k.GetRouteState(ctx)
	if err != nil {
		return err
	}
	if !state.Paused {
		return ErrRouteNotPaused
	}
	next := RouteState{Paused: false}
	payloadHash, err := RouteStatePayloadHash(config, next)
	if err != nil {
		return err
	}
	if !strings.EqualFold(common.BytesToHash(payloadHash[:]).Hex(), action.PayloadHash) {
		return ErrControlPayloadMismatch
	}
	id, err := action.ID()
	if err != nil {
		return err
	}
	store := ctx.KVStore(k.storeKey)
	if store.Has(processedControlKey(id)) {
		return ErrControlActionProcessed
	}
	if _, err := types.VerifyControlApprovals(action, config.BridgeSigners, signatures); err != nil {
		return err
	}
	encoded, err := json.Marshal(next)
	if err != nil {
		return err
	}
	store.Set(routeStateKey, encoded)
	store.Set(processedControlKey(id), []byte{1})
	return nil
}
