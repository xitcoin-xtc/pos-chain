package validatoradmission

import (
	errorsmod "cosmossdk.io/errors"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/cosmos/cosmos-sdk/x/authz"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	govv1beta1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1beta1"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/xitcoin-xtc/pos-chain/x/validatoradmission/keeper"
)

// maxNestedMessages bounds the authz MsgExec nesting depth that is inspected.
// It matches the limit of the authz limiter decorator in the EVM ante chain.
const maxNestedMessages = 7

// NewAdmissionAnteHandler enforces the Xitcoin validator admission, fixed
// monetary policy and governance safeguards for every message of a
// transaction, including messages wrapped in authz MsgExec or carried by a
// governance proposal, at any depth.
//
// Validator creation and unjailing, mint parameter changes and governance
// proposals are never accepted inside MsgExec: they must be signed directly
// by the validator operator or proposer, where the full checks apply. Granting
// an authorization for those message types is rejected as well. When
// executable proposals are enabled, the messages of a proposal are checked
// like the messages of a MsgExec: x/gov executes them later without this
// handler. GovHooks applies the same check inside x/gov.
func NewAdmissionAnteHandler(
	admissionKeeper keeper.Keeper,
	next sdk.AnteHandler,
	allowGovernanceProposalSubmission ...bool,
) sdk.AnteHandler {
	checker := admissionChecker{
		keeper: admissionKeeper,
		allowGovernance: len(allowGovernanceProposalSubmission) > 0 &&
			allowGovernanceProposalSubmission[0],
	}

	return func(ctx sdk.Context, tx sdk.Tx, simulate bool) (sdk.Context, error) {
		if err := checker.checkMessages(ctx, tx.GetMsgs(), 0); err != nil {
			return ctx, err
		}
		return next(ctx, tx, simulate)
	}
}

type admissionChecker struct {
	keeper          keeper.Keeper
	allowGovernance bool
}

func (c admissionChecker) checkMessages(ctx sdk.Context, messages []sdk.Msg, depth int) error {
	if depth >= maxNestedMessages {
		return errorsmod.Wrapf(
			sdkerrors.ErrUnauthorized,
			"Validator Admission: authz messages nested deeper than %d levels",
			maxNestedMessages,
		)
	}

	nested := depth > 0
	for _, message := range messages {
		if nested && c.isRestricted(message) {
			return errorsmod.Wrapf(
				sdkerrors.ErrUnauthorized,
				"Validator Admission: %s cannot be executed through authz or a governance proposal",
				sdk.MsgTypeURL(message),
			)
		}

		switch msg := message.(type) {
		case *authz.MsgExec:
			inner, err := msg.GetMessages()
			if err != nil {
				return errorsmod.Wrap(sdkerrors.ErrInvalidRequest, err.Error())
			}
			if err := c.checkMessages(ctx, inner, depth+1); err != nil {
				return err
			}

		case *authz.MsgGrant:
			authorization, err := msg.GetAuthorization()
			if err != nil {
				return errorsmod.Wrap(sdkerrors.ErrInvalidRequest, err.Error())
			}
			if c.isRestrictedTypeURL(authorization.MsgTypeURL()) {
				return errorsmod.Wrapf(
					sdkerrors.ErrUnauthorized,
					"Validator Admission: authorizations for %s are not allowed",
					authorization.MsgTypeURL(),
				)
			}

		case *stakingtypes.MsgCreateValidator:
			if err := ValidateCreateValidator(ctx, c.keeper, msg); err != nil {
				return err
			}

		case *slashingtypes.MsgUnjail:
			if err := ValidateUnjail(ctx, c.keeper, msg.ValidatorAddr); err != nil {
				return err
			}

		case *minttypes.MsgUpdateParams:
			return errorsmod.Wrap(
				sdkerrors.ErrUnauthorized,
				"Fixed monetary policy: Mint parameters cannot be changed",
			)

		case *govtypes.MsgSubmitProposal:
			if err := c.checkProposalMessages(ctx, msg, depth); err != nil {
				return err
			}

		case *govv1beta1.MsgSubmitProposal:
			// A legacy proposal carries a content, not messages.
			if !c.allowGovernance {
				return errGovernanceDisabled()
			}
		}
	}

	return nil
}

// checkProposalMessages rejects a proposal while executable governance is
// disabled, and otherwise checks its messages as nested messages.
func (c admissionChecker) checkProposalMessages(ctx sdk.Context, proposal *govtypes.MsgSubmitProposal, depth int) error {
	if !c.allowGovernance {
		return errGovernanceDisabled()
	}
	inner, err := proposal.GetMsgs()
	if err != nil {
		return errorsmod.Wrap(sdkerrors.ErrInvalidRequest, err.Error())
	}
	return c.checkMessages(ctx, inner, depth+1)
}

func errGovernanceDisabled() error {
	return errorsmod.Wrap(
		sdkerrors.ErrUnauthorized,
		"Governance safeguards: executable on-chain proposals are disabled",
	)
}

func (c admissionChecker) isRestricted(message sdk.Msg) bool {
	return c.isRestrictedTypeURL(sdk.MsgTypeURL(message))
}

// isRestrictedTypeURL lists the message types that must be signed directly.
func (c admissionChecker) isRestrictedTypeURL(typeURL string) bool {
	switch typeURL {
	case sdk.MsgTypeURL(&stakingtypes.MsgCreateValidator{}),
		sdk.MsgTypeURL(&slashingtypes.MsgUnjail{}),
		sdk.MsgTypeURL(&minttypes.MsgUpdateParams{}):
		return true
	case sdk.MsgTypeURL(&govtypes.MsgSubmitProposal{}),
		sdk.MsgTypeURL(&govv1beta1.MsgSubmitProposal{}):
		return !c.allowGovernance
	default:
		return false
	}
}
