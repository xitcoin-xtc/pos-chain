package keeper

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math/big"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/xitcoin-xtc/pos-chain/x/bridge/types"
)

var (
	ErrRouteDisabled         = errors.New("bridge route is disabled")
	ErrTransferLimitExceeded = errors.New("bridge transfer limit exceeded")
	ErrDailyLimitExceeded    = errors.New("bridge daily limit exceeded")
)

var dailyUsagePrefix = []byte{0x03}

// CheckAndRecordLimits uses the block timestamp, not relayer or user time.
// The write participates in the surrounding transaction and rolls back with it.
func (k Keeper) CheckAndRecordLimits(ctx sdk.Context, config types.RouteConfig, amountText string) error {
	if err := k.RequireRouteAvailable(ctx, config); err != nil {
		return err
	}
	amount, ok := new(big.Int).SetString(amountText, 10)
	if !ok || amount.Sign() <= 0 {
		return errors.New("bridge amount must be positive")
	}
	maxTransfer, _ := new(big.Int).SetString(config.MaxTransferAmount, 10)
	dailyLimit, _ := new(big.Int).SetString(config.DailyLimit, 10)
	if amount.Cmp(maxTransfer) > 0 {
		return ErrTransferLimitExceeded
	}

	key := dailyUsageKey(config.RouteID, ctx.BlockTime().Unix())
	used := new(big.Int).SetBytes(ctx.KVStore(k.storeKey).Get(key))
	next := new(big.Int).Add(used, amount)
	if next.Cmp(dailyLimit) > 0 {
		return ErrDailyLimitExceeded
	}
	ctx.KVStore(k.storeKey).Set(key, next.Bytes())
	return nil
}

func (k Keeper) DailyUsage(ctx sdk.Context, routeID string) *big.Int {
	return new(big.Int).SetBytes(ctx.KVStore(k.storeKey).Get(dailyUsageKey(routeID, ctx.BlockTime().Unix())))
}

func dailyUsageKey(routeID string, unixTime int64) []byte {
	routeHash := sha256.Sum256([]byte(routeID))
	key := make([]byte, len(dailyUsagePrefix)+len(routeHash)+8)
	copy(key, dailyUsagePrefix)
	copy(key[len(dailyUsagePrefix):], routeHash[:])
	binary.BigEndian.PutUint64(key[len(dailyUsagePrefix)+len(routeHash):], uint64(unixTime/86400))
	return key
}
