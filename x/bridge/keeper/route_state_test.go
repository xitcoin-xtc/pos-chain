package keeper

import (
	"testing"
	"time"

	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	sdktestutil "github.com/cosmos/cosmos-sdk/testutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/xitcoin-xtc/pos-chain/x/bridge/types"
)

func TestGuardianCanOnlyPauseRoute(t *testing.T) {
	key := storetypes.NewKVStoreKey(types.StoreKey)
	ctx := sdktestutil.DefaultContext(key, storetypes.NewTransientStoreKey("bridge_pause_test")).WithBlockTime(time.Unix(1800000000, 0))
	k := NewKeeper(key)
	guardian, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	config := keeperRouteConfig()
	config.Enabled = true
	config.Guardian = crypto.PubkeyToAddress(guardian.PublicKey).Hex()
	action := types.GuardianPauseAction{RouteID: config.RouteID, Nonce: 1, ExpiresUnix: 1800003600}
	digest, err := types.GuardianPauseDigest(action)
	if err != nil {
		t.Fatal(err)
	}
	signature, err := crypto.Sign(digest.Bytes(), guardian)
	if err != nil {
		t.Fatal(err)
	}
	if err := k.PauseRoute(ctx, config, action, signature); err != nil {
		t.Fatalf("guardian pause rejected: %v", err)
	}
	if err := k.RequireRouteAvailable(ctx, config); err != ErrRoutePaused {
		t.Fatalf("paused route remained available: %v", err)
	}
	if err := k.PauseRoute(ctx, config, action, signature); err != ErrControlActionProcessed {
		t.Fatalf("replayed guardian pause accepted: %v", err)
	}
}

func TestNonGuardianCannotPauseRoute(t *testing.T) {
	key := storetypes.NewKVStoreKey(types.StoreKey)
	ctx := sdktestutil.DefaultContext(key, storetypes.NewTransientStoreKey("bridge_bad_pause_test")).WithBlockTime(time.Unix(1800000000, 0))
	k := NewKeeper(key)
	guardian, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	other, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	config := keeperRouteConfig()
	config.Guardian = crypto.PubkeyToAddress(guardian.PublicKey).Hex()
	action := types.GuardianPauseAction{RouteID: config.RouteID, Nonce: 1, ExpiresUnix: 1800003600}
	digest, err := types.GuardianPauseDigest(action)
	if err != nil {
		t.Fatal(err)
	}
	signature, err := crypto.Sign(digest.Bytes(), other)
	if err != nil {
		t.Fatal(err)
	}
	if err := k.PauseRoute(ctx, config, action, signature); err == nil {
		t.Fatal("non-guardian pause was accepted")
	}
}
