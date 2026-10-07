package keeper

import (
	"errors"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"

	"github.com/xitcoin-xtc/pos-chain/x/bridge/types"
)

var (
	ErrControlActionNotActive = errors.New("bridge control action is not active")
	ErrControlActionProcessed = errors.New("bridge control action already processed")
	ErrControlPayloadMismatch = errors.New("bridge control payload does not match route configuration")
)

var processedControlPrefix = []byte{0x04}

// ApplyRouteConfigUpdate accepts a configuration only after two distinct
// current bridge signers have approved its exact payload. It has no bank,
// minting, reserve, relayer, or transfer authority.
func (k Keeper) ApplyRouteConfigUpdate(
	ctx sdk.Context,
	current types.RouteConfig,
	action types.ControlAction,
	next types.RouteConfig,
	signatures [][]byte,
) error {
	if err := current.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if action.Action != types.ActionUpdateRouteConfig || action.RouteID != current.RouteID || next.RouteID != current.RouteID {
		return ErrControlPayloadMismatch
	}
	if err := action.Validate(); err != nil {
		return err
	}
	now := ctx.BlockTime().Unix()
	if now < action.NotBeforeUnix || now >= action.ExpiresUnix {
		return ErrControlActionNotActive
	}
	payloadHash, err := types.RouteConfigPayloadHash(next)
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
	key := processedControlKey(id)
	if store.Has(key) {
		return ErrControlActionProcessed
	}
	if _, err := types.VerifyControlApprovals(action, current.BridgeSigners, signatures); err != nil {
		return err
	}
	if err := k.SetRouteConfig(ctx, next); err != nil {
		return err
	}
	store.Set(key, []byte{1})
	return nil
}

func processedControlKey(id [32]byte) []byte {
	key := make([]byte, len(processedControlPrefix)+len(id))
	copy(key, processedControlPrefix)
	copy(key[len(processedControlPrefix):], id[:])
	return key
}
