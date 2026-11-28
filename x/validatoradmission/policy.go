package validatoradmission

import (
	errorsmod "cosmossdk.io/errors"
	sdkmath "cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/xitcoin-xtc/pos-chain/x/validatoradmission/keeper"
)

// minimumSelfDelegation returns the on-chain minimum self delegation.
func minimumSelfDelegation(ctx sdk.Context, admissionKeeper keeper.Keeper) (sdk.Coin, error) {
	minimum, err := sdk.ParseCoinNormalized(admissionKeeper.GetMinimumSelfDelegation(ctx))
	if err != nil {
		return sdk.Coin{}, errorsmod.Wrap(
			sdkerrors.ErrInvalidRequest,
			"Validator Admission: invalid on-chain minimum self delegation",
		)
	}
	return minimum, nil
}

// isGenesis reports whether ctx belongs to InitChain. The SDK uses block
// height zero for genesis transactions; genesis validators are checked
// against the genesis approval list when the chain is initialised.
func isGenesis(ctx sdk.Context) bool {
	return ctx.BlockHeight() == 0
}

// ValidateCreateValidator enforces the Xitcoin admission policy for a new
// validator: the operator must be approved, the initial self delegation must
// be at least the on-chain minimum, and, after genesis, MinSelfDelegation
// must also be at least that minimum so that the staking module keeps
// enforcing it for the lifetime of the validator.
func ValidateCreateValidator(
	ctx sdk.Context,
	admissionKeeper keeper.Keeper,
	msg *stakingtypes.MsgCreateValidator,
) error {
	if !admissionKeeper.IsApprovedValidator(ctx, msg.ValidatorAddress) {
		return errNotApproved()
	}

	minimum, err := minimumSelfDelegation(ctx, admissionKeeper)
	if err != nil {
		return err
	}
	// A decoded message may carry no amount at all (a nil Int): compare only
	// a present amount.
	if msg.Value.Denom != minimum.Denom || msg.Value.Amount.IsNil() || msg.Value.Amount.LT(minimum.Amount) {
		return errorsmod.Wrapf(
			sdkerrors.ErrUnauthorized,
			"Validator Admission: self delegation must be at least %s",
			minimum.String(),
		)
	}

	return validateMinSelfDelegation(ctx, minimum, msg.MinSelfDelegation)
}

func validateMinSelfDelegation(ctx sdk.Context, minimum sdk.Coin, value sdkmath.Int) error {
	if isGenesis(ctx) {
		return nil
	}
	if value.IsNil() || value.LT(minimum.Amount) {
		return errorsmod.Wrapf(
			sdkerrors.ErrUnauthorized,
			"Validator Admission: min_self_delegation must be at least %s",
			minimum.Amount.String(),
		)
	}
	return nil
}

func errNotApproved() error {
	return errorsmod.Wrap(
		sdkerrors.ErrUnauthorized,
		"Validator Admission: validator is not approved by Xitcoin",
	)
}

// ValidateUnjail rejects unjailing a validator whose approval was revoked.
func ValidateUnjail(ctx sdk.Context, admissionKeeper keeper.Keeper, validatorAddress string) error {
	if !admissionKeeper.IsApprovedValidator(ctx, validatorAddress) {
		return errorsmod.Wrap(
			sdkerrors.ErrUnauthorized,
			"Validator Admission: revoked validator cannot unjail",
		)
	}
	return nil
}
