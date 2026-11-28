package keeper

import (
	"bytes"
	"regexp"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"

	"github.com/xitcoin-xtc/pos-chain/x/bridge/types"

	sdkmath "cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// nonCanonicalAmounts are accepted by big.Int.SetString(s, 10) or by
// sdkmath.NewIntFromString (SetString(s, 0)) but are not the canonical
// decimal form of a positive integer. "010" is 10 in base 10 and 8 in base 0.
var nonCanonicalAmounts = []string{
	"010", "0010", "+10", "-10", "0", "00", " 10", "10 ", "\t10", "1_0",
	"0x10", "0o10", "0b10", "1e1", "10.0", "", "08", "+0",
}

// The relayer accepts only this form in bridge_outbound_burned and stops
// decoding the whole block on anything else (bridge-relayer src/network.js).
var relayerAmount = regexp.MustCompile(`^[1-9][0-9]*$`)

func TestNonCanonicalOutboundAmountIsRejectedBeforeBurn(t *testing.T) {
	for _, amount := range nonCanonicalAmounts {
		f := newSettlementFixture(t, testChainID)
		f.keeper.setOutstandingAmount(f.ctx, sdkmath.NewInt(50))
		sender := sdk.AccAddress(bytes.Repeat([]byte{3}, 20))
		msg := &types.MsgInitiateOutboundTransfer{
			Sender: sender.String(), RouteId: f.config.RouteID,
			Destination: "0x0000000000000000000000000000000000000001", Amount: amount,
		}
		if err := msg.ValidateBasic(); err == nil {
			t.Errorf("%q: ValidateBasic accepted a non-canonical amount", amount)
		}
		_, err := NewMsgServer(f.keeper).InitiateOutboundTransfer(f.ctx, msg)
		if err == nil {
			t.Errorf("%q: outbound transfer accepted (burned %s, daily usage %s, outstanding %s)",
				amount, f.bank.burned.AmountOf("axtc"), f.keeper.DailyUsage(f.ctx, f.config.RouteID), f.keeper.OutstandingAmount(f.ctx))
		}
		if f.bank.burned != nil || f.keeper.DailyUsage(f.ctx, f.config.RouteID).Sign() != 0 || f.keeper.OutstandingAmount(f.ctx).Int64() != 50 {
			t.Errorf("%q: rejected outbound transfer changed state", amount)
		}
		if _, _, err := f.keeper.InitiateOutboundTransfer(f.ctx, f.config, sender, msg.Destination, amount); err == nil {
			t.Errorf("%q: keeper burned a non-canonical amount", amount)
		}
	}
}

func TestNonCanonicalInboundAmountIsRejectedBeforeMint(t *testing.T) {
	for _, amount := range nonCanonicalAmounts {
		f := newSettlementFixture(t, testChainID)
		msg := validBridgeMessage(t, f.config.RouteID, nil)
		msg.Amount = amount
		// Sign exactly what is submitted whenever a digest exists, so only
		// the amount rule decides.
		if digest, err := types.SigningDigest(msg.Attestation(), testChainID); err == nil {
			for _, key := range f.keys[:2] {
				signature, err := crypto.Sign(digest.Bytes(), key)
				if err != nil {
					t.Fatal(err)
				}
				msg.Signatures = append(msg.Signatures, signature)
			}
		}
		_, err := NewMsgServer(f.keeper).SubmitAttestation(f.ctx, &msg)
		if err == nil {
			t.Errorf("%q: inbound attestation accepted (minted %s, daily usage %s)",
				amount, f.bank.minted.AmountOf("axtc"), f.keeper.DailyUsage(f.ctx, f.config.RouteID))
		}
		if f.bank.minted != nil || f.keeper.DailyUsage(f.ctx, f.config.RouteID).Sign() != 0 || len(f.keeper.ProcessedSourceDeposits(f.ctx)) != 0 {
			t.Errorf("%q: rejected attestation changed state", amount)
		}
	}
}

func TestOutboundEventCarriesTheBurnedAmountAndCanonicalDestination(t *testing.T) {
	f := newSettlementFixture(t, testChainID)
	f.keeper.setOutstandingAmount(f.ctx, sdkmath.NewInt(50))
	sender := sdk.AccAddress(bytes.Repeat([]byte{3}, 20))
	// Accepted by common.IsHexAddress, refused by ethers getAddress (bad
	// checksum, surrounding space or 0X prefix) if echoed verbatim.
	for _, destination := range []string{
		"0xAAAAaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		" 0x00000000000000000000000000000000000000aa",
		"0X00000000000000000000000000000000000000AA",
	} {
		ctx := f.ctx.WithEventManager(sdk.NewEventManager())
		msg := &types.MsgInitiateOutboundTransfer{Sender: sender.String(), RouteId: f.config.RouteID, Destination: destination, Amount: "7"}
		if _, err := NewMsgServer(f.keeper).InitiateOutboundTransfer(ctx, msg); err != nil {
			t.Fatalf("%q: valid outbound transfer rejected: %v", destination, err)
		}
		want, err := types.ValidateCronosDestination(destination)
		if err != nil || want.Hex() == destination {
			t.Fatalf("%q must be a valid, non-canonical destination (err %v)", destination, err)
		}
		event := findEvent(t, ctx.EventManager().Events(), "bridge_outbound_burned")
		if got := event["amount"]; got != f.bank.burned.AmountOf("axtc").String() || !relayerAmount.MatchString(got) {
			t.Fatalf("event amount %q, burned %s", got, f.bank.burned.AmountOf("axtc"))
		}
		if got := event["destination"]; got != want.Hex() {
			t.Fatalf("event destination %q, want canonical %s", got, want.Hex())
		}
	}
}

func findEvent(t *testing.T, events sdk.Events, eventType string) map[string]string {
	t.Helper()
	for _, event := range events {
		if event.Type != eventType {
			continue
		}
		attributes := make(map[string]string, len(event.Attributes))
		for _, attribute := range event.Attributes {
			attributes[attribute.Key] = attribute.Value
		}
		return attributes
	}
	t.Fatalf("event %s not emitted", eventType)
	return nil
}
