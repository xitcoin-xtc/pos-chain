package keeper

import (
	"errors"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/xitcoin-xtc/pos-chain/x/bridge/types"
)

var (
	ErrAttestationRouteMismatch       = errors.New("bridge attestation route does not match configuration")
	ErrAttestationSourceChainMismatch = errors.New("bridge attestation source chain does not match the route configuration")
)

// AdmitAttestation validates one attestation as a single operation: route,
// availability, deadline, source chain, 2-of-3 signatures over the
// network-specific digest, source-deposit replay protection and limits. It has
// no bank, minting, reserve or custody capability (see SettleInbound).
//
// With the replay key it stores the AttestationRecord returned by the
// Attestation query: every approved field, the recovered approvers, the
// signer set version, and the block height and hash of the transaction
// (ctx.TxBytes) that processes it. All of these are identical on every
// validator, and the writes roll back with the transaction.
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
	if attestation.SourceChainID != config.EffectiveSourceChainID(ctx.ChainID()) {
		return [32]byte{}, ErrAttestationSourceChainMismatch
	}
	approvers, err := types.VerifyApprovals(attestation, ctx.ChainID(), config.BridgeSigners, signatures)
	if err != nil {
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
	record, err := types.NewAttestationRecord(attestation, ctx.ChainID(), k.SignerSetVersion(ctx), approvers, ctx.BlockHeight(), types.TxHash(ctx.TxBytes()))
	if err != nil {
		return [32]byte{}, err
	}
	if err := k.setAttestationRecord(ctx, id, record); err != nil {
		return [32]byte{}, err
	}
	k.MarkProcessed(ctx, id)
	return id, nil
}
