package validatoradmission

import (
	"context"

	"github.com/xitcoin-xtc/pos-chain/x/validatoradmission/keeper"

	errorsmod "cosmossdk.io/errors"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
)

// ProposalReader returns a stored governance proposal.
type ProposalReader func(ctx context.Context, proposalID uint64) (govv1.Proposal, error)

// GovHooks enforces the admission policy on every new governance proposal
// inside x/gov itself, so that it also holds for proposals submitted through
// a path that does not pass the transaction ante handler (for example the
// governance precompile). x/gov executes the messages of a passed proposal
// without the ante handler, so they are checked when the proposal is
// submitted, like the messages of an authz MsgExec.
type GovHooks struct {
	keeper          keeper.Keeper
	proposal        ProposalReader
	allowGovernance bool
}

var _ govtypes.GovHooks = GovHooks{}

// NewGovHooks returns the validator admission governance hooks.
// allowGovernanceProposalSubmission must be the setting of the ante handler.
func NewGovHooks(admissionKeeper keeper.Keeper, proposal ProposalReader, allowGovernanceProposalSubmission bool) GovHooks {
	return GovHooks{keeper: admissionKeeper, proposal: proposal, allowGovernance: allowGovernanceProposalSubmission}
}

// AfterProposalSubmission rejects every proposal while executable governance
// is disabled, and otherwise a proposal carrying a restricted message (a
// validator creation or unjail, a mint parameter change, a grant for one of
// them) at any depth. x/gov calls it inside SubmitProposal; an error aborts
// the submission.
func (h GovHooks) AfterProposalSubmission(goCtx context.Context, proposalID uint64, _ sdk.AccAddress) error {
	if !h.allowGovernance {
		return errGovernanceDisabled()
	}
	proposal, err := h.proposal(goCtx, proposalID)
	if err != nil {
		return err
	}
	messages, err := proposal.GetMsgs()
	if err != nil {
		return errorsmod.Wrap(sdkerrors.ErrInvalidRequest, err.Error())
	}
	checker := admissionChecker{keeper: h.keeper, allowGovernance: h.allowGovernance}
	return checker.checkMessages(sdk.UnwrapSDKContext(goCtx), messages, 1)
}

func (GovHooks) AfterProposalDeposit(context.Context, uint64, sdk.AccAddress) error { return nil }

func (GovHooks) AfterProposalVote(context.Context, uint64, sdk.AccAddress) error { return nil }

func (GovHooks) AfterProposalFailedMinDeposit(context.Context, uint64) error { return nil }

func (GovHooks) AfterProposalVotingPeriodEnded(context.Context, uint64) error { return nil }
