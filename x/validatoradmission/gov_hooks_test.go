package validatoradmission

import (
	"context"
	"testing"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/authz"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

func proposalWith(t *testing.T, msgs ...sdk.Msg) govtypes.Proposal {
	t.Helper()
	now := time.Unix(1_800_000_000, 0)
	proposal, err := govtypes.NewProposal(msgs, 1, now, now, "", "t", "s", sdk.AccAddress(make([]byte, 20)), false)
	if err != nil {
		t.Fatalf("proposal construction failed: %v", err)
	}
	return proposal
}

func restrictedProposalMessages(t *testing.T, f admissionFixture) map[string]sdk.Msg {
	t.Helper()
	grant, err := authz.NewMsgGrant(f.operator, f.operator,
		authz.NewGenericAuthorization(sdk.MsgTypeURL(&minttypes.MsgUpdateParams{})), nil)
	if err != nil {
		t.Fatalf("grant construction failed: %v", err)
	}
	inner, err := govtypes.NewMsgSubmitProposal([]sdk.Msg{&minttypes.MsgUpdateParams{Authority: f.operator.String()}},
		nil, f.operator.String(), "", "t", "s", false)
	if err != nil {
		t.Fatalf("proposal construction failed: %v", err)
	}
	return map[string]sdk.Msg{
		"mint parameters":            &minttypes.MsgUpdateParams{Authority: f.operator.String()},
		"validator creation":         f.createValidator(f.minimum.Amount),
		"unjail":                     &slashingtypes.MsgUnjail{ValidatorAddr: f.validator},
		"mint parameters in MsgExec": exec(f.operator, &minttypes.MsgUpdateParams{Authority: f.operator.String()}),
		"grant for mint parameters":  grant,
		"nested proposal":            inner,
	}
}

// With executable governance enabled (the legacy integration suites), a
// proposal carrying a restricted message passed the ante handler and x/gov
// executed it without any admission check: a mint parameter change went
// through. The ante handler and the x/gov hook now reject it.
func TestProposalsCannotCarryRestrictedMessages(t *testing.T) {
	f := newAdmissionFixture(t)
	f.keeper.SetApprovedValidator(f.ctx, f.validator, true)
	for name, msg := range restrictedProposalMessages(t, f) {
		t.Run(name, func(t *testing.T) {
			proposal, err := govtypes.NewMsgSubmitProposal([]sdk.Msg{msg}, nil, f.operator.String(), "", "t", "s", false)
			if err != nil {
				t.Fatalf("proposal construction failed: %v", err)
			}
			handler := NewAdmissionAnteHandler(f.keeper, func(ctx sdk.Context, _ sdk.Tx, _ bool) (sdk.Context, error) {
				t.Fatal("a proposal carrying a restricted message reached the next ante handler")
				return ctx, nil
			}, true)
			if _, err := handler(f.ctx, testTx{messages: []sdk.Msg{proposal}}, false); err == nil {
				t.Fatal("the ante handler accepted the proposal")
			}

			stored := proposalWith(t, msg)
			hooks := NewGovHooks(f.keeper, func(context.Context, uint64) (govtypes.Proposal, error) { return stored, nil }, true)
			if err := hooks.AfterProposalSubmission(f.ctx, stored.Id, f.operator); err == nil {
				t.Fatal("the governance hook accepted the proposal")
			}
		})
	}

	harmless := proposalWith(t, &stakingtypes.MsgDelegate{DelegatorAddress: f.operator.String()})
	hooks := NewGovHooks(f.keeper, func(context.Context, uint64) (govtypes.Proposal, error) { return harmless, nil }, true)
	if err := hooks.AfterProposalSubmission(f.ctx, harmless.Id, f.operator); err != nil {
		t.Fatalf("the governance hook rejected an unrestricted proposal: %v", err)
	}
	disabled := NewGovHooks(f.keeper, func(context.Context, uint64) (govtypes.Proposal, error) { return harmless, nil }, false)
	if err := disabled.AfterProposalSubmission(f.ctx, harmless.Id, f.operator); err == nil {
		t.Fatal("the governance hook accepted a proposal while executable governance is disabled")
	}
}
