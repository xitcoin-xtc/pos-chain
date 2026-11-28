package bridge_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/xitcoin-xtc/pos-chain/x/bridge"
	"github.com/xitcoin-xtc/pos-chain/x/bridge/keeper"
	"github.com/xitcoin-xtc/pos-chain/x/bridge/types"

	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	sdktestutil "github.com/cosmos/cosmos-sdk/testutil"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// exportedGenesis is a genesis exported from xitcoin-testnet-v2-1 on UTC day
// 20833 (2027-01-15) with one attestation record.
func exportedGenesis(t *testing.T) types.GenesisState {
	t.Helper()
	attestation := types.Attestation{
		RouteID: liveRouteID, Direction: types.DirectionCronosToXitcoin, SourceChainID: "338",
		SourceRef: strings.Repeat("ab", 32), Nonce: 1, Destination: sdk.AccAddress(bytes.Repeat([]byte{1}, 20)).String(),
		Amount: "60", DeadlineUnix: 1800003600,
	}
	signers := []string{"0x00000000000000000000000000000000000000A1", "0x00000000000000000000000000000000000000a2", "0x00000000000000000000000000000000000000A3"}
	record, err := types.NewAttestationRecord(attestation, "xitcoin-testnet-v2-1", 1,
		[]common.Address{common.HexToAddress(signers[0]), common.HexToAddress(signers[1])}, 5, types.TxHash([]byte("mint")))
	if err != nil {
		t.Fatal(err)
	}
	return types.GenesisState{
		RouteConfig: &types.RouteConfig{
			RouteID: liveRouteID, BridgeSigners: signers, Guardian: "0x00000000000000000000000000000000000000B4",
			MaxTransferAmount: "100", DailyLimit: "150", MaxOutstandingAmount: "1000", Enabled: true, SourceChainID: "338",
		},
		OutstandingAmount:       "60",
		ProcessedSourceDeposits: []string{record.AttestationId},
		SignerSetVersion:        1,
		DailyUsage:              &types.DailyUsage{UTCDay: 20833, Amount: "60"},
		AttestationRecords:      []types.AttestationRecord{record},
	}
}

func initGenesis(t *testing.T, chainID string, genesisTime time.Time, state types.GenesisState) (panicked any) {
	t.Helper()
	key := storetypes.NewKVStoreKey(types.StoreKey)
	ctx := sdktestutil.DefaultContext(key, storetypes.NewTransientStoreKey("bridge_module_genesis_test")).
		WithChainID(chainID).WithBlockTime(genesisTime)
	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { panicked = recover() }()
	bridge.NewAppModule(keeper.NewKeeper(key)).InitGenesis(ctx, nil, encoded)
	return nil
}

func TestModuleInitGenesisBindsRecordsAndDailyUsageToTheChain(t *testing.T) {
	sameDay := time.Date(2027, 1, 15, 23, 0, 0, 0, time.UTC)
	if p := initGenesis(t, "xitcoin-testnet-v2-1", sameDay, exportedGenesis(t)); p != nil {
		t.Fatalf("genesis of this chain rejected: %v", p)
	}
	if p := initGenesis(t, "xitcoin-testnet-v2-1", sameDay.Add(48*time.Hour), exportedGenesis(t)); p != nil {
		t.Fatalf("genesis imported on a later day rejected: %v", p)
	}
	if p := initGenesis(t, "xitcoin-testnet-v3-1", sameDay, exportedGenesis(t)); p == nil {
		t.Fatal("record processed on another chain imported")
	}
	if p := initGenesis(t, "xitcoin-testnet-v2-1", sameDay.Add(-24*time.Hour), exportedGenesis(t)); p == nil {
		t.Fatal("daily usage of a UTC day after the genesis time imported")
	}
	legacy := exportedGenesis(t)
	legacy.RouteConfig.SourceChainID = ""
	if p := initGenesis(t, "xitcoin", sameDay, legacy); p == nil {
		t.Fatal("record processed on xitcoin-testnet-v2-1 from Cronos Testnet imported on mainnet")
	}
}
