package types

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	sdkmath "cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

const recordRouteID = "0x21121c16b53a726056a6683f00c7eb4da5501ce8a2abc8a4677e06f1e94b5cd9"

func recordAttestation() Attestation {
	return Attestation{
		RouteID: recordRouteID, Direction: DirectionCronosToXitcoin, SourceChainID: "338",
		SourceRef: "0X" + strings.Repeat("AB", 32), Nonce: 7,
		Destination: sdk.AccAddress(bytes.Repeat([]byte{1}, 20)).String(), Amount: "1000", DeadlineUnix: 1800003600,
	}
}

func validAttestationRecord(t *testing.T) AttestationRecord {
	t.Helper()
	approvers := []common.Address{common.HexToAddress("0x00000000000000000000000000000000000000a1"), common.HexToAddress("0x00000000000000000000000000000000000000a2")}
	record, err := NewAttestationRecord(recordAttestation(), "xitcoin-testnet-v2-1", 1, approvers, 42, TxHash([]byte("tx")))
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func validOutboundTransfer() OutboundTransferRecord {
	sender := sdk.AccAddress(bytes.Repeat([]byte{2}, 20)).String()
	destination := common.HexToAddress("0x00000000000000000000000000000000000000c5")
	id := OutboundRequestID(recordRouteID, sender, destination, sdkmath.NewInt(25), 1)
	return OutboundTransferRecord{
		RequestId: HexID(id), RouteId: recordRouteID, Sender: sender, Destination: destination.Hex(),
		Amount: "25", Nonce: 1, Height: 43, TxHash: TxHash([]byte("burn")),
	}
}

func TestTxHashIsTheCometBFTHash(t *testing.T) {
	if TxHash(nil) != "" {
		t.Fatal("no transaction bytes must give no hash")
	}
	if got := TxHash([]byte("abc")); got != "BA7816BF8F01CFEA414140DE5DAE2223B00361A396177A9CB410FF61F20015AD" {
		t.Fatalf("TxHash = %s", got)
	}
}

func TestAttestationRecordBindsTheIDToEveryApprovedField(t *testing.T) {
	record := validAttestationRecord(t)
	if err := record.Validate(); err != nil {
		t.Fatalf("valid record rejected: %v", err)
	}
	if record.SourceRef != strings.Repeat("ab", 32) {
		t.Fatalf("source reference not canonical: %s", record.SourceRef)
	}
	id, err := recordAttestation().ID()
	if err != nil || record.AttestationId != HexID(id) {
		t.Fatalf("record ID %s, want %s (err %v)", record.AttestationId, HexID(id), err)
	}
	for name, mutate := range map[string]func(*AttestationRecord){
		"amount":            func(r *AttestationRecord) { r.Amount = "1001" },
		"recipient":         func(r *AttestationRecord) { r.Recipient = sdk.AccAddress(bytes.Repeat([]byte{3}, 20)).String() },
		"deadline":          func(r *AttestationRecord) { r.DeadlineUnix++ },
		"nonce":             func(r *AttestationRecord) { r.Nonce++ },
		"route":             func(r *AttestationRecord) { r.RouteId = "cronos-testnet-xitcoin-testnet" },
		"destination chain": func(r *AttestationRecord) { r.DestinationChainId = "xitcoin" },
		"source reference":  func(r *AttestationRecord) { r.SourceRef = strings.Repeat("cd", 32) },
		"spelled reference": func(r *AttestationRecord) { r.SourceRef = "0x" + strings.Repeat("ab", 32) },
		"source chain":      func(r *AttestationRecord) { r.SourceChainId = "25" },
		"ID spelling":       func(r *AttestationRecord) { r.AttestationId = strings.ToUpper(r.AttestationId) },
		"payload hash":      func(r *AttestationRecord) { r.PayloadHash = HexID([32]byte{1}) },
		"signing digest":    func(r *AttestationRecord) { r.SigningDigest = HexID([32]byte{1}) },
		"direction":         func(r *AttestationRecord) { r.Direction = string(DirectionXitcoinToCronos) },
		"version":           func(r *AttestationRecord) { r.SignerSetVersion = 0 },
		"one approver":      func(r *AttestationRecord) { r.Approvers = r.Approvers[:1] },
		"duplicate":         func(r *AttestationRecord) { r.Approvers[1] = r.Approvers[0] },
		"lowercase":         func(r *AttestationRecord) { r.Approvers[0] = "0x00000000000000000000000000000000000000Aa" },
		"tx hash":           func(r *AttestationRecord) { r.TxHash = strings.ToLower(r.TxHash) },
		"height":            func(r *AttestationRecord) { r.Height = -1 },
	} {
		tampered := validAttestationRecord(t)
		mutate(&tampered)
		if err := tampered.Validate(); err == nil {
			t.Fatalf("record with a changed %s accepted", name)
		}
	}
}

func TestOutboundTransferRecordBindsTheRequestID(t *testing.T) {
	record := validOutboundTransfer()
	if err := record.Validate(); err != nil {
		t.Fatalf("valid transfer rejected: %v", err)
	}
	for name, mutate := range map[string]func(*OutboundTransferRecord){
		"amount":      func(r *OutboundTransferRecord) { r.Amount = "26" },
		"nonce":       func(r *OutboundTransferRecord) { r.Nonce = 2 },
		"sender":      func(r *OutboundTransferRecord) { r.Sender = sdk.AccAddress(bytes.Repeat([]byte{3}, 20)).String() },
		"destination": func(r *OutboundTransferRecord) { r.Destination = "0x00000000000000000000000000000000000000C6" },
		"spelling":    func(r *OutboundTransferRecord) { r.Destination = strings.ToLower(r.Destination) + " " },
		"dead":        func(r *OutboundTransferRecord) { r.Destination = CronosDeadAddress.Hex() },
		"route":       func(r *OutboundTransferRecord) { r.RouteId = "Route" },
		"ID":          func(r *OutboundTransferRecord) { r.RequestId = strings.TrimPrefix(r.RequestId, "0x") },
		"amount form": func(r *OutboundTransferRecord) { r.Amount = "025" },
		"zero nonce":  func(r *OutboundTransferRecord) { r.Nonce = 0 },
		"tx hash":     func(r *OutboundTransferRecord) { r.TxHash = "0x" + r.TxHash[2:] },
	} {
		tampered := validOutboundTransfer()
		mutate(&tampered)
		if err := tampered.Validate(); err == nil {
			t.Fatalf("transfer with a changed %s accepted", name)
		}
	}
}

func TestSameSignerSetIgnoresOrderAndSpelling(t *testing.T) {
	a := []string{"0x00000000000000000000000000000000000000A1", "0x00000000000000000000000000000000000000a2", "0x00000000000000000000000000000000000000A3"}
	if !SameSignerSet(a, []string{a[2], strings.ToLower(a[0]), a[1]}) {
		t.Fatal("reordered signer set reported as changed")
	}
	if SameSignerSet(a, []string{a[0], a[1], "0x00000000000000000000000000000000000000A4"}) {
		t.Fatal("rotated signer set reported as unchanged")
	}
	if SameSignerSet(a, a[:2]) {
		t.Fatal("invalid signer set reported as unchanged")
	}
}

func recordGenesis(t *testing.T) GenesisState {
	t.Helper()
	record := validAttestationRecord(t)
	return GenesisState{
		RouteConfig: &RouteConfig{
			RouteID: recordRouteID,
			BridgeSigners: []string{
				"0x00000000000000000000000000000000000000A1", "0x00000000000000000000000000000000000000a2", "0x00000000000000000000000000000000000000A3",
			},
			Guardian: "0x00000000000000000000000000000000000000B4", MaxTransferAmount: "100", DailyLimit: "150", MaxOutstandingAmount: "1000",
		},
		OutstandingAmount:       "975",
		OutboundNonce:           1,
		ProcessedSourceDeposits: []string{record.AttestationId},
		SignerSetVersion:        2,
		DailyUsage:              &DailyUsage{UTCDay: 20833, Amount: "1025"},
		AttestationRecords:      []AttestationRecord{record},
		OutboundTransfers:       []OutboundTransferRecord{validOutboundTransfer()},
	}
}

func TestGenesisAcceptsRecordsAndDailyUsage(t *testing.T) {
	// The usage may exceed a daily limit lowered after the transfers.
	if err := recordGenesis(t).Validate(); err != nil {
		t.Fatalf("valid genesis rejected: %v", err)
	}
}

func TestGenesisRejectsInconsistentRecords(t *testing.T) {
	for name, mutate := range map[string]func(*GenesisState){
		"record without processed key": func(g *GenesisState) { g.ProcessedSourceDeposits = nil },
		"duplicate record":             func(g *GenesisState) { g.AttestationRecords = append(g.AttestationRecords, g.AttestationRecords[0]) },
		"tampered record":              func(g *GenesisState) { g.AttestationRecords[0].Amount = "999" },
		"record of another route":      func(g *GenesisState) { g.RouteConfig.RouteID = "cronos-testnet-xitcoin-testnet" },
		"transfer beyond nonce":        func(g *GenesisState) { g.OutboundNonce = 0 },
		"duplicate transfer":           func(g *GenesisState) { g.OutboundTransfers = append(g.OutboundTransfers, g.OutboundTransfers[0]) },
		"tampered transfer":            func(g *GenesisState) { g.OutboundTransfers[0].Amount = "24" },
		"zero daily usage":             func(g *GenesisState) { g.DailyUsage.Amount = "0" },
		"non-canonical daily usage":    func(g *GenesisState) { g.DailyUsage.Amount = "0100" },
	} {
		genesis := recordGenesis(t)
		mutate(&genesis)
		if err := genesis.Validate(); err == nil {
			t.Fatalf("genesis with %s accepted", name)
		}
	}
	for name, genesis := range map[string]GenesisState{
		"version":     {SignerSetVersion: 1},
		"daily usage": {DailyUsage: &DailyUsage{UTCDay: 1, Amount: "1"}},
		"record":      {AttestationRecords: []AttestationRecord{validAttestationRecord(t)}},
		"transfer":    {OutboundTransfers: []OutboundTransferRecord{validOutboundTransfer()}},
	} {
		if err := genesis.Validate(); err == nil {
			t.Fatalf("genesis with a %s but no route accepted", name)
		}
	}
}
