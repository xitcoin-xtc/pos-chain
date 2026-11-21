package types

import (
	"strings"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

func validControlSignatures() [][]byte {
	return [][]byte{make([]byte, 65), make([]byte, 65)}
}

func validSubmitMessage() MsgSubmitAttestation {
	return MsgSubmitAttestation{
		Submitter:     sdk.AccAddress(make([]byte, 20)).String(),
		RouteId:       "cronos-testnet-xitcoin-testnet",
		Direction:     string(DirectionCronosToXitcoin),
		SourceChainId: "338",
		SourceRef:     "0x" + strings.Repeat("a", 64),
		Nonce:         1,
		Destination:   "xitcoin1testdestination",
		Amount:        "1",
		DeadlineUnix:  1800003600,
	}
}

func TestMsgSubmitAttestationValidateBasic(t *testing.T) {
	msg := validSubmitMessage()
	if err := msg.ValidateBasic(); err != nil {
		t.Fatalf("valid submission rejected: %v", err)
	}
	msg.Submitter = "not-an-address"
	if err := msg.ValidateBasic(); err == nil {
		t.Fatal("invalid submitter accepted")
	}
}

func TestMsgInitiateOutboundTransferValidateBasic(t *testing.T) {
	msg := MsgInitiateOutboundTransfer{
		Sender:      sdk.AccAddress(make([]byte, 20)).String(),
		RouteId:     "cronos-testnet-xitcoin-testnet",
		Destination: "0x0000000000000000000000000000000000000001",
		Amount:      "1",
	}
	if err := msg.ValidateBasic(); err != nil {
		t.Fatalf("valid outbound transfer rejected: %v", err)
	}
	msg.Sender = "not-an-address"
	if err := msg.ValidateBasic(); err == nil {
		t.Fatal("invalid outbound sender accepted")
	}
}

func TestMsgInitializeRouteConfigValidateBasicForcesDisabled(t *testing.T) {
	msg := MsgInitializeRouteConfig{
		Authority: sdk.AccAddress(make([]byte, 20)).String(),
		RouteId:   "cronos-testnet-xitcoin-testnet",
		BridgeSigners: []string{
			"0x0000000000000000000000000000000000000001",
			"0x0000000000000000000000000000000000000002",
			"0x0000000000000000000000000000000000000003",
		},
		Guardian:          "0x0000000000000000000000000000000000000004",
		MaxTransferAmount: "10", DailyLimit: "100", MaxOutstandingAmount: "1000",
	}
	if err := msg.ValidateBasic(); err != nil {
		t.Fatalf("valid initial route rejected: %v", err)
	}
	if msg.RouteConfig().Enabled {
		t.Fatal("initial route message can enable its route")
	}
	msg.Authority = "not-an-address"
	if err := msg.ValidateBasic(); err == nil {
		t.Fatal("invalid authority accepted")
	}
}

func TestBridgeControlMessagesValidateBasic(t *testing.T) {
	submitter := sdk.AccAddress(make([]byte, 20)).String()
	pause := MsgEmergencyPauseRoute{Submitter: submitter, RouteId: "cronos-testnet-xitcoin-testnet", Nonce: 1, ExpiresUnix: 1800003600, GuardianSignature: make([]byte, 65)}
	if err := pause.ValidateBasic(); err != nil {
		t.Fatalf("valid emergency pause rejected: %v", err)
	}
	resume := MsgResumeRoute{Submitter: submitter, RouteId: pause.RouteId, Nonce: 2, NotBeforeUnix: 1800000000, ExpiresUnix: 1800003600, Signatures: validControlSignatures()}
	if err := resume.ValidateBasic(); err != nil {
		t.Fatalf("valid resume rejected: %v", err)
	}
	update := MsgUpdateRouteConfig{Submitter: submitter, RouteId: pause.RouteId, BridgeSigners: []string{"0x0000000000000000000000000000000000000001", "0x0000000000000000000000000000000000000002", "0x0000000000000000000000000000000000000003"}, Guardian: "0x0000000000000000000000000000000000000004", MaxTransferAmount: "10", DailyLimit: "100", MaxOutstandingAmount: "1000", Enabled: true, Nonce: 3, NotBeforeUnix: 1800000000, ExpiresUnix: 1800003600, Signatures: validControlSignatures()}
	if err := update.ValidateBasic(); err != nil {
		t.Fatalf("valid route update rejected: %v", err)
	}

	pause.GuardianSignature = pause.GuardianSignature[:64]
	if err := pause.ValidateBasic(); err == nil {
		t.Fatal("short guardian signature accepted")
	}
	resume.Signatures = resume.Signatures[:1]
	if err := resume.ValidateBasic(); err == nil {
		t.Fatal("one-signature resume accepted")
	}
	update.NotBeforeUnix = update.ExpiresUnix
	if err := update.ValidateBasic(); err == nil {
		t.Fatal("invalid control window accepted")
	}
}
