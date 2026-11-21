package keeper

import (
	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	sdktestutil "github.com/cosmos/cosmos-sdk/testutil"
	"github.com/xitcoin-xtc/pos-chain/x/bridge/types"
	"testing"
)

func TestEmptyGenesisLeavesBridgeUnconfigured(t *testing.T) {
	key := storetypes.NewKVStoreKey(types.StoreKey)
	ctx := sdktestutil.DefaultContext(key, storetypes.NewTransientStoreKey("bridge_genesis_test"))
	k := NewKeeper(key)
	k.InitGenesis(ctx, types.DefaultGenesisState())
	exported := k.ExportGenesis(ctx)
	if exported.RouteConfig != nil || exported.Paused {
		t.Fatal("empty genesis unexpectedly configured bridge")
	}
}
