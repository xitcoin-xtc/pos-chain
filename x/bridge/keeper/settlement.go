package keeper

import (
	"encoding/binary"
	"math/big"
	"strings"

	"github.com/xitcoin-xtc/pos-chain/x/bridge/types"

	errorsmod "cosmossdk.io/errors"
	sdkmath "cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

var (
	ErrSettlementUnavailable    = types.ErrSettlementUnavailable
	ErrSettlementDirection      = types.ErrSettlementDirection
	ErrOutstandingLimitExceeded = types.ErrOutstandingLimitExceeded
	ErrInsufficientOutstanding  = types.ErrInsufficientOutstanding
	ErrMaximumSupplyExceeded    = types.ErrMaximumSupplyExceeded
	ErrBlockedDestination       = types.ErrBlockedDestination
)

func (k Keeper) OutstandingAmount(ctx sdk.Context) sdkmath.Int {
	value := ctx.KVStore(k.storeKey).Get(types.KeyOutstandingAmount)
	if len(value) == 0 {
		return sdkmath.ZeroInt()
	}
	return sdkmath.NewIntFromBigInt(new(big.Int).SetBytes(value))
}

func (k Keeper) setOutstandingAmount(ctx sdk.Context, amount sdkmath.Int) {
	if amount.IsZero() {
		ctx.KVStore(k.storeKey).Delete(types.KeyOutstandingAmount)
		return
	}
	ctx.KVStore(k.storeKey).Set(types.KeyOutstandingAmount, amount.BigInt().Bytes())
}

func (k Keeper) SettleInbound(ctx sdk.Context, config types.RouteConfig, attestation types.Attestation) error {
	if k.bankKeeper == nil {
		return ErrSettlementUnavailable
	}
	if attestation.Direction != types.DirectionCronosToXitcoin {
		return ErrSettlementDirection
	}
	recipient, err := sdk.AccAddressFromBech32(strings.TrimSpace(attestation.Destination))
	if err != nil {
		return errorsmod.Wrap(types.ErrInvalidXitcoinDestination, err.Error())
	}
	if k.bankKeeper.BlockedAddr(recipient) {
		return ErrBlockedDestination
	}
	amount, err := types.ParseAmount(attestation.Amount)
	if err != nil {
		return err
	}
	limits, err := config.Limits()
	if err != nil {
		return err
	}
	// SafeAdd: a sum beyond 256 bits exceeds every limit instead of panicking.
	nextOutstanding, err := k.OutstandingAmount(ctx).SafeAdd(amount)
	if err != nil || nextOutstanding.GT(limits.MaxOutstandingAmount) {
		return ErrOutstandingLimitExceeded
	}
	nextSupply, err := k.bankKeeper.GetSupply(ctx, k.nativeDenom).Amount.SafeAdd(amount)
	if err != nil || nextSupply.GT(k.maxSupply) {
		return ErrMaximumSupplyExceeded
	}
	coins := sdk.NewCoins(sdk.NewCoin(k.nativeDenom, amount))
	if err := k.bankKeeper.MintCoins(ctx, types.ModuleName, coins); err != nil {
		return err
	}
	if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, recipient, coins); err != nil {
		return err
	}
	k.setOutstandingAmount(ctx, nextOutstanding)
	return nil
}

func (k Keeper) InitiateOutboundTransfer(ctx sdk.Context, config types.RouteConfig, sender sdk.AccAddress, destination, amountText string) ([32]byte, uint64, error) {
	if k.bankKeeper == nil {
		return [32]byte{}, 0, ErrSettlementUnavailable
	}
	if err := k.RequireRouteAvailable(ctx, config); err != nil {
		return [32]byte{}, 0, err
	}
	cronosDestination, err := types.ValidateCronosDestination(destination)
	if err != nil {
		return [32]byte{}, 0, err
	}
	amount, err := types.ParseAmount(amountText)
	if err != nil {
		return [32]byte{}, 0, err
	}
	if err := k.checkAndRecordLimits(ctx, config, amount); err != nil {
		return [32]byte{}, 0, err
	}
	outstanding := k.OutstandingAmount(ctx)
	if amount.GT(outstanding) {
		return [32]byte{}, 0, ErrInsufficientOutstanding
	}
	coins := sdk.NewCoins(sdk.NewCoin(k.nativeDenom, amount))
	if err := k.bankKeeper.SendCoinsFromAccountToModule(ctx, sender, types.ModuleName, coins); err != nil {
		return [32]byte{}, 0, err
	}
	if err := k.bankKeeper.BurnCoins(ctx, types.ModuleName, coins); err != nil {
		return [32]byte{}, 0, err
	}
	nonce := k.nextOutboundNonce(ctx)
	id := types.OutboundRequestID(config.RouteID, sender.String(), cronosDestination, amount, nonce)
	k.setOutstandingAmount(ctx, outstanding.Sub(amount))
	// The record returned by the OutboundTransfer query: the burned values,
	// and the block height and hash of this transaction.
	if err := k.setOutboundTransfer(ctx, id, types.OutboundTransferRecord{
		RequestId:   types.HexID(id),
		RouteId:     config.RouteID,
		Sender:      sender.String(),
		Destination: cronosDestination.Hex(),
		Amount:      amount.String(),
		Nonce:       nonce,
		Height:      ctx.BlockHeight(),
		TxHash:      types.TxHash(ctx.TxBytes()),
	}); err != nil {
		return [32]byte{}, 0, err
	}
	return id, nonce, nil
}

func (k Keeper) nextOutboundNonce(ctx sdk.Context) uint64 {
	store := ctx.KVStore(k.storeKey)
	value := store.Get(types.KeyOutboundNonce)
	var current uint64
	if len(value) == 8 {
		current = binary.BigEndian.Uint64(value)
	}
	next := current + 1
	encoded := make([]byte, 8)
	binary.BigEndian.PutUint64(encoded, next)
	store.Set(types.KeyOutboundNonce, encoded)
	return next
}
