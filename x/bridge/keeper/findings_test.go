package keeper

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/xitcoin-xtc/pos-chain/x/bridge/types"

	sdkmath "cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// A 2-of-3 update could set max_outstanding_amount below the outstanding
// amount, leaving outstanding > cap and a state whose exported genesis fails
// validation.
func TestConfigUpdateCannotSetCapBelowOutstanding(t *testing.T) {
	ctx, k, current, keys, _ := controlServerSetup(t)
	k.setOutstandingAmount(ctx, sdkmath.NewInt(600))
	next := current
	next.MaxOutstandingAmount = "599"
	action, signatures := signedConfigActionWithNonce(t, current, next, keys, 1)
	if err := k.ApplyRouteConfigUpdate(ctx, current, action, next, signatures); err != ErrCapBelowOutstanding {
		t.Fatalf("cap below outstanding error = %v", err)
	}
	if k.ControlNonce(ctx) != 0 {
		t.Fatal("rejected update consumed the control nonce")
	}
	next.MaxOutstandingAmount = "600"
	action, signatures = signedConfigActionWithNonce(t, current, next, keys, 1)
	if err := k.ApplyRouteConfigUpdate(ctx, current, action, next, signatures); err != nil {
		t.Fatalf("cap equal to outstanding rejected: %v", err)
	}
	if err := k.ExportGenesis(ctx).Validate(); err != nil {
		t.Fatalf("exported genesis invalid: %v", err)
	}
}

// Sums beyond 256 bits are rejected as limit breaches instead of panicking.
func TestSettlementArithmeticCannotOverflow(t *testing.T) {
	f := newSettlementFixture(t, testChainID)
	maxUint256 := sdkmath.NewIntFromBigInt(new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1)))
	config := f.config
	config.MaxTransferAmount, config.DailyLimit, config.MaxOutstandingAmount = maxUint256.String(), maxUint256.String(), maxUint256.String()
	f.keeper.setOutstandingAmount(f.ctx, maxUint256)
	attestation := types.Attestation{
		Direction: types.DirectionCronosToXitcoin, Destination: sdk.AccAddress(bytes.Repeat([]byte{2}, 20)).String(), Amount: maxUint256.String(),
	}
	if err := f.keeper.SettleInbound(f.ctx, config, attestation); err != ErrOutstandingLimitExceeded {
		t.Fatalf("overflowing outstanding error = %v", err)
	}
	f.keeper.setOutstandingAmount(f.ctx, sdkmath.ZeroInt())
	f.bank.supply = maxUint256
	if err := f.keeper.SettleInbound(f.ctx, config, attestation); err != ErrMaximumSupplyExceeded {
		t.Fatalf("overflowing supply error = %v", err)
	}
}
