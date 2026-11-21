package keeper

import (
	"crypto/ecdsa"
	"testing"
	"time"

	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	sdktestutil "github.com/cosmos/cosmos-sdk/testutil"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/xitcoin-xtc/pos-chain/x/bridge/types"
)

func controlledRouteConfig(t *testing.T) (types.RouteConfig, []*ecdsa.PrivateKey) {
	t.Helper()
	keys := make([]*ecdsa.PrivateKey, 3)
	signers := make([]string, 3)
	for i := range keys {
		key, err := crypto.GenerateKey()
		if err != nil {
			t.Fatal(err)
		}
		keys[i] = key
		signers[i] = crypto.PubkeyToAddress(key.PublicKey).Hex()
	}
	return types.RouteConfig{
		RouteID:              "cronos-testnet-xitcoin-testnet",
		BridgeSigners:        signers,
		Guardian:             common.HexToAddress("0x0000000000000000000000000000000000000004").Hex(),
		MaxTransferAmount:    "1000000000000000000",
		DailyLimit:           "5000000000000000000",
		MaxOutstandingAmount: "1000000000000000000000000000",
	}, keys
}

func signedConfigAction(t *testing.T, current, next types.RouteConfig, keys []*ecdsa.PrivateKey) (types.ControlAction, [][]byte) {
	t.Helper()
	payloadHash, err := types.RouteConfigPayloadHash(next)
	if err != nil {
		t.Fatal(err)
	}
	action := types.ControlAction{
		RouteID:       current.RouteID,
		Action:        types.ActionUpdateRouteConfig,
		PayloadHash:   common.BytesToHash(payloadHash[:]).Hex(),
		Nonce:         1,
		NotBeforeUnix: 1800000000,
		ExpiresUnix:   1800003600,
	}
	digest, err := types.ControlDigest(action)
	if err != nil {
		t.Fatal(err)
	}
	signatures := make([][]byte, 2)
	for i := range signatures {
		signatures[i], err = crypto.Sign(digest.Bytes(), keys[i])
		if err != nil {
			t.Fatal(err)
		}
	}
	return action, signatures
}

func TestApplyRouteConfigUpdateRequiresBoundTwoOfThreeApproval(t *testing.T) {
	key := storetypes.NewKVStoreKey(types.StoreKey)
	ctx := sdktestutil.DefaultContext(key, storetypes.NewTransientStoreKey("bridge_control_apply_test"))
	ctx = ctx.WithBlockTime(time.Unix(1800000100, 0))
	k := NewKeeper(key)
	current, keys := controlledRouteConfig(t)
	if err := k.SetRouteConfig(ctx, current); err != nil {
		t.Fatal(err)
	}
	next := current
	next.MaxTransferAmount = "2000000000000000000"
	action, signatures := signedConfigAction(t, current, next, keys)
	if err := k.ApplyRouteConfigUpdate(ctx, current, action, next, signatures); err != nil {
		t.Fatalf("approved route update rejected: %v", err)
	}
	stored, found, err := k.GetRouteConfig(ctx)
	if err != nil || !found || stored.MaxTransferAmount != next.MaxTransferAmount {
		t.Fatalf("approved route update not stored: found=%v err=%v", found, err)
	}
	if err := k.ApplyRouteConfigUpdate(ctx, next, action, next, signatures); err != ErrControlActionProcessed {
		t.Fatalf("replayed control action was accepted: %v", err)
	}
}

func TestApplyRouteConfigUpdateRejectsWrongPayloadAndOneSignature(t *testing.T) {
	key := storetypes.NewKVStoreKey(types.StoreKey)
	ctx := sdktestutil.DefaultContext(key, storetypes.NewTransientStoreKey("bridge_control_reject_test"))
	ctx = ctx.WithBlockTime(time.Unix(1800000100, 0))
	k := NewKeeper(key)
	current, keys := controlledRouteConfig(t)
	next := current
	next.DailyLimit = "6000000000000000000"
	action, signatures := signedConfigAction(t, current, next, keys)
	if err := k.ApplyRouteConfigUpdate(ctx, current, action, next, signatures[:1]); err == nil {
		t.Fatal("one signer approval was accepted")
	}

	wrong := next
	wrong.DailyLimit = "7000000000000000000"
	if err := k.ApplyRouteConfigUpdate(ctx, current, action, wrong, signatures); err != ErrControlPayloadMismatch {
		t.Fatalf("mismatched payload was accepted: %v", err)
	}
}
