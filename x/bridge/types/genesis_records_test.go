package types

import (
	"math"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

const recordChainID = "xitcoin-testnet-v2-1"

// recordWith builds a valid record of recordAttestation on recordChainID.
func recordWith(t *testing.T, sourceChainID string, version uint64, approvers ...string) AttestationRecord {
	t.Helper()
	attestation := recordAttestation()
	attestation.SourceChainID = sourceChainID
	addresses := make([]common.Address, len(approvers))
	for i, approver := range approvers {
		addresses[i] = common.HexToAddress(approver)
	}
	record, err := NewAttestationRecord(attestation, recordChainID, version, addresses, 42, TxHash([]byte("tx")))
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func genesisWith(record AttestationRecord, version uint64) GenesisState {
	return GenesisState{
		RouteConfig: &RouteConfig{
			RouteID: recordRouteID,
			BridgeSigners: []string{
				"0x00000000000000000000000000000000000000A1", "0x00000000000000000000000000000000000000a2", "0x00000000000000000000000000000000000000A3",
			},
			Guardian: "0x00000000000000000000000000000000000000B4", MaxTransferAmount: "100", DailyLimit: "150",
			MaxOutstandingAmount: "1000000", SourceChainID: "338",
		},
		ProcessedSourceDeposits: []string{record.AttestationId},
		SignerSetVersion:        version,
		AttestationRecords:      []AttestationRecord{record},
	}
}

func TestGenesisRejectsASignerSetVersionThatCannotRotate(t *testing.T) {
	record := recordWith(t, "338", 1, "0x00000000000000000000000000000000000000a1", "0x00000000000000000000000000000000000000a2")
	if err := genesisWith(record, math.MaxUint64).Validate(); err == nil {
		t.Fatal("signer set version 2^64-1 accepted: one rotation would wrap it to 0")
	}
	if err := genesisWith(record, math.MaxUint64-1).Validate(); err != nil {
		t.Fatalf("largest signer set version rejected: %v", err)
	}
	if next, err := NextSignerSetVersion(math.MaxUint64 - 2); err != nil || next != math.MaxUint64-1 {
		t.Fatalf("next version = %d, %v", next, err)
	}
	for _, current := range []uint64{math.MaxUint64 - 1, math.MaxUint64} {
		if _, err := NextSignerSetVersion(current); err != ErrSignerSetVersionExhausted {
			t.Fatalf("next version after %d: %v", current, err)
		}
	}
}

func TestGenesisRejectsRecordsInconsistentWithTheRoute(t *testing.T) {
	signer1, signer2 := "0x00000000000000000000000000000000000000a1", "0x00000000000000000000000000000000000000a2"
	outsider := "0x00000000000000000000000000000000000000a4"

	if err := genesisWith(recordWith(t, "338", 1, signer1, outsider), 1).Validate(); err == nil {
		t.Fatal("record of the current signer set approved by an unconfigured signer accepted")
	}
	if err := genesisWith(recordWith(t, "338", 1, signer1, signer2), 0).Validate(); err != nil {
		t.Fatalf("record of the default signer set version rejected: %v", err)
	}
	if err := genesisWith(recordWith(t, "338", 3, signer1, signer2), 2).Validate(); err == nil {
		t.Fatal("record of a signer set version later than the genesis version accepted")
	}
	if err := genesisWith(recordWith(t, "25", 1, signer1, signer2), 1).Validate(); err == nil {
		t.Fatal("record from another source chain than the route accepted")
	}
	// A record of an earlier signer set was verified against signers that a
	// later rotation removed: it must stay importable.
	if err := genesisWith(recordWith(t, "338", 1, signer1, outsider), 2).Validate(); err != nil {
		t.Fatalf("record of an earlier signer set rejected: %v", err)
	}
}

func TestValidateForChainBindsRecordsAndDailyUsageToTheImportingChain(t *testing.T) {
	record := recordWith(t, "338", 1, "0x00000000000000000000000000000000000000a1", "0x00000000000000000000000000000000000000a2")
	genesisTime := time.Date(2027, 1, 15, 9, 0, 0, 0, time.UTC) // UTC day 20833
	genesis := genesisWith(record, 1)
	genesis.DailyUsage = &DailyUsage{UTCDay: 20833, Amount: "85"}
	if err := genesis.ValidateForChain(recordChainID, genesisTime); err != nil {
		t.Fatalf("genesis of this chain rejected: %v", err)
	}
	if err := genesis.ValidateForChain("xitcoin", genesisTime); err == nil {
		t.Fatal("record processed on another chain accepted")
	}
	genesis.DailyUsage.UTCDay = 20834
	if err := genesis.ValidateForChain(recordChainID, genesisTime); err == nil {
		t.Fatal("daily usage of a later UTC day than the genesis time accepted")
	}
	genesis.DailyUsage.UTCDay = 20000
	if err := genesis.ValidateForChain(recordChainID, genesisTime); err != nil {
		t.Fatalf("daily usage of an earlier UTC day rejected: %v", err)
	}

	// A route stored without source_chain_id uses the chain's default
	// (25 on xitcoin mainnet).
	legacy := genesisWith(record, 1)
	legacy.RouteConfig.SourceChainID = ""
	if err := legacy.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := legacy.ValidateForChain(recordChainID, genesisTime); err != nil {
		t.Fatalf("record matching the default source chain rejected: %v", err)
	}
	mainnet := recordWith(t, "338", 1, "0x00000000000000000000000000000000000000a1", "0x00000000000000000000000000000000000000a2")
	mainnet.DestinationChainId = "xitcoin"
	if err := (GenesisState{RouteConfig: legacy.RouteConfig, AttestationRecords: []AttestationRecord{mainnet}}).ValidateForChain("xitcoin", genesisTime); err == nil {
		t.Fatal("record from Cronos Testnet accepted for a mainnet route")
	}

	if err := (GenesisState{}).ValidateForChain(recordChainID, time.Time{}); err != nil {
		t.Fatalf("empty genesis rejected: %v", err)
	}
}

func TestUTCDayNeverWraps(t *testing.T) {
	if UTCDay(-1) != 0 || UTCDay(86399) != 0 || UTCDay(86400) != 1 {
		t.Fatal("unexpected UTC day")
	}
	if UTCDay(time.Date(2026, 11, 28, 23, 59, 59, 0, time.UTC).Unix()) != 20785 {
		t.Fatal("unexpected UTC day of 2026-11-28")
	}
}
