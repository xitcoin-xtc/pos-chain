package keeper

import (
	"bytes"
	"context"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	sdktestutil "github.com/cosmos/cosmos-sdk/testutil"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/xitcoin-xtc/pos-chain/x/bridge/types"
)

type fakeBankKeeper struct {
	supply sdkmath.Int
	minted sdk.Coins
	burned sdk.Coins
}

func (f *fakeBankKeeper) MintCoins(_ context.Context, _ string, amount sdk.Coins) error {
	f.minted = amount
	f.supply = f.supply.Add(amount.AmountOf("axtc"))
	return nil
}

func (f *fakeBankKeeper) BurnCoins(_ context.Context, _ string, amount sdk.Coins) error {
	f.burned = amount
	f.supply = f.supply.Sub(amount.AmountOf("axtc"))
	return nil
}

func (*fakeBankKeeper) SendCoinsFromModuleToAccount(context.Context, string, sdk.AccAddress, sdk.Coins) error {
	return nil
}

func (*fakeBankKeeper) SendCoinsFromAccountToModule(context.Context, sdk.AccAddress, string, sdk.Coins) error {
	return nil
}

func (f *fakeBankKeeper) GetSupply(_ context.Context, denom string) sdk.Coin {
	return sdk.NewCoin(denom, f.supply)
}

func settlementRouteConfig() types.RouteConfig {
	config := limitsConfig()
	config.MaxOutstandingAmount = "20"
	return config
}

func TestInboundSettlementAndOwnerBurn(t *testing.T) {
	key := storetypes.NewKVStoreKey(types.StoreKey)
	ctx := sdktestutil.DefaultContext(key, storetypes.NewTransientStoreKey("bridge_settlement_test")).
		WithBlockTime(time.Unix(1800000000, 0))
	bank := &fakeBankKeeper{supply: sdkmath.NewInt(100)}
	keeper, err := NewSettlementKeeper(key, bank, "axtc", "200", testAuthority())
	if err != nil {
		t.Fatal(err)
	}
	recipient := sdk.AccAddress(bytes.Repeat([]byte{1}, 20))
	attestation := types.Attestation{
		RouteID:       settlementRouteConfig().RouteID,
		Direction:     types.DirectionCronosToXitcoin,
		SourceChainID: "cronos-testnet",
		SourceRef:     "0x" + string(bytes.Repeat([]byte{'a'}, 64)),
		Nonce:         1,
		Destination:   recipient.String(),
		Amount:        "10",
		DeadlineUnix:  1800003600,
	}
	if err := keeper.SettleInbound(ctx, settlementRouteConfig(), attestation); err != nil {
		t.Fatalf("inbound settlement rejected: %v", err)
	}
	if got := keeper.OutstandingAmount(ctx).String(); got != "10" {
		t.Fatalf("outstanding amount = %s, want 10", got)
	}
	if bank.minted.AmountOf("axtc").String() != "10" {
		t.Fatal("native XTC was not minted")
	}
	requestID, nonce, err := keeper.InitiateOutboundTransfer(
		ctx,
		settlementRouteConfig(),
		recipient,
		"0x0000000000000000000000000000000000000001",
		"4",
	)
	if err != nil {
		t.Fatalf("owner burn rejected: %v", err)
	}
	if requestID == [32]byte{} || nonce != 1 {
		t.Fatal("outbound request identity was not generated")
	}
	if got := keeper.OutstandingAmount(ctx).String(); got != "6" {
		t.Fatalf("outstanding amount = %s, want 6", got)
	}
	if bank.burned.AmountOf("axtc").String() != "4" {
		t.Fatal("native XTC was not burned")
	}
}

func TestSettlementCaps(t *testing.T) {
	key := storetypes.NewKVStoreKey(types.StoreKey)
	ctx := sdktestutil.DefaultContext(key, storetypes.NewTransientStoreKey("bridge_settlement_caps_test"))
	bank := &fakeBankKeeper{supply: sdkmath.NewInt(195)}
	keeper, err := NewSettlementKeeper(key, bank, "axtc", "200", testAuthority())
	if err != nil {
		t.Fatal(err)
	}
	attestation := types.Attestation{
		Direction:   types.DirectionCronosToXitcoin,
		Destination: sdk.AccAddress(bytes.Repeat([]byte{2}, 20)).String(),
		Amount:      "10",
	}
	if err := keeper.SettleInbound(ctx, settlementRouteConfig(), attestation); err != ErrMaximumSupplyExceeded {
		t.Fatalf("maximum supply error = %v", err)
	}
	bank.supply = sdkmath.ZeroInt()
	keeper.setOutstandingAmount(ctx, sdkmath.NewInt(15))
	if err := keeper.SettleInbound(ctx, settlementRouteConfig(), attestation); err != ErrOutstandingLimitExceeded {
		t.Fatalf("outstanding limit error = %v", err)
	}
}
