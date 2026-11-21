package keeper

import (
	"testing"
	"time"

	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	sdktestutil "github.com/cosmos/cosmos-sdk/testutil"

	"github.com/xitcoin-xtc/pos-chain/x/bridge/types"
)

func limitsConfig() types.RouteConfig {
	return types.RouteConfig{
		RouteID: "cronos-testnet-xitcoin-testnet",
		BridgeSigners: []string{
			"0x0000000000000000000000000000000000000001",
			"0x0000000000000000000000000000000000000002",
			"0x0000000000000000000000000000000000000003",
		},
		Guardian:             "0x0000000000000000000000000000000000000004",
		MaxTransferAmount:    "10",
		DailyLimit:           "15",
		MaxOutstandingAmount: "1000000000000000000000000000",
		Enabled:              true,
	}
}

func TestLimitsRejectDisabledRouteAndExcesses(t *testing.T) {
	key := storetypes.NewKVStoreKey(types.StoreKey)
	ctx := sdktestutil.DefaultContext(key, storetypes.NewTransientStoreKey("bridge_limits_test")).
		WithBlockTime(time.Unix(1800000000, 0))
	k := NewKeeper(key)

	disabled := limitsConfig()
	disabled.Enabled = false
	if err := k.CheckAndRecordLimits(ctx, disabled, "1"); err != ErrRouteDisabled {
		t.Fatalf("disabled route error = %v", err)
	}
	if err := k.CheckAndRecordLimits(ctx, limitsConfig(), "11"); err != ErrTransferLimitExceeded {
		t.Fatalf("transfer limit error = %v", err)
	}
}

func TestLimitsTrackDailyUsageByBlockDay(t *testing.T) {
	key := storetypes.NewKVStoreKey(types.StoreKey)
	ctx := sdktestutil.DefaultContext(key, storetypes.NewTransientStoreKey("bridge_daily_test")).
		WithBlockTime(time.Unix(1800000000, 0))
	k := NewKeeper(key)
	config := limitsConfig()

	if err := k.CheckAndRecordLimits(ctx, config, "10"); err != nil {
		t.Fatal(err)
	}
	if err := k.CheckAndRecordLimits(ctx, config, "6"); err != ErrDailyLimitExceeded {
		t.Fatalf("daily limit error = %v", err)
	}
	if got := k.DailyUsage(ctx, config.RouteID).String(); got != "10" {
		t.Fatalf("daily usage = %s, want 10", got)
	}

	nextDay := ctx.WithBlockTime(ctx.BlockTime().Add(24 * time.Hour))
	if err := k.CheckAndRecordLimits(nextDay, config, "10"); err != nil {
		t.Fatalf("next day should reset usage: %v", err)
	}
}
