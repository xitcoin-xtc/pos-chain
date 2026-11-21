package keeper

import (
	"errors"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/xitcoin-xtc/pos-chain/x/bridge/types"
)

var ErrAttestationRouteMismatch = errors.New("bridge attestation route does not match configuration")

// AdmitAttestation validates one testnet attestation as a single operation.
// It records replay protection and test-only accounting state, but has no
// bank, minting, reserve, custody, or settlement capability.
func (k Keeper) AdmitAttestation(
	ctx sdk.Context,
	config types.RouteConfig,
	attestation types.Attestation,
	signatures [][]byte,
) ([32]byte, error) {
	if err := config.Validate(); err != nil {
		return [32]byte{}, err
	}
	if attestation.RouteID != config.RouteID {
		return [32]byte{}, ErrAttestationRouteMismatch
	}
	if err := k.RequireRouteAvailable(ctx, config); err != nil {
		return [32]byte{}, err
	}
	if err := attestation.ValidateAt(ctx.BlockTime().Unix()); err != nil {
		return [32]byte{}, err
	}
	if _, err := types.VerifyApprovals(attestation, config.BridgeSigners, signatures); err != nil {
		return [32]byte{}, err
	}
	id, err := attestation.ID()
	if err != nil {
		return [32]byte{}, err
	}
	if k.IsProcessed(ctx, id) {
		return [32]byte{}, ErrAttestationAlreadyProcessed
	}
	if err := k.CheckAndRecordLimits(ctx, config, attestation.Amount); err != nil {
		return [32]byte{}, err
	}
	k.MarkProcessed(ctx, id)
	return id, nil
}
