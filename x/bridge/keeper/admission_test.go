package keeper

import (
	"strings"
	"testing"
	"time"

	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	sdktestutil "github.com/cosmos/cosmos-sdk/testutil"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/xitcoin-xtc/pos-chain/x/bridge/types"
)

func TestAdmitAttestationRequiresMatchingEnabledRouteAndTwoSignatures(t *testing.T) {
	key := storetypes.NewKVStoreKey(types.StoreKey)
	ctx := sdktestutil.DefaultContext(key, storetypes.NewTransientStoreKey("bridge_admission_test")).WithBlockTime(time.Unix(1800000000, 0))
	k := NewKeeper(key)

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
		RouteID:           "cronos-testnet-xitcoin-testnet",
		BridgeSigners:     []string{crypto.PubkeyToAddress(first.PublicKey).Hex(), crypto.PubkeyToAddress(second.PublicKey).Hex(), crypto.PubkeyToAddress(third.PublicKey).Hex()},
		Guardian:          crypto.PubkeyToAddress(guardian.PublicKey).Hex(),
		MaxTransferAmount: "10", DailyLimit: "15",
		MaxOutstandingAmount: "1000000000000000000000000000", Enabled: true,
	}
	attestation := types.Attestation{
		RouteID: config.RouteID, Direction: types.DirectionCronosToXitcoin,
		SourceChainID: "cronos-testnet", SourceRef: "0x" + strings.Repeat("a", 64),
		Nonce: 1, Destination: "xitcoin1testdestination", Amount: "10", DeadlineUnix: 1800003600,
	}
	digest, err := types.SigningDigest(attestation)
	if err != nil {
		t.Fatal(err)
	}
	sigOne, err := crypto.Sign(digest.Bytes(), first)
	if err != nil {
		t.Fatal(err)
	}
	sigTwo, err := crypto.Sign(digest.Bytes(), second)
	if err != nil {
		t.Fatal(err)
	}

	id, err := k.AdmitAttestation(ctx, config, attestation, [][]byte{sigOne, sigTwo})
	if err != nil {
		t.Fatalf("valid attestation rejected: %v", err)
	}
	if !k.IsProcessed(ctx, id) {
		t.Fatal("accepted attestation was not replay-protected")
	}
	if got := k.DailyUsage(ctx, config.RouteID).String(); got != "10" {
		t.Fatalf("daily usage = %s, want 10", got)
	}
	if _, err := k.AdmitAttestation(ctx, config, attestation, [][]byte{sigOne, sigTwo}); err != ErrAttestationAlreadyProcessed {
		t.Fatalf("replay error = %v, want %v", err, ErrAttestationAlreadyProcessed)
	}
}

func TestAdmitAttestationRejectsDisabledOrMismatchedRouteWithoutRecording(t *testing.T) {
	key := storetypes.NewKVStoreKey(types.StoreKey)
	ctx := sdktestutil.DefaultContext(key, storetypes.NewTransientStoreKey("bridge_admission_reject_test")).WithBlockTime(time.Unix(1800000000, 0))
	k := NewKeeper(key)
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
	config := types.RouteConfig{RouteID: "cronos-testnet-xitcoin-testnet", BridgeSigners: []string{crypto.PubkeyToAddress(first.PublicKey).Hex(), crypto.PubkeyToAddress(second.PublicKey).Hex(), crypto.PubkeyToAddress(third.PublicKey).Hex()}, Guardian: crypto.PubkeyToAddress(guardian.PublicKey).Hex(), MaxTransferAmount: "10", DailyLimit: "15", MaxOutstandingAmount: "100"}
	attestation := types.Attestation{RouteID: config.RouteID, Direction: types.DirectionCronosToXitcoin, SourceChainID: "cronos-testnet", SourceRef: "0x" + strings.Repeat("b", 64), Nonce: 1, Destination: "xitcoin1testdestination", Amount: "1", DeadlineUnix: 1800003600}

	if _, err := k.AdmitAttestation(ctx, config, attestation, nil); err != ErrRouteDisabled {
		t.Fatalf("disabled route error = %v", err)
	}
	config.Enabled = true
	attestation.DeadlineUnix = ctx.BlockTime().Unix() - 1
	if _, err := k.AdmitAttestation(ctx, config, attestation, nil); err != types.ErrAttestationExpired {
		t.Fatalf("expired attestation error = %v", err)
	}
	attestation.DeadlineUnix = 1800003600
	attestation.RouteID = "other-testnet-route"
	if _, err := k.AdmitAttestation(ctx, config, attestation, nil); err != ErrAttestationRouteMismatch {
		t.Fatalf("route mismatch error = %v", err)
	}
	if got := k.DailyUsage(ctx, config.RouteID).Sign(); got != 0 {
		t.Fatalf("rejected input recorded daily usage: %d", got)
	}
}
