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

func controlServerSetup(t *testing.T) (sdk.Context, Keeper, types.RouteConfig, []*ecdsa.PrivateKey, *ecdsa.PrivateKey) {
	t.Helper()
	key := storetypes.NewKVStoreKey(types.StoreKey)
	ctx := sdktestutil.DefaultContext(key, storetypes.NewTransientStoreKey("bridge_msg_control_test")).WithBlockTime(time.Unix(1800000100, 0))
	k := NewKeeper(key)
	keys := make([]*ecdsa.PrivateKey, 3)
	signers := make([]string, 3)
	for i := range keys {
		var err error
		keys[i], err = crypto.GenerateKey()
		if err != nil {
			t.Fatal(err)
		}
		signers[i] = crypto.PubkeyToAddress(keys[i].PublicKey).Hex()
	}
	guardian, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	config := types.RouteConfig{RouteID: "cronos-testnet-xitcoin-testnet", BridgeSigners: signers, Guardian: crypto.PubkeyToAddress(guardian.PublicKey).Hex(), MaxTransferAmount: "10", DailyLimit: "100", MaxOutstandingAmount: "1000", Enabled: true}
	if err := k.SetRouteConfig(ctx, config); err != nil {
		t.Fatal(err)
	}
	return ctx, k, config, keys, guardian
}

func signControl(t *testing.T, action types.ControlAction, keys []*ecdsa.PrivateKey) [][]byte {
	t.Helper()
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
	return signatures
}

func TestMsgServerEmergencyPauseAndControlledResume(t *testing.T) {
	ctx, k, config, keys, guardian := controlServerSetup(t)
	server := NewMsgServer(k)
	submitter := sdk.AccAddress(make([]byte, 20)).String()
	pauseAction := types.GuardianPauseAction{RouteID: config.RouteID, Nonce: 1, ExpiresUnix: 1800003600}
	pauseDigest, err := types.GuardianPauseDigest(pauseAction)
	if err != nil {
		t.Fatal(err)
	}
	pauseSignature, err := crypto.Sign(pauseDigest.Bytes(), guardian)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.EmergencyPauseRoute(sdk.WrapSDKContext(ctx), &types.MsgEmergencyPauseRoute{Submitter: submitter, RouteId: config.RouteID, Nonce: 1, ExpiresUnix: 1800003600, GuardianSignature: pauseSignature}); err != nil {
		t.Fatalf("guardian pause rejected: %v", err)
	}
	state, err := k.GetRouteState(ctx)
	if err != nil || !state.Paused {
		t.Fatalf("route not paused: state=%+v err=%v", state, err)
	}
	payloadHash, err := RouteStatePayloadHash(config, RouteState{Paused: false})
	if err != nil {
		t.Fatal(err)
	}
	resumeAction := types.ControlAction{RouteID: config.RouteID, Action: types.ActionResumeRoute, PayloadHash: common.BytesToHash(payloadHash[:]).Hex(), Nonce: 2, NotBeforeUnix: 1800000000, ExpiresUnix: 1800003600}
	resume := &types.MsgResumeRoute{Submitter: submitter, RouteId: config.RouteID, Nonce: 2, NotBeforeUnix: 1800000000, ExpiresUnix: 1800003600, Signatures: signControl(t, resumeAction, keys)}
	if _, err := server.ResumeRoute(sdk.WrapSDKContext(ctx), resume); err != nil {
		t.Fatalf("threshold resume rejected: %v", err)
	}
	if err := k.RequireRouteAvailable(ctx, config); err != nil {
		t.Fatalf("resumed route unavailable: %v", err)
	}
}

func TestMsgServerRouteUpdateUsesCurrentSignerSet(t *testing.T) {
	ctx, k, current, keys, _ := controlServerSetup(t)
	next := current
	next.Enabled = false
	next.DailyLimit = "50"
	payloadHash, err := types.RouteConfigPayloadHash(next)
	if err != nil {
		t.Fatal(err)
	}
	action := types.ControlAction{RouteID: current.RouteID, Action: types.ActionUpdateRouteConfig, PayloadHash: common.BytesToHash(payloadHash[:]).Hex(), Nonce: 3, NotBeforeUnix: 1800000000, ExpiresUnix: 1800003600}
	msg := &types.MsgUpdateRouteConfig{Submitter: sdk.AccAddress(make([]byte, 20)).String(), RouteId: next.RouteID, BridgeSigners: next.BridgeSigners, Guardian: next.Guardian, MaxTransferAmount: next.MaxTransferAmount, DailyLimit: next.DailyLimit, MaxOutstandingAmount: next.MaxOutstandingAmount, Enabled: next.Enabled, Nonce: action.Nonce, NotBeforeUnix: action.NotBeforeUnix, ExpiresUnix: action.ExpiresUnix, Signatures: signControl(t, action, keys)}
	if _, err := NewMsgServer(k).UpdateRouteConfig(sdk.WrapSDKContext(ctx), msg); err != nil {
		t.Fatalf("approved route update rejected: %v", err)
	}
	stored, found, err := k.GetRouteConfig(ctx)
	if err != nil || !found || stored.Enabled || stored.DailyLimit != "50" {
		t.Fatalf("route update not stored: config=%+v found=%v err=%v", stored, found, err)
	}
}
