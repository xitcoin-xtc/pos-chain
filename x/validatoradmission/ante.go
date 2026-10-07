package validatoradmission

import (
	errorsmod "cosmossdk.io/errors"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/xitcoin-xtc/pos-chain/x/validatoradmission/keeper"
)

func NewAdmissionAnteHandler(
	admissionKeeper keeper.Keeper,
	next sdk.AnteHandler,
	allowGovernanceProposalSubmission ...bool,
) sdk.AnteHandler {
	allowGovernance := len(allowGovernanceProposalSubmission) > 0 &&
		allowGovernanceProposalSubmission[0]

	return func(ctx sdk.Context, tx sdk.Tx, simulate bool) (sdk.Context, error) {
		for _, message := range tx.GetMsgs() {
			switch msg := message.(type) {
			case *stakingtypes.MsgCreateValidator:
				if !admissionKeeper.IsApprovedValidator(ctx, msg.ValidatorAddress) {
					return ctx, errorsmod.Wrap(
						sdkerrors.ErrUnauthorized,
						"Validator Admission: validator is not approved by Xitcoin",
					)
				}

				minimumSelfDelegation, err := sdk.ParseCoinNormalized(
					admissionKeeper.GetMinimumSelfDelegation(ctx),
				)
				if err != nil {
					return ctx, errorsmod.Wrap(
						sdkerrors.ErrInvalidRequest,
						"Validator Admission: invalid on-chain minimum self delegation",
					)
				}
				if msg.Value.Denom != minimumSelfDelegation.Denom ||
					msg.Value.Amount.LT(minimumSelfDelegation.Amount) {
					return ctx, errorsmod.Wrapf(
						sdkerrors.ErrUnauthorized,
						"Validator Admission: self delegation must be at least %s",
						minimumSelfDelegation.String(),
					)
				}

			case *slashingtypes.MsgUnjail:
				if !admissionKeeper.IsApprovedValidator(ctx, msg.ValidatorAddr) {
					return ctx, errorsmod.Wrap(
						sdkerrors.ErrUnauthorized,
						"Validator Admission: revoked validator cannot unjail",
					)
				}

			case *minttypes.MsgUpdateParams:
				return ctx, errorsmod.Wrap(
					sdkerrors.ErrUnauthorized,
					"Fixed monetary policy: Mint parameters cannot be changed",
				)

			case *govtypes.MsgSubmitProposal:
				if allowGovernance {
					continue
				}
				return ctx, errorsmod.Wrap(
					sdkerrors.ErrUnauthorized,
					"Governance safeguards: executable on-chain proposals are disabled",
				)
			}
		}

		return next(ctx, tx, simulate)
	}
}
