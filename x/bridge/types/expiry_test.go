package types

import (
	"strings"
	"testing"
)

func TestValidateAtRejectsExpiredAttestation(t *testing.T) {
	a := Attestation{
		RouteID:       "cronos-testnet-xitcoin-testnet",
		Direction:     DirectionCronosToXitcoin,
		SourceChainID: "338",
		SourceRef:     "0x" + strings.Repeat("a", 64),
		Nonce:         1,
		Destination:   "xitcoin1testdestination",
		Amount:        "1",
		DeadlineUnix:  100,
	}
	if err := a.ValidateAt(100); err != nil {
		t.Fatalf("attestation at deadline rejected: %v", err)
	}
	if err := a.ValidateAt(101); err != ErrAttestationExpired {
		t.Fatalf("expiry error = %v, want %v", err, ErrAttestationExpired)
	}
}
