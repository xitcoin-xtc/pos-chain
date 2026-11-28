package keeper

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math/big"

	"github.com/xitcoin-xtc/pos-chain/x/bridge/types"

	sdkmath "cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

var (
	ErrRouteDisabled         = errors.New("bridge route is disabled")
	ErrTransferLimitExceeded = errors.New("bridge transfer limit exceeded")
	ErrDailyLimitExceeded    = errors.New("bridge daily limit exceeded")
)

var dailyUsagePrefix = []byte{0x03}

// CheckAndRecordLimits uses the block timestamp, not relayer or user time.
// The write participates in the surrounding transaction and rolls back with it.
// amountText and the route limits are parsed with types.ParseAmount, the
// parser used for minting and burning, so the amount counted here is always
// the amount that is minted or burned.
func (k Keeper) CheckAndRecordLimits(ctx sdk.Context, config types.RouteConfig, amountText string) error {
	if err := k.RequireRouteAvailable(ctx, config); err != nil {
		return err
	}
	amount, err := types.ParseAmount(amountText)
	if err != nil {
		return err
	}
	return k.checkAndRecordLimits(ctx, config, amount)
}

func (k Keeper) checkAndRecordLimits(ctx sdk.Context, config types.RouteConfig, amount sdkmath.Int) error {
	limits, err := config.Limits()
	if err != nil {
		return err
	}
	if !amount.IsPositive() {
		return types.ErrNonCanonicalAmount
	}
	if amount.GT(limits.MaxTransferAmount) {
		return ErrTransferLimitExceeded
	}

	key := dailyUsageKey(config.RouteID, ctx.BlockTime().Unix())
	used := new(big.Int).SetBytes(ctx.KVStore(k.storeKey).Get(key))
	next := new(big.Int).Add(used, amount.BigInt())
	if next.Cmp(limits.DailyLimit.BigInt()) > 0 {
		return ErrDailyLimitExceeded
	}
	ctx.KVStore(k.storeKey).Set(key, next.Bytes())
	return nil
}

func (k Keeper) DailyUsage(ctx sdk.Context, routeID string) *big.Int {
	return new(big.Int).SetBytes(ctx.KVStore(k.storeKey).Get(dailyUsageKey(routeID, ctx.BlockTime().Unix())))
}

func dailyUsageKey(routeID string, unixTime int64) []byte {
	return dailyUsageKeyForDay(routeID, UTCDay(unixTime))
}

// UTCDay returns the number of whole days between 1970-01-01T00:00:00Z and a
// Unix time: the day of the daily-limit bucket. Block times are later than
// 1970; an earlier time maps to day 0.
func UTCDay(unixTime int64) uint64 {
	day := unixTime / 86400
	if day < 0 {
		return 0
	}
	return uint64(day)
}

// dailyUsageKeyForDay is 0x03 || sha256(route ID) || big-endian UTC day.
func dailyUsageKeyForDay(routeID string, day uint64) []byte {
	routeHash := sha256.Sum256([]byte(routeID))
	key := make([]byte, len(dailyUsagePrefix)+len(routeHash)+8)
	copy(key, dailyUsagePrefix)
	copy(key[len(dailyUsagePrefix):], routeHash[:])
	binary.BigEndian.PutUint64(key[len(dailyUsagePrefix)+len(routeHash):], day)
	return key
}
