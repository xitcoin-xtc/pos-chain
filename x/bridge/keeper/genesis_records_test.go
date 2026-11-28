package keeper

import (
	"encoding/json"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"

	"github.com/xitcoin-xtc/pos-chain/x/bridge/types"

	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
)

// rotate replaces the first bridge signer of the fixture's route with a new
// key through a signed update_route_config.
func (f *queryFixture) rotate(t *testing.T, nonce uint64) error {
	t.Helper()
	replacement, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	next := f.config
	next.BridgeSigners = []string{crypto.PubkeyToAddress(replacement.PublicKey).Hex(), f.config.BridgeSigners[1], f.config.BridgeSigners[2]}
	action, signatures := signedConfigActionWithNonce(t, f.config, next, f.keys[1:], nonce)
	if err := f.keeper.ApplyRouteConfigUpdate(f.ctx, f.config, action, next, signatures); err != nil {
		return err
	}
	f.config = next
	return nil
}

func requireExportImportable(t *testing.T, f queryFixture) types.GenesisState {
	t.Helper()
	exported := f.keeper.ExportGenesis(f.ctx)
	if err := exported.Validate(); err != nil {
		t.Fatalf("exported genesis is invalid: %v", err)
	}
	if err := exported.ValidateForChain(testChainID, f.ctx.BlockTime()); err != nil {
		t.Fatalf("exported genesis is invalid for this chain: %v", err)
	}
	return exported
}

func TestSignerRotationCannotWrapTheSignerSetVersion(t *testing.T) {
	f := newQueryFixture(t)
	f.keeper.setSignerSetVersion(f.ctx, math.MaxUint64-2)
	if err := f.rotate(t, 1); err != nil {
		t.Fatalf("rotation to the largest version rejected: %v", err)
	}
	if got := f.keeper.SignerSetVersion(f.ctx); got != math.MaxUint64-1 {
		t.Fatalf("version %d after rotation", got)
	}
	requireExportImportable(t, f)

	// One more rotation would reach 2^64-1 (and wrap to 0 after that): it is
	// rejected before anything is written.
	before := f.config
	if err := f.rotate(t, 2); !errors.Is(err, types.ErrSignerSetVersionExhausted) {
		t.Fatalf("rotation beyond the largest version: %v", err)
	}
	stored, _, err := f.keeper.GetRouteConfig(f.ctx)
	if err != nil || f.keeper.SignerSetVersion(f.ctx) != math.MaxUint64-1 || f.keeper.ControlNonce(f.ctx) != 1 ||
		stored.BridgeSigners[0] != before.BridgeSigners[0] {
		t.Fatalf("rejected rotation changed state: version %d, nonce %d, err %v", f.keeper.SignerSetVersion(f.ctx), f.keeper.ControlNonce(f.ctx), err)
	}
	requireExportImportable(t, f)

	// Updates that keep the signer set remain possible.
	next := f.config
	next.DailyLimit = "149"
	action, signatures := signedConfigActionWithNonce(t, f.config, next, f.keys[1:], 2)
	if err := f.keeper.ApplyRouteConfigUpdate(f.ctx, f.config, action, next, signatures); err != nil {
		t.Fatalf("limit update at the largest version rejected: %v", err)
	}
}

func TestExportAfterSignerRotationKeepsEarlierRecordsImportable(t *testing.T) {
	f := newQueryFixture(t)
	// Approved by the first signer, whom the rotation then removes.
	if _, err := f.msgs.SubmitAttestation(f.ctx.WithTxBytes([]byte("mint")), f.submission(t, 0xc1, "60", f.keys[0], f.keys[1])); err != nil {
		t.Fatal(err)
	}
	if err := f.rotate(t, 1); err != nil {
		t.Fatal(err)
	}
	exported := requireExportImportable(t, f)
	if exported.SignerSetVersion != 2 || exported.AttestationRecords[0].SignerSetVersion != 1 {
		t.Fatalf("unexpected versions: genesis %d, record %d", exported.SignerSetVersion, exported.AttestationRecords[0].SignerSetVersion)
	}
}

// A signed update may lower the daily limit below the usage of the day. The
// export keeps the exact usage (above the new limit) and stays importable;
// after a later same-day raise the imported chain allows exactly what the
// exporting chain would have.
func TestExportAfterLoweringTheDailyLimitKeepsTheExactUsage(t *testing.T) {
	f := newQueryFixture(t)
	f.settle(t) // 85 used of 150
	lowered := f.config
	lowered.DailyLimit = "50"
	action, signatures := signedConfigActionWithNonce(t, f.config, lowered, f.keys[:2], 1)
	if err := f.keeper.ApplyRouteConfigUpdate(f.ctx, f.config, action, lowered, signatures); err != nil {
		t.Fatal(err)
	}
	exported := requireExportImportable(t, f)
	if exported.DailyUsage == nil || exported.DailyUsage.Amount != "85" {
		t.Fatalf("exported daily usage = %+v", exported.DailyUsage)
	}

	encoded, err := json.Marshal(exported)
	if err != nil {
		t.Fatal(err)
	}
	var imported types.GenesisState
	if err := json.Unmarshal(encoded, &imported); err != nil {
		t.Fatal(err)
	}
	key := storetypes.NewKVStoreKey(types.StoreKey)
	ctx := testContext(key, storetypes.NewTransientStoreKey("bridge_lowered_limit_import")).WithBlockTime(time.Unix(queryBlockUnix+60, 0).UTC())
	k := NewKeeper(key)
	k.InitGenesis(ctx, imported)
	raised := lowered
	raised.DailyLimit, raised.MaxTransferAmount = "200", "200"
	if err := k.CheckAndRecordLimits(ctx, raised, "116"); err != ErrDailyLimitExceeded {
		t.Fatalf("import forgot part of the day's usage: %v", err)
	}
	if err := k.CheckAndRecordLimits(ctx, raised, "115"); err != nil {
		t.Fatalf("remaining capacity after the raise rejected: %v", err)
	}
}
