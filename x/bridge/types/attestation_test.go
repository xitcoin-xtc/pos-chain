package types

import (
	"encoding/hex"
	"strings"
	"testing"
)

const testChainID = "xitcoin-testnet-v2-1"

func validAttestation() Attestation {
	return Attestation{
		RouteID:       "cronos-testnet-xitcoin-testnet",
		Direction:     DirectionCronosToXitcoin,
		SourceChainID: "338",
		SourceRef:     "0x" + strings.Repeat("a", 64),
		Nonce:         1,
		Destination:   "xitcoin1testdestination",
		Amount:        "1000000000000000000",
		DeadlineUnix:  1800000000,
	}
}

func TestAttestationIDIsDeterministic(t *testing.T) {
	a := validAttestation()
	first, err := a.ID()
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	second, err := a.ID()
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if first != second {
		t.Fatal("attestation ID must be deterministic")
	}
	if hex.EncodeToString(first[:]) == strings.Repeat("0", 64) {
		t.Fatal("attestation ID must not be empty")
	}
}

func TestAttestationRejectsReplayUnsafeInput(t *testing.T) {
	a := validAttestation()
	a.Nonce = 0
	if err := a.Validate(); err == nil {
		t.Fatal("zero nonce must be rejected")
	}
	a = validAttestation()
	a.SourceRef = "0x1234"
	if err := a.Validate(); err == nil {
		t.Fatal("short source reference must be rejected")
	}
	a = validAttestation()
	a.Amount = "0"
	if err := a.Validate(); err == nil {
		t.Fatal("zero amount must be rejected")
	}
}

func TestApprovalSetRequiresDistinctTwoOfThree(t *testing.T) {
	if err := ValidateApprovalSet([]string{"signer-one"}); err == nil {
		t.Fatal("one approval must be rejected")
	}
	if err := ValidateApprovalSet([]string{"signer-one", "SIGNER-ONE"}); err == nil {
		t.Fatal("duplicate approval must be rejected")
	}
	if err := ValidateApprovalSet([]string{"signer-one", "signer-two"}); err != nil {
		t.Fatalf("two distinct approvals must be accepted: %v", err)
	}
	if err := ValidateApprovalSet([]string{"one", "two", "three", "four"}); err == nil {
		t.Fatal("more than three approvals must be rejected")
	}
}
