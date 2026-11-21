package keeper

import (
	"strings"
	"testing"

	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	sdktestutil "github.com/cosmos/cosmos-sdk/testutil"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/xitcoin-xtc/pos-chain/x/bridge/types"
)

const testChainID = "xitcoin-testnet-v2-1"

func testAuthority() string { return sdk.AccAddress(make([]byte, 20)).String() }

// testContext returns a context on the canonical testnet chain ID, which the
// network-specific signing domains require.
func testContext(key, transientKey storetypes.StoreKey) sdk.Context {
	return sdktestutil.DefaultContext(key, transientKey).WithChainID(testChainID)
}

func testAttestation() types.Attestation {
	return types.Attestation{
		RouteID:       "cronos-testnet-xitcoin-testnet",
		Direction:     types.DirectionCronosToXitcoin,
		SourceChainID: "338",
		SourceRef:     "0x" + strings.Repeat("a", 64),
		Nonce:         1,
		Destination:   "xitcoin1testdestination",
		Amount:        "1000000000000000000",
		DeadlineUnix:  1800000000,
	}
}

func TestConsumeAttestationRejectsReplay(t *testing.T) {
	key := storetypes.NewKVStoreKey(types.StoreKey)
	ctx := testContext(key, storetypes.NewTransientStoreKey("bridge_replay_test"))
	k := NewKeeper(key)

	id, err := k.ConsumeAttestation(ctx, testAttestation())
	if err != nil {
		t.Fatalf("first valid attestation rejected: %v", err)
	}
	if !k.IsProcessed(ctx, id) {
		t.Fatal("processed attestation was not stored")
	}
	if _, err := k.ConsumeAttestation(ctx, testAttestation()); err != ErrAttestationAlreadyProcessed {
		t.Fatalf("replay error = %v, want %v", err, ErrAttestationAlreadyProcessed)
	}
}

func TestConsumeAttestationDoesNotStoreInvalidInput(t *testing.T) {
	key := storetypes.NewKVStoreKey(types.StoreKey)
	ctx := testContext(key, storetypes.NewTransientStoreKey("bridge_invalid_test"))
	k := NewKeeper(key)

	invalid := testAttestation()
	invalid.Nonce = 0
	if _, err := k.ConsumeAttestation(ctx, invalid); err == nil {
		t.Fatal("invalid attestation was accepted")
	}
}
