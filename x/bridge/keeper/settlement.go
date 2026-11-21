package keeper

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"strings"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"

	"github.com/xitcoin-xtc/pos-chain/x/bridge/types"
)

var (
	ErrSettlementUnavailable    = errors.New("bridge settlement is unavailable")
	ErrSettlementDirection      = errors.New("bridge attestation is not an inbound Cronos transfer")
	ErrOutstandingLimitExceeded = errors.New("bridge outstanding limit exceeded")
	ErrInsufficientOutstanding  = errors.New("bridge outstanding amount is insufficient")
	ErrMaximumSupplyExceeded    = errors.New("XTC maximum supply exceeded")
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
		return fmt.Errorf("invalid Xitcoin destination: %w", err)
	}
	amount, ok := sdkmath.NewIntFromString(attestation.Amount)
	if !ok || !amount.IsPositive() {
		return errors.New("bridge amount must be positive")
	}
	outstandingLimit, _ := sdkmath.NewIntFromString(config.MaxOutstandingAmount)
	nextOutstanding := k.OutstandingAmount(ctx).Add(amount)
	if nextOutstanding.GT(outstandingLimit) {
		return ErrOutstandingLimitExceeded
	}
	if k.bankKeeper.GetSupply(ctx, k.nativeDenom).Amount.Add(amount).GT(k.maxSupply) {
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
	destination = strings.TrimSpace(destination)
	if !common.IsHexAddress(destination) || common.HexToAddress(destination) == (common.Address{}) {
		return [32]byte{}, 0, errors.New("invalid Cronos destination")
	}
	amount, ok := sdkmath.NewIntFromString(amountText)
	if !ok || !amount.IsPositive() {
		return [32]byte{}, 0, errors.New("bridge amount must be positive")
	}
	if err := k.CheckAndRecordLimits(ctx, config, amountText); err != nil {
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
	payload := strings.Join([]string{config.RouteID, sender.String(), strings.ToLower(common.HexToAddress(destination).Hex()), amount.String(), fmt.Sprintf("%d", nonce)}, "\x00")
	id := sha256.Sum256([]byte(payload))
	k.setOutstandingAmount(ctx, outstanding.Sub(amount))
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
