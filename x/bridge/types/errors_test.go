package types

import (
	"strings"
	"testing"

	errorsmod "cosmossdk.io/errors"

	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

// The codes are part of the consensus results and of the client contract
// (docs/bridge-architecture.md#error-codes): this table must only grow.
var registeredErrorCodes = []struct {
	err  *errorsmod.Error
	code uint32
}{
	{ErrEmptyRequest, 2},
	{ErrInvalidRouteID, 3},
	{ErrNonCanonicalAmount, 4},
	{ErrAmountOutOfRange, 5},
	{ErrInvalidAttestation, 6},
	{ErrInvalidCronosDestination, 7},
	{ErrInvalidXitcoinDestination, 8},
	{ErrInvalidChainID, 9},
	{ErrInvalidRouteConfig, 10},
	{ErrInvalidControlAction, 11},
	{ErrInvalidGuardianPause, 12},
	{ErrInvalidSignature, 13},
	{ErrInvalidApprovals, 14},
	{ErrRouteDisabled, 15},
	{ErrRoutePaused, 16},
	{ErrRouteAlreadyPaused, 17},
	{ErrRouteNotPaused, 18},
	{ErrRouteAlreadyInitialized, 19},
	{ErrAttestationRouteMismatch, 20},
	{ErrAttestationSourceChainMismatch, 21},
	{ErrAttestationAlreadyProcessed, 22},
	{ErrAttestationExpired, 23},
	{ErrSettlementUnavailable, 24},
	{ErrSettlementDirection, 25},
	{ErrBlockedDestination, 26},
	{ErrTransferLimitExceeded, 27},
	{ErrDailyLimitExceeded, 28},
	{ErrOutstandingLimitExceeded, 29},
	{ErrInsufficientOutstanding, 30},
	{ErrMaximumSupplyExceeded, 31},
	{ErrCapBelowOutstanding, 32},
	{ErrControlActionNotActive, 33},
	{ErrControlActionProcessed, 34},
	{ErrControlPayloadMismatch, 35},
	{ErrControlNonceGap, 36},
	{ErrGuardianPauseRouteMismatch, 37},
	{ErrGuardianPauseExpired, 38},
	{ErrSignerSetVersionExhausted, 39},
}

func TestRegisteredErrorCodesAreStable(t *testing.T) {
	seen := map[uint32]bool{}
	for i, entry := range registeredErrorCodes {
		if entry.err.Codespace() != ModuleName || entry.err.Codespace() != "bridge" {
			t.Fatalf("%v: codespace %q", entry.err, entry.err.Codespace())
		}
		if entry.err.ABCICode() != entry.code {
			t.Fatalf("%v: code %d, want %d", entry.err, entry.err.ABCICode(), entry.code)
		}
		// Contiguous from 2: a gap or a reused code is a renumbering.
		if entry.code != uint32(i+2) || seen[entry.code] {
			t.Fatalf("%v: code %d out of sequence", entry.err, entry.code)
		}
		seen[entry.code] = true
	}
}

// abci returns what a transaction result reports for err, after the wrapping
// the SDK applies to a failing message.
func abci(err error) (string, uint32, string) {
	return errorsmod.ABCIInfo(errorsmod.Wrapf(err, "failed to execute message; message index: %d", 0), false)
}

// twoTo256 is 2^256, one more than the largest bridge amount.
const twoTo256 = "115792089237316195423570985008687907853269984665640564039457584007913129639936"

func TestValidationErrorsReportBridgeCodes(t *testing.T) {
	valid := Attestation{
		RouteID: "route", Direction: DirectionCronosToXitcoin, SourceChainID: "338",
		SourceRef: "0x" + strings.Repeat("ab", 32), Nonce: 1, Destination: "xtc1destination", Amount: "1", DeadlineUnix: 100,
	}
	mutate := func(change func(*Attestation)) Attestation {
		a := valid
		change(&a)
		return a
	}
	testCases := []struct {
		name string
		err  error
		want *errorsmod.Error
	}{
		{"route ID", mutate(func(a *Attestation) { a.RouteID = "Route" }).Validate(), ErrInvalidRouteID},
		{"direction", mutate(func(a *Attestation) { a.Direction = "sideways" }).Validate(), ErrInvalidAttestation},
		{"source chain", mutate(func(a *Attestation) { a.SourceChainID = "0338" }).Validate(), ErrInvalidChainID},
		{"source reference", mutate(func(a *Attestation) { a.SourceRef = "0x12" }).Validate(), ErrInvalidAttestation},
		{"nonce", mutate(func(a *Attestation) { a.Nonce = 0 }).Validate(), ErrInvalidAttestation},
		{"amount", mutate(func(a *Attestation) { a.Amount = "010" }).Validate(), ErrNonCanonicalAmount},
		{"amount range", mutate(func(a *Attestation) { a.Amount = twoTo256 }).Validate(), ErrAmountOutOfRange},
		{"deadline", mutate(func(a *Attestation) { a.DeadlineUnix = 0 }).Validate(), ErrInvalidAttestation},
		{"expired", valid.ValidateAt(101), ErrAttestationExpired},
		{"outbound destination", (&MsgInitiateOutboundTransfer{
			Sender: "xtc1invalid", RouteId: "route", Destination: "0x0000000000000000000000000000000000000000", Amount: "1",
		}).ValidateBasic(), nil},
		{"empty message", (*MsgSubmitAttestation)(nil).ValidateBasic(), ErrEmptyRequest},
		{"route config", RouteConfig{RouteID: "route"}.Validate(), ErrInvalidRouteConfig},
		{"control action", ControlAction{RouteID: "route", Action: "mint"}.Validate(), ErrInvalidControlAction},
		{"guardian pause", GuardianPauseAction{RouteID: "route"}.Validate(), ErrInvalidGuardianPause},
		{"control signatures", validateControlSignatures([][]byte{make([]byte, 65), make([]byte, 64)}), ErrInvalidSignature},
		{"control approvals", validateControlSignatures([][]byte{make([]byte, 65)}), ErrInvalidApprovals},
		{"approval signature", func() error {
			_, err := recoverSigner([32]byte{}, make([]byte, 64))
			return err
		}(), ErrInvalidSignature},
		{"signer set", func() error {
			_, err := configuredSigners([]string{"0x1"})
			return err
		}(), ErrInvalidRouteConfig},
		{"signer set version", func() error {
			_, err := NextSignerSetVersion(MaxSignerSetVersion)
			return err
		}(), ErrSignerSetVersionExhausted},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.err == nil {
				t.Fatal("expected an error")
			}
			codespace, code, log := abci(tc.err)
			if tc.want == nil {
				// An address that does not parse is the SDK's invalid address.
				if codespace != sdkerrors.ErrInvalidAddress.Codespace() || code != sdkerrors.ErrInvalidAddress.ABCICode() {
					t.Fatalf("got %s/%d (%s)", codespace, code, log)
				}
				return
			}
			if codespace != "bridge" || code != tc.want.ABCICode() {
				t.Fatalf("got %s/%d (%s), want bridge/%d", codespace, code, log, tc.want.ABCICode())
			}
			if !errorsmod.IsOf(tc.err, tc.want) {
				t.Fatalf("%v is not %v", tc.err, tc.want)
			}
		})
	}
}

func TestDestinationErrorsReportBridgeCodes(t *testing.T) {
	for _, destination := range []string{"0x0000000000000000000000000000000000000000", CronosDeadAddress.Hex(), "not an address"} {
		_, err := ValidateCronosDestination(destination)
		if codespace, code, _ := abci(err); codespace != "bridge" || code != ErrInvalidCronosDestination.ABCICode() {
			t.Fatalf("%s: got %s/%d", destination, codespace, code)
		}
	}
}
