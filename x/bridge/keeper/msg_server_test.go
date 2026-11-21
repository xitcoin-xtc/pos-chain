package keeper

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	sdktestutil "github.com/cosmos/cosmos-sdk/testutil"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/xitcoin-xtc/pos-chain/x/bridge/types"
)

func TestSubmitAttestationRejectsDefaultUnconfiguredBridge(t *testing.T) {
	key := storetypes.NewKVStoreKey(types.StoreKey)
	ctx := sdktestutil.DefaultContext(key, storetypes.NewTransientStoreKey("bridge_msg_disabled_test")).WithBlockTime(time.Unix(1800000000, 0))
	k, err := NewSettlementKeeper(key, &fakeBankKeeper{supply: sdkmath.ZeroInt()}, "axtc", "1000", testAuthority())
	if err != nil {
		t.Fatal(err)
	}
	msg := validBridgeMessage(t, "cronos-testnet-xitcoin-testnet", nil)

	if _, err := NewMsgServer(k).SubmitAttestation(sdk.WrapSDKContext(ctx), &msg); err != ErrRouteDisabled {
		t.Fatalf("unconfigured bridge error = %v, want %v", err, ErrRouteDisabled)
	}
}

func TestInitializeRouteConfigRequiresAuthorityAndRunsOncePaused(t *testing.T) {
	key := storetypes.NewKVStoreKey(types.StoreKey)
	ctx := sdktestutil.DefaultContext(key, storetypes.NewTransientStoreKey("bridge_initialize_test"))
	k, err := NewSettlementKeeper(key, &fakeBankKeeper{supply: sdkmath.ZeroInt()}, "axtc", "1000", testAuthority())
	if err != nil {
		t.Fatal(err)
	}
	request := &types.MsgInitializeRouteConfig{
		Authority: testAuthority(), RouteId: "cronos-testnet-xitcoin-testnet",
		BridgeSigners: []string{
			"0x0000000000000000000000000000000000000001",
			"0x0000000000000000000000000000000000000002",
			"0x0000000000000000000000000000000000000003",
		},
		Guardian: "0x0000000000000000000000000000000000000004",
		MaxTransferAmount: "10", DailyLimit: "100", MaxOutstandingAmount: "1000",
	}
	wrong := *request
	wrong.Authority = sdk.AccAddress(bytes.Repeat([]byte{9}, 20)).String()
	if _, err := NewMsgServer(k).InitializeRouteConfig(sdk.WrapSDKContext(ctx), &wrong); err == nil {
		t.Fatal("non-authority initialized the bridge route")
	}
	if _, found, err := k.GetRouteConfig(ctx); err != nil || found {
		t.Fatalf("rejected initialization changed route state: found=%v err=%v", found, err)
	}
	if _, err := NewMsgServer(k).InitializeRouteConfig(sdk.WrapSDKContext(ctx), request); err != nil {
		t.Fatalf("authority initialization rejected: %v", err)
	}
	config, found, err := k.GetRouteConfig(ctx)
	if err != nil || !found {
		t.Fatalf("initialized route not stored: found=%v err=%v", found, err)
	}
	if config.Enabled {
		t.Fatal("initial route was enabled")
	}
	state, err := k.GetRouteState(ctx)
	if err != nil || !state.Paused {
		t.Fatalf("initial route was not paused: state=%+v err=%v", state, err)
	}
	if _, err := NewMsgServer(k).InitializeRouteConfig(sdk.WrapSDKContext(ctx), request); err == nil {
		t.Fatal("second initial route configuration was accepted")
	}
}

func TestSubmitAttestationRecordsOnlyApprovedAttestation(t *testing.T) {
	key := storetypes.NewKVStoreKey(types.StoreKey)
	ctx := sdktestutil.DefaultContext(key, storetypes.NewTransientStoreKey("bridge_msg_admit_test")).WithBlockTime(time.Unix(1800000000, 0))
	bank := &fakeBankKeeper{supply: sdkmath.ZeroInt()}
	k, err := NewSettlementKeeper(key, bank, "axtc", "1000", testAuthority())
	if err != nil {
		t.Fatal(err)
	}

	first, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	second, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	third, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	guardian, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	config := types.RouteConfig{
		RouteID:              "cronos-testnet-xitcoin-testnet",
		BridgeSigners:        []string{crypto.PubkeyToAddress(first.PublicKey).Hex(), crypto.PubkeyToAddress(second.PublicKey).Hex(), crypto.PubkeyToAddress(third.PublicKey).Hex()},
		Guardian:             crypto.PubkeyToAddress(guardian.PublicKey).Hex(),
		MaxTransferAmount:    "10",
		DailyLimit:           "15",
		MaxOutstandingAmount: "1000000000000000000000000000",
		Enabled:              true,
	}
	if err := k.SetRouteConfig(ctx, config); err != nil {
		t.Fatal(err)
	}
	msg := validBridgeMessage(t, config.RouteID, nil)
	attestation := msg.Attestation()
	digest, err := types.SigningDigest(attestation)
	if err != nil {
		t.Fatal(err)
	}
	firstSignature, err := crypto.Sign(digest.Bytes(), first)
	if err != nil {
		t.Fatal(err)
	}
	secondSignature, err := crypto.Sign(digest.Bytes(), second)
	if err != nil {
		t.Fatal(err)
	}
	msg.Signatures = [][]byte{firstSignature, secondSignature}

	response, err := NewMsgServer(k).SubmitAttestation(sdk.WrapSDKContext(ctx), &msg)
	if err != nil {
		t.Fatalf("valid submission rejected: %v", err)
	}
	id, err := attestation.ID()
	if err != nil {
		t.Fatal(err)
	}
	if response.AttestationId != hex.EncodeToString(id[:]) {
		t.Fatalf("attestation ID = %s, want %s", response.AttestationId, hex.EncodeToString(id[:]))
	}
	if !k.IsProcessed(ctx, id) {
		t.Fatal("accepted attestation was not replay-protected")
	}
}

func validBridgeMessage(t *testing.T, routeID string, signatures [][]byte) types.MsgSubmitAttestation {
	t.Helper()
	return types.MsgSubmitAttestation{
		Submitter:     sdk.AccAddress(make([]byte, 20)).String(),
		RouteId:       routeID,
		Direction:     string(types.DirectionCronosToXitcoin),
		SourceChainId: "cronos-testnet",
		SourceRef:     "0x" + strings.Repeat("c", 64),
		Nonce:         1,
		Destination:   sdk.AccAddress(bytes.Repeat([]byte{1}, 20)).String(),
		Amount:        "10",
		DeadlineUnix:  1800003600,
		Signatures:    signatures,
	}
}
