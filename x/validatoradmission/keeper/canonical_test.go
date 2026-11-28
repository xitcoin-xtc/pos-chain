package keeper

import (
	"bytes"
	"strings"
	"testing"

	"github.com/xitcoin-xtc/pos-chain/x/validatoradmission/types"

	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	sdktestutil "github.com/cosmos/cosmos-sdk/testutil"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

// The bech32 decoder accepts the upper-case spelling of an operator address.
// Approvals used to be stored and looked up under the spelling given, so a
// revocation spelled in upper case jailed the validator but deleted nothing:
// the lower-case approval stayed, and the validator could unjail at once.
func TestRevocationRemovesTheApprovalWhateverTheSpelling(t *testing.T) {
	key := storetypes.NewKVStoreKey(types.StoreKey)
	ctx := sdktestutil.DefaultContext(key, storetypes.NewTransientStoreKey("validator_admission_canonical_test"))
	authority := sdk.AccAddress(bytes.Repeat([]byte{1}, 20)).String()
	validator := sdk.ValAddress(bytes.Repeat([]byte{3}, 20)).String()
	upper := strings.ToUpper(validator)

	k := NewKeeper(key)
	k.SetAuthority(ctx, authority)
	staking := &testStakingKeeper{getErr: stakingtypes.ErrNoValidatorFound}
	server := NewMsgServer(k, staking)

	if _, err := server.ApproveValidator(ctx, &types.MsgApproveValidator{Authority: authority, ValidatorAddress: validator}); err != nil {
		t.Fatalf("approval rejected: %v", err)
	}
	if !k.IsApprovedValidator(ctx, upper) {
		t.Fatal("the upper-case spelling of an approved operator is not approved")
	}
	if _, err := server.RevokeValidator(ctx, &types.MsgRevokeValidator{Authority: authority, ValidatorAddress: upper}); err != nil {
		t.Fatalf("revocation rejected: %v", err)
	}
	if k.IsApprovedValidator(ctx, validator) || k.IsApprovedValidator(ctx, upper) {
		t.Fatal("a revocation spelled in upper case left the operator approved")
	}

	// An approval spelled in upper case approves the canonical operator,
	// once.
	if _, err := server.ApproveValidator(ctx, &types.MsgApproveValidator{Authority: authority, ValidatorAddress: upper}); err != nil {
		t.Fatalf("approval rejected: %v", err)
	}
	if _, err := server.ApproveValidator(ctx, &types.MsgApproveValidator{Authority: authority, ValidatorAddress: validator}); err != nil {
		t.Fatalf("approval rejected: %v", err)
	}
	if !k.IsApprovedValidator(ctx, validator) || k.ApprovedValidatorCount(ctx) != 1 {
		t.Fatalf("approved=%v count=%d, want one canonical approval", k.IsApprovedValidator(ctx, validator), k.ApprovedValidatorCount(ctx))
	}
	if got := k.ExportGenesis(ctx).ApprovedValidators; len(got) != 1 || got[0] != validator {
		t.Fatalf("exported approvals %v, want [%s]", got, validator)
	}

	// Whitespace and malformed addresses name no operator.
	for _, spelling := range []string{" " + validator, validator + " ", "not-an-operator", ""} {
		if k.IsApprovedValidator(ctx, spelling) {
			t.Fatalf("%q is approved", spelling)
		}
	}
}

// An entry stored under a non-canonical spelling by an earlier binary is
// never matched, can still be deleted by a revocation spelled the same way,
// and is removed by RemoveNonCanonicalApprovals.
func TestNonCanonicalApprovalEntries(t *testing.T) {
	key := storetypes.NewKVStoreKey(types.StoreKey)
	ctx := sdktestutil.DefaultContext(key, storetypes.NewTransientStoreKey("validator_admission_canonical_test"))
	k := NewKeeper(key)
	validator := sdk.ValAddress(bytes.Repeat([]byte{4}, 20)).String()
	other := sdk.ValAddress(bytes.Repeat([]byte{5}, 20)).String()
	k.SetApprovedValidator(ctx, other, true)
	for _, spelling := range []string{strings.ToUpper(validator), "garbage"} {
		ctx.KVStore(key).Set(approvalKey(spelling), []byte{1})
	}
	if k.IsApprovedValidator(ctx, validator) || k.IsApprovedValidator(ctx, strings.ToUpper(validator)) {
		t.Fatal("a non-canonical entry approves its operator")
	}

	k.SetApprovedValidator(ctx, "garbage", false)
	if ctx.KVStore(key).Has(approvalKey("garbage")) {
		t.Fatal("a revocation spelled like the entry did not delete it")
	}
	removed := k.RemoveNonCanonicalApprovals(ctx)
	if len(removed) != 1 || removed[0] != strings.ToUpper(validator) {
		t.Fatalf("removed %v", removed)
	}
	if k.ApprovedValidatorCount(ctx) != 1 || !k.IsApprovedValidator(ctx, other) {
		t.Fatal("a canonical approval was removed")
	}
	if again := k.RemoveNonCanonicalApprovals(ctx); len(again) != 0 {
		t.Fatalf("second run removed %v", again)
	}
}
