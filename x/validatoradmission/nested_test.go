package validatoradmission

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"

	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	sdktestutil "github.com/cosmos/cosmos-sdk/testutil"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/authz"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	govv1beta1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1beta1"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/xitcoin-xtc/pos-chain/x/validatoradmission/keeper"
	"github.com/xitcoin-xtc/pos-chain/x/validatoradmission/types"
)

type admissionFixture struct {
	ctx       sdk.Context
	keeper    keeper.Keeper
	operator  sdk.AccAddress
	validator string
	minimum   sdk.Coin
}

func newAdmissionFixture(t *testing.T) admissionFixture {
	t.Helper()
	key := storetypes.NewKVStoreKey(types.StoreKey)
	ctx := sdktestutil.DefaultContext(key, storetypes.NewTransientStoreKey("validator_admission_nested_test"))
	minimum, err := sdk.ParseCoinNormalized(types.DefaultMinimumSelfDelegation)
	if err != nil {
		t.Fatalf("minimum self delegation parsing failed: %v", err)
	}
	operator := sdk.AccAddress(bytes.Repeat([]byte{41}, 20))
	return admissionFixture{
		// A block height above zero: genesis-only exemptions do not apply.
		ctx:       ctx.WithBlockHeight(100),
		keeper:    keeper.NewKeeper(key),
		operator:  operator,
		validator: sdk.ValAddress(operator).String(),
		minimum:   minimum,
	}
}

func (f admissionFixture) createValidator(minSelfDelegation sdkmath.Int) *stakingtypes.MsgCreateValidator {
	return &stakingtypes.MsgCreateValidator{
		ValidatorAddress:  f.validator,
		Value:             f.minimum,
		MinSelfDelegation: minSelfDelegation,
	}
}

// run executes the admission ante handler and reports whether the
// transaction reached the next handler.
func (f admissionFixture) run(t *testing.T, messages ...sdk.Msg) (bool, error) {
	t.Helper()
	nextCalled := false
	handler := NewAdmissionAnteHandler(f.keeper, func(ctx sdk.Context, _ sdk.Tx, _ bool) (sdk.Context, error) {
		nextCalled = true
		return ctx, nil
	})
	_, err := handler(f.ctx, testTx{messages: messages}, false)
	return nextCalled, err
}

func (f admissionFixture) requireRejected(t *testing.T, want string, messages ...sdk.Msg) {
	t.Helper()
	nextCalled, err := f.run(t, messages...)
	if err == nil {
		t.Fatal("transaction was accepted")
	}
	if nextCalled {
		t.Fatal("rejected transaction reached the next ante handler")
	}
	if want != "" && !strings.Contains(err.Error(), want) {
		t.Fatalf("unexpected error %q, want it to contain %q", err, want)
	}
}

func exec(grantee sdk.AccAddress, messages ...sdk.Msg) *authz.MsgExec {
	msg := authz.NewMsgExec(grantee, messages)
	return &msg
}

// The authz keeper accepts MsgExec without a grant when granter equals
// grantee. Before the fix this let an unapproved operator create a validator
// below the minimum by wrapping MsgCreateValidator in its own MsgExec.
func TestAdmissionRejectsSelfGrantedMsgExecCreateValidator(t *testing.T) {
	f := newAdmissionFixture(t)
	underfunded := f.createValidator(sdkmath.OneInt())
	underfunded.Value = sdk.NewInt64Coin(f.minimum.Denom, 1)

	f.requireRejected(t, "cannot be executed through authz", exec(f.operator, underfunded))

	// Approval does not make the authz path acceptable either.
	f.keeper.SetApprovedValidator(f.ctx, f.validator, true)
	f.requireRejected(t, "cannot be executed through authz", exec(f.operator, f.createValidator(f.minimum.Amount)))
}

func TestAdmissionRejectsNestedMsgExec(t *testing.T) {
	f := newAdmissionFixture(t)
	nested := exec(f.operator, exec(f.operator, exec(f.operator, f.createValidator(sdkmath.OneInt()))))
	f.requireRejected(t, "cannot be executed through authz", nested)

	// A harmless message nested too deeply is rejected by the depth bound.
	var deep sdk.Msg = &stakingtypes.MsgDelegate{DelegatorAddress: f.operator.String()}
	for i := 0; i < maxNestedMessages; i++ {
		deep = exec(f.operator, deep)
	}
	f.requireRejected(t, "nested deeper", deep)
}

func TestAdmissionRejectsUnjailOfRevokedValidator(t *testing.T) {
	f := newAdmissionFixture(t)
	unjail := &slashingtypes.MsgUnjail{ValidatorAddr: f.validator}

	// Approved, then revoked by the authority.
	f.keeper.SetApprovedValidator(f.ctx, f.validator, true)
	f.keeper.SetApprovedValidator(f.ctx, f.validator, false)

	f.requireRejected(t, "revoked validator cannot unjail", unjail)
	f.requireRejected(t, "cannot be executed through authz", exec(f.operator, unjail))
	f.requireRejected(t, "cannot be executed through authz", exec(f.operator, exec(f.operator, unjail)))

	// Even an approved validator must unjail directly, not through authz.
	f.keeper.SetApprovedValidator(f.ctx, f.validator, true)
	f.requireRejected(t, "cannot be executed through authz", exec(f.operator, unjail))
	if reached, err := f.run(t, unjail); err != nil || !reached {
		t.Fatalf("direct unjail of an approved validator was rejected: %v", err)
	}
}

func TestAdmissionRejectsGovernanceAndMintThroughMsgExec(t *testing.T) {
	f := newAdmissionFixture(t)
	proposer := f.operator.String()
	for name, msg := range map[string]sdk.Msg{
		"gov v1 proposal":      &govtypes.MsgSubmitProposal{Proposer: proposer, Title: "t", Summary: "s"},
		"gov v1beta1 proposal": &govv1beta1.MsgSubmitProposal{Proposer: proposer},
		"mint params":          &minttypes.MsgUpdateParams{Authority: proposer},
	} {
		t.Run(name, func(t *testing.T) {
			f.requireRejected(t, "", msg)
			f.requireRejected(t, "cannot be executed through authz", exec(f.operator, msg))
		})
	}
}

func TestAdmissionRejectsGrantsForRestrictedMessages(t *testing.T) {
	f := newAdmissionFixture(t)
	expiration := time.Unix(2_000_000_000, 0)
	for _, typeURL := range []string{
		sdk.MsgTypeURL(&stakingtypes.MsgCreateValidator{}),
		sdk.MsgTypeURL(&slashingtypes.MsgUnjail{}),
		sdk.MsgTypeURL(&govtypes.MsgSubmitProposal{}),
	} {
		grant, err := authz.NewMsgGrant(f.operator, f.operator, authz.NewGenericAuthorization(typeURL), &expiration)
		if err != nil {
			t.Fatalf("grant construction failed: %v", err)
		}
		f.requireRejected(t, "authorizations for", grant)
	}

	delegate, err := authz.NewMsgGrant(f.operator, f.operator,
		authz.NewGenericAuthorization(sdk.MsgTypeURL(&stakingtypes.MsgDelegate{})), &expiration)
	if err != nil {
		t.Fatalf("grant construction failed: %v", err)
	}
	if reached, err := f.run(t, delegate); err != nil || !reached {
		t.Fatalf("unrestricted grant was rejected: %v", err)
	}
}

func TestAdmissionRequiresMinSelfDelegationAtLeastMinimum(t *testing.T) {
	f := newAdmissionFixture(t)
	f.keeper.SetApprovedValidator(f.ctx, f.validator, true)

	f.requireRejected(t, "min_self_delegation must be at least", f.createValidator(sdkmath.OneInt()))
	f.requireRejected(t, "min_self_delegation must be at least", f.createValidator(f.minimum.Amount.SubRaw(1)))

	if reached, err := f.run(t, f.createValidator(f.minimum.Amount)); err != nil || !reached {
		t.Fatalf("validator meeting the minimum was rejected: %v", err)
	}

	// Genesis transactions keep their historical MinSelfDelegation; genesis
	// approval is validated separately when the chain is initialised.
	genesis := f
	genesis.ctx = f.ctx.WithBlockHeight(0)
	if reached, err := genesis.run(t, f.createValidator(sdkmath.OneInt())); err != nil || !reached {
		t.Fatalf("genesis validator was rejected: %v", err)
	}
}

// A MsgCreateValidator decoded without an amount carries a nil Int. Before
// the fix the comparison with the minimum panicked in the ante handler for an
// approved operator, so a block proposer could include such a transaction to
// get a panic result instead of a rejection.
func TestAdmissionRejectsCreateValidatorWithoutAmount(t *testing.T) {
	f := newAdmissionFixture(t)
	f.keeper.SetApprovedValidator(f.ctx, f.validator, true)
	msg := f.createValidator(f.minimum.Amount)
	msg.Value = sdk.Coin{Denom: f.minimum.Denom}
	f.requireRejected(t, "self delegation must be at least", msg)
	f.requireRejected(t, "", exec(f.operator, msg))
}

type fakeValidators map[string]stakingtypes.Validator

func (v fakeValidators) GetValidator(_ context.Context, addr sdk.ValAddress) (stakingtypes.Validator, error) {
	validator, ok := v[addr.String()]
	if !ok {
		return stakingtypes.Validator{}, stakingtypes.ErrNoValidatorFound
	}
	return validator, nil
}

// The staking hook covers every path that creates a validator, including the
// staking precompile, which does not pass the transaction ante handler.
func TestStakingHookEnforcesAdmission(t *testing.T) {
	f := newAdmissionFixture(t)
	valAddr := sdk.ValAddress(f.operator)
	validators := fakeValidators{f.validator: {OperatorAddress: f.validator, MinSelfDelegation: f.minimum.Amount}}
	hooks := NewStakingHooks(f.keeper, validators)

	if err := hooks.AfterValidatorCreated(f.ctx, valAddr); err == nil {
		t.Fatal("unapproved validator creation passed the staking hook")
	}

	f.keeper.SetApprovedValidator(f.ctx, f.validator, true)
	if err := hooks.AfterValidatorCreated(f.ctx, valAddr); err != nil {
		t.Fatalf("approved validator creation was rejected: %v", err)
	}

	validators[f.validator] = stakingtypes.Validator{OperatorAddress: f.validator, MinSelfDelegation: sdkmath.OneInt()}
	if err := hooks.AfterValidatorCreated(f.ctx, valAddr); err == nil {
		t.Fatal("validator with a low min_self_delegation passed the staking hook")
	}
	// Genesis import (InitChain) is checked elsewhere: exported state may hold
	// validators whose approval was revoked.
	f.keeper.SetApprovedValidator(f.ctx, f.validator, false)
	if err := hooks.AfterValidatorCreated(f.ctx.WithBlockHeight(0), valAddr); err != nil {
		t.Fatalf("genesis validator import was rejected by the staking hook: %v", err)
	}
}

type recordingStakingServer struct {
	stakingtypes.MsgServer
	called bool
}

func (s *recordingStakingServer) CreateValidator(context.Context, *stakingtypes.MsgCreateValidator) (*stakingtypes.MsgCreateValidatorResponse, error) {
	s.called = true
	return &stakingtypes.MsgCreateValidatorResponse{}, nil
}

type recordingSlashingServer struct {
	slashingtypes.MsgServer
	called bool
}

func (s *recordingSlashingServer) Unjail(context.Context, *slashingtypes.MsgUnjail) (*slashingtypes.MsgUnjailResponse, error) {
	s.called = true
	return &slashingtypes.MsgUnjailResponse{}, nil
}

// The precompiles call message servers directly; the wrappers apply the same
// policy there.
func TestPrecompileMessageServersEnforceAdmission(t *testing.T) {
	f := newAdmissionFixture(t)

	staking := &recordingStakingServer{}
	wrappedStaking := NewStakingMsgServer(f.keeper, staking)
	if _, err := wrappedStaking.CreateValidator(f.ctx, f.createValidator(f.minimum.Amount)); err == nil || staking.called {
		t.Fatal("unapproved validator creation reached the staking message server")
	}
	f.keeper.SetApprovedValidator(f.ctx, f.validator, true)
	if _, err := wrappedStaking.CreateValidator(f.ctx, f.createValidator(f.minimum.Amount)); err != nil || !staking.called {
		t.Fatalf("approved validator creation did not reach the staking message server: %v", err)
	}

	f.keeper.SetApprovedValidator(f.ctx, f.validator, false)
	slashing := &recordingSlashingServer{}
	wrappedSlashing := NewSlashingMsgServer(f.keeper, slashing)
	if _, err := wrappedSlashing.Unjail(f.ctx, &slashingtypes.MsgUnjail{ValidatorAddr: f.validator}); err == nil || slashing.called {
		t.Fatal("revoked validator unjail reached the slashing message server")
	}
}
