package keeper

import (
	"testing"

	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	sdktestutil "github.com/cosmos/cosmos-sdk/testutil"

	"github.com/xitcoin-xtc/pos-chain/x/bridge/types"
)

func keeperRouteConfig() types.RouteConfig {
	return types.RouteConfig{
		RouteID: "cronos-testnet-xitcoin-testnet",
		BridgeSigners: []string{
			"0x0000000000000000000000000000000000000001",
			"0x0000000000000000000000000000000000000002",
			"0x0000000000000000000000000000000000000003",
		},
		Guardian:             "0x0000000000000000000000000000000000000004",
		MaxTransferAmount:    "1000000000000000000",
		DailyLimit:           "5000000000000000000",
		MaxOutstandingAmount: "1000000000000000000000000000",
	}
}

func TestRouteConfigRoundTrip(t *testing.T) {
	key := storetypes.NewKVStoreKey(types.StoreKey)
	ctx := sdktestutil.DefaultContext(key, storetypes.NewTransientStoreKey("bridge_config_test"))
	k := NewKeeper(key)

	if _, found, err := k.GetRouteConfig(ctx); err != nil || found {
		t.Fatalf("unexpected initial config state: found=%v err=%v", found, err)
	}
	want := keeperRouteConfig()
	if err := k.SetRouteConfig(ctx, want); err != nil {
		t.Fatalf("route config rejected: %v", err)
	}
	got, found, err := k.GetRouteConfig(ctx)
	if err != nil || !found {
		t.Fatalf("stored route config missing: found=%v err=%v", found, err)
	}
	if got.RouteID != want.RouteID || got.Guardian != want.Guardian || got.DailyLimit != want.DailyLimit {
		t.Fatal("stored route config changed")
	}
}

func TestRouteConfigRejectsInvalidInput(t *testing.T) {
	key := storetypes.NewKVStoreKey(types.StoreKey)
	ctx := sdktestutil.DefaultContext(key, storetypes.NewTransientStoreKey("bridge_bad_config_test"))
	k := NewKeeper(key)
	config := keeperRouteConfig()
	config.MaxTransferAmount = "0"
	if err := k.SetRouteConfig(ctx, config); err == nil {
		t.Fatal("invalid route config was stored")
	}
}
