package keeper

import (
	"bytes"
	"context"
	"testing"

	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	ed25519 "github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	sdktestutil "github.com/cosmos/cosmos-sdk/testutil"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/xitcoin-xtc/pos-chain/x/validatoradmission/types"
)

type testStakingKeeper struct {
	getCalls    int
	validator   stakingtypes.Validator
	getErr      error
	jailed      bool
	jailAddress sdk.ConsAddress
}

func (s *testStakingKeeper) GetValidator(
	_ context.Context,
	_ sdk.ValAddress,
) (stakingtypes.Validator, error) {
	s.getCalls++
	if s.getErr != nil {
		return stakingtypes.Validator{}, s.getErr
	}
	return s.validator, nil
}

func (s *testStakingKeeper) Jail(
	_ context.Context,
	consAddress sdk.ConsAddress,
) error {
	s.jailed = true
	s.jailAddress = consAddress
	return nil
}

func TestApproveAndRevokeValidator(t *testing.T) {
	key := storetypes.NewKVStoreKey(types.StoreKey)
	ctx := sdktestutil.DefaultContext(
		key,
		storetypes.NewTransientStoreKey("validator_admission_test"),
	)

	authority := sdk.AccAddress(bytes.Repeat([]byte{1}, 20)).String()
	wrongAuthority := sdk.AccAddress(bytes.Repeat([]byte{2}, 20)).String()
	validator := sdk.ValAddress(bytes.Repeat([]byte{3}, 20)).String()

	k := NewKeeper(key)
	k.SetAuthority(ctx, authority)

	staking := &testStakingKeeper{getErr: stakingtypes.ErrNoValidatorFound}
	server := NewMsgServer(k, staking)
	goCtx := sdk.WrapSDKContext(ctx)

	_, err := server.ApproveValidator(goCtx, &types.MsgApproveValidator{
		Authority:        wrongAuthority,
		ValidatorAddress: validator,
	})
	if err == nil {
		t.Fatal("approval from a non-authority was accepted")
	}
	if k.IsApprovedValidator(ctx, validator) {
		t.Fatal("non-authority changed the approval list")
	}

	_, err = server.ApproveValidator(goCtx, &types.MsgApproveValidator{
		Authority:        authority,
		ValidatorAddress: validator,
	})
	if err != nil {
		t.Fatalf("authority approval rejected: %v", err)
	}
	if !k.IsApprovedValidator(ctx, validator) {
		t.Fatal("approved validator missing from on-chain state")
	}

	_, err = server.RevokeValidator(goCtx, &types.MsgRevokeValidator{
		Authority:        authority,
		ValidatorAddress: validator,
	})
	if err != nil {
		t.Fatalf("authority revocation rejected: %v", err)
	}
	if k.IsApprovedValidator(ctx, validator) {
		t.Fatal("revoked validator remained approved")
	}
	if staking.getCalls != 1 {
		t.Fatalf("expected one validator lookup, got %d", staking.getCalls)
	}
}

func TestRevokeExistingValidatorSuspendsIt(t *testing.T) {
	key := storetypes.NewKVStoreKey(types.StoreKey)
	ctx := sdktestutil.DefaultContext(
		key,
		storetypes.NewTransientStoreKey("validator_admission_revoke_test"),
	)

	authority := sdk.AccAddress(bytes.Repeat([]byte{11}, 20)).String()
	validatorAddress := sdk.ValAddress(bytes.Repeat([]byte{12}, 20)).String()

	consensusKey := &ed25519.PubKey{Key: bytes.Repeat([]byte{13}, 32)}
	consensusAny, err := codectypes.NewAnyWithValue(consensusKey)
	if err != nil {
		t.Fatalf("consensus key packaging failed: %v", err)
	}

	validator := stakingtypes.Validator{
		OperatorAddress: validatorAddress,
		ConsensusPubkey: consensusAny,
	}

	k := NewKeeper(key)
	k.SetAuthority(ctx, authority)
	k.SetApprovedValidator(ctx, validatorAddress, true)

	staking := &testStakingKeeper{validator: validator}
	server := NewMsgServer(k, staking)

	_, err = server.RevokeValidator(
		sdk.WrapSDKContext(ctx),
		&types.MsgRevokeValidator{
			Authority:        authority,
			ValidatorAddress: validatorAddress,
		},
	)
	if err != nil {
		t.Fatalf("revocation rejected: %v", err)
	}

	if k.IsApprovedValidator(ctx, validatorAddress) {
		t.Fatal("revoked validator remained approved")
	}
	if !staking.jailed {
		t.Fatal("existing revoked validator was not suspended")
	}

	expected := sdk.ConsAddress(consensusKey.Address())
	if !bytes.Equal(staking.jailAddress, expected) {
		t.Fatalf("wrong consensus address suspended: got %X want %X",
			staking.jailAddress, expected)
	}
}

func TestPolicyCapacityAndUpdateParams(t *testing.T) {
	key := storetypes.NewKVStoreKey(types.StoreKey)
	ctx := sdktestutil.DefaultContext(
		key,
		storetypes.NewTransientStoreKey("validator_admission_policy_test"),
	)

	authority := sdk.AccAddress(bytes.Repeat([]byte{31}, 20)).String()
	otherAuthority := sdk.AccAddress(bytes.Repeat([]byte{32}, 20)).String()
	validatorOne := sdk.ValAddress(bytes.Repeat([]byte{33}, 20)).String()
	validatorTwo := sdk.ValAddress(bytes.Repeat([]byte{34}, 20)).String()

	k := NewKeeper(key)
	k.SetAuthority(ctx, authority)
	k.SetMaxApprovedValidators(ctx, 1)

	server := NewMsgServer(k, &testStakingKeeper{getErr: stakingtypes.ErrNoValidatorFound})
	goCtx := sdk.WrapSDKContext(ctx)

	_, err := server.ApproveValidator(goCtx, &types.MsgApproveValidator{
		Authority: authority, ValidatorAddress: validatorOne,
	})
	if err != nil {
		t.Fatalf("first approval rejected: %v", err)
	}

	_, err = server.ApproveValidator(goCtx, &types.MsgApproveValidator{
		Authority: authority, ValidatorAddress: validatorTwo,
	})
	if err == nil {
		t.Fatal("approval above configured capacity was accepted")
	}

	_, err = server.UpdateParams(goCtx, &types.MsgUpdateParams{
		Authority:             otherAuthority,
		MaxApprovedValidators: 2,
		MinimumSelfDelegation: types.DefaultMinimumSelfDelegation,
	})
	if err == nil {
		t.Fatal("non-authority policy update was accepted")
	}

	_, err = server.UpdateParams(goCtx, &types.MsgUpdateParams{
		Authority:             authority,
		MaxApprovedValidators: 0,
		MinimumSelfDelegation: types.DefaultMinimumSelfDelegation,
	})
	if err == nil {
		t.Fatal("invalid capacity update was accepted")
	}

	_, err = server.UpdateParams(goCtx, &types.MsgUpdateParams{
		Authority:             authority,
		MaxApprovedValidators: 2,
		MinimumSelfDelegation: types.DefaultMinimumSelfDelegation,
	})
	if err != nil {
		t.Fatalf("authority policy update rejected: %v", err)
	}
	if got := k.GetMaxApprovedValidators(ctx); got != 2 {
		t.Fatalf("wrong capacity after update: got %d want 2", got)
	}
}

func TestQueryPolicyAndValidatorApproval(t *testing.T) {
	key := storetypes.NewKVStoreKey(types.StoreKey)
	ctx := sdktestutil.DefaultContext(
		key,
		storetypes.NewTransientStoreKey("validator_admission_query_test"),
	)

	authority := sdk.AccAddress(bytes.Repeat([]byte{41}, 20)).String()
	validator := sdk.ValAddress(bytes.Repeat([]byte{42}, 20)).String()

	k := NewKeeper(key)
	k.SetAuthority(ctx, authority)
	k.SetMaxApprovedValidators(ctx, 208)
	k.SetMinimumSelfDelegation(ctx, types.DefaultMinimumSelfDelegation)
	k.SetApprovedValidator(ctx, validator, true)

	goCtx := sdk.WrapSDKContext(ctx)

	params, err := k.Params(goCtx, &types.QueryParamsRequest{})
	if err != nil {
		t.Fatalf("policy query failed: %v", err)
	}
	if params.Authority != authority ||
		params.MaxApprovedValidators != 208 ||
		params.MinimumSelfDelegation != types.DefaultMinimumSelfDelegation {
		t.Fatal("policy query returned incorrect values")
	}

	status, err := k.Validator(goCtx, &types.QueryValidatorRequest{
		ValidatorAddress: validator,
	})
	if err != nil {
		t.Fatalf("validator query failed: %v", err)
	}
	if !status.Approved {
		t.Fatal("approved validator was reported as unapproved")
	}
}
