package keeper

import (
	"testing"
	"time"

	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	sdktestutil "github.com/cosmos/cosmos-sdk/testutil"

	"github.com/xitcoin-xtc/pos-chain/x/bridge/types"
)

func TestConsumeAttestationAtRejectsExpiredInput(t *testing.T) {
	key := storetypes.NewKVStoreKey(types.StoreKey)
	ctx := sdktestutil.DefaultContext(key, storetypes.NewTransientStoreKey("bridge_expiry_test")).
		WithBlockTime(time.Unix(1800000001, 0))
	k := NewKeeper(key)

	if _, err := k.ConsumeAttestationAt(ctx, testAttestation()); err != types.ErrAttestationExpired {
		t.Fatalf("expiry error = %v, want %v", err, types.ErrAttestationExpired)
	}
}
