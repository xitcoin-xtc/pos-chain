package keeper

import (
	"crypto/ecdsa"
	"testing"
	"time"

	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	sdktestutil "github.com/cosmos/cosmos-sdk/testutil"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/xitcoin-xtc/pos-chain/x/bridge/types"
)

func pausedRouteForResume(t *testing.T) (sdkContext sdk.Context, k Keeper, config types.RouteConfig, keys []*ecdsa.PrivateKey) {
	t.Helper()
	key := storetypes.NewKVStoreKey(types.StoreKey)
	sdkContext = sdktestutil.DefaultContext(key, storetypes.NewTransientStoreKey("bridge_resume_test")).WithBlockTime(time.Unix(1800000100, 0))
	k = NewKeeper(key)
	keys = make([]*ecdsa.PrivateKey, 3)
	signers := make([]string, 3)
	for i := range keys {
		key, err := crypto.GenerateKey()
		if err != nil {
			t.Fatal(err)
		}
		keys[i] = key
		signers[i] = crypto.PubkeyToAddress(key.PublicKey).Hex()
	}
	config = types.RouteConfig{
		RouteID: "cronos-testnet-xitcoin-testnet", BridgeSigners: signers,
		Guardian:          common.HexToAddress("0x0000000000000000000000000000000000000004").Hex(),
		MaxTransferAmount: "10", DailyLimit: "15",
		MaxOutstandingAmount: "1000000000000000000000000000", Enabled: true,
	}
	sdkContext.KVStore(key).Set(routeStateKey, []byte(`{"paused":true}`))
	return sdkContext, k, config, keys
}

func signedResume(t *testing.T, config types.RouteConfig, keys []*ecdsa.PrivateKey) (types.ControlAction, [][]byte) {
	t.Helper()
	payloadHash, err := RouteStatePayloadHash(config, RouteState{Paused: false})
	if err != nil {
		t.Fatal(err)
	}
	action := types.ControlAction{RouteID: config.RouteID, Action: types.ActionResumeRoute, PayloadHash: common.BytesToHash(payloadHash[:]).Hex(), Nonce: 1, NotBeforeUnix: 1800000000, ExpiresUnix: 1800003600}
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

func TestResumeRouteRequiresBoundTwoOfThreeApproval(t *testing.T) {
	ctx, k, config, keys := pausedRouteForResume(t)
	action, signatures := signedResume(t, config, keys)
	if err := k.ResumeRoute(ctx, config, action, signatures); err != nil {
		t.Fatalf("approved resume rejected: %v", err)
	}
	if err := k.RequireRouteAvailable(ctx, config); err != nil {
		t.Fatalf("resumed route remained unavailable: %v", err)
	}
	if err := k.ResumeRoute(ctx, config, action, signatures); err != ErrRouteNotPaused {
		t.Fatalf("repeat resume accepted: %v", err)
	}
}

func TestResumeRouteRejectsOneApproval(t *testing.T) {
	ctx, k, config, keys := pausedRouteForResume(t)
	action, signatures := signedResume(t, config, keys)
	if err := k.ResumeRoute(ctx, config, action, signatures[:1]); err == nil {
		t.Fatal("one signer resume was accepted")
	}
}
