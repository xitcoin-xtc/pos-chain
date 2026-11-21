package keeper

import (
	"encoding/binary"
	"errors"

	"github.com/xitcoin-xtc/pos-chain/x/bridge/types"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// ErrControlNonceGap rejects a control action whose nonce skips ahead of the
// next sequential control nonce.
var ErrControlNonceGap = errors.New("bridge control nonce is not the next sequential nonce")

// ControlNonce returns the nonce of the last applied 2-of-3 control action
// (update_route_config or resume_route); zero before the first one.
func (k Keeper) ControlNonce(ctx sdk.Context) uint64 {
	value := ctx.KVStore(k.storeKey).Get(types.KeyControlNonce)
	if len(value) != 8 {
		return 0
	}
	return binary.BigEndian.Uint64(value)
}

func (k Keeper) setControlNonce(ctx sdk.Context, nonce uint64) {
	encoded := make([]byte, 8)
	binary.BigEndian.PutUint64(encoded, nonce)
	ctx.KVStore(k.storeKey).Set(types.KeyControlNonce, encoded)
}

// requireNextControlNonce accepts only ControlNonce()+1. A consumed or older
// nonce is reported as already processed; a later nonce as a gap. Control
// actions therefore apply strictly in signing order, and an older signed
// action that is still inside its validity window can never be applied after
// a newer one.
func (k Keeper) requireNextControlNonce(ctx sdk.Context, nonce uint64) error {
	last := k.ControlNonce(ctx)
	if nonce <= last {
		return ErrControlActionProcessed
	}
	if nonce != last+1 {
		return ErrControlNonceGap
	}
	return nil
}
