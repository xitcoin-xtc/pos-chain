package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/xitcoin-xtc/pos-chain/x/bridge/types"
)

// ConsumeAttestationAt validates the signed deadline before reserving the
// replay-protection key. The block time is the only accepted time source.
func (k Keeper) ConsumeAttestationAt(ctx sdk.Context, attestation types.Attestation) ([32]byte, error) {
	if err := attestation.ValidateAt(ctx.BlockTime().Unix()); err != nil {
		return [32]byte{}, err
	}
	return k.ConsumeAttestation(ctx, attestation)
}
