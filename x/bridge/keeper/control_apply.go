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
	ErrCapBelowOutstanding    = errors.New("bridge max outstanding amount cannot be set below the current outstanding amount")
)

// ApplyRouteConfigUpdate accepts a configuration only after two distinct
// current bridge signers have approved its exact payload with the next
// sequential control nonce. The route ID and source chain ID are immutable.
// The new max outstanding amount must cover the current outstanding amount:
// outstanding <= cap is a state invariant (genesis validation requires it),
// and a cap equal to the outstanding amount already stops every new mint.
// It has no bank, minting, reserve, relayer, or transfer authority.
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
	if action.Action != types.ActionUpdateRouteConfig || action.RouteID != current.RouteID ||
		next.RouteID != current.RouteID || next.SourceChainID != current.SourceChainID {
		return ErrControlPayloadMismatch
	}
	nextLimits, err := next.Limits()
	if err != nil {
		return err
	}
	if k.OutstandingAmount(ctx).GT(nextLimits.MaxOutstandingAmount) {
		return ErrCapBelowOutstanding
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
	if err := k.requireNextControlNonce(ctx, action.Nonce); err != nil {
		return err
	}
	if _, err := types.VerifyControlApprovals(action, ctx.ChainID(), current.BridgeSigners, signatures); err != nil {
		return err
	}
	if err := k.SetRouteConfig(ctx, next); err != nil {
		return err
	}
	k.setControlNonce(ctx, action.Nonce)
	return nil
}
