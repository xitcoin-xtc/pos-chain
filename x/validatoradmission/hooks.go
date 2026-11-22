package validatoradmission

import (
	"context"

	"cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/xitcoin-xtc/pos-chain/x/validatoradmission/keeper"
)

// ValidatorReader is the part of the staking keeper used by the hooks.
type ValidatorReader interface {
	GetValidator(ctx context.Context, addr sdk.ValAddress) (stakingtypes.Validator, error)
}

// StakingHooks enforces validator admission inside the staking module itself,
// so the policy also holds for validators created through any path that does
// not pass the transaction ante handler (for example an EVM precompile).
type StakingHooks struct {
	keeper     keeper.Keeper
	validators ValidatorReader
}

var _ stakingtypes.StakingHooks = StakingHooks{}

// NewStakingHooks returns the validator admission staking hooks.
func NewStakingHooks(admissionKeeper keeper.Keeper, validators ValidatorReader) StakingHooks {
	return StakingHooks{keeper: admissionKeeper, validators: validators}
}

// AfterValidatorCreated rejects the creation of a validator that is not
// approved, and after genesis a validator whose MinSelfDelegation is below the
// on-chain minimum. The staking module calls it before the self delegation is
// made; an error aborts the whole message.
func (h StakingHooks) AfterValidatorCreated(goCtx context.Context, valAddr sdk.ValAddress) error {
	ctx := sdk.UnwrapSDKContext(goCtx)
	validator, err := h.validators.GetValidator(goCtx, valAddr)
	if err != nil {
		return err
	}
	if !h.keeper.IsApprovedValidator(ctx, validator.OperatorAddress) {
		return errNotApproved()
	}

	minimum, err := minimumSelfDelegation(ctx, h.keeper)
	if err != nil {
		return err
	}
	return validateMinSelfDelegation(ctx, minimum, validator.MinSelfDelegation)
}

func (StakingHooks) BeforeValidatorModified(context.Context, sdk.ValAddress) error { return nil }

func (StakingHooks) AfterValidatorRemoved(context.Context, sdk.ConsAddress, sdk.ValAddress) error {
	return nil
}

func (StakingHooks) AfterValidatorBonded(context.Context, sdk.ConsAddress, sdk.ValAddress) error {
	return nil
}

func (StakingHooks) AfterValidatorBeginUnbonding(context.Context, sdk.ConsAddress, sdk.ValAddress) error {
	return nil
}

func (StakingHooks) BeforeDelegationCreated(context.Context, sdk.AccAddress, sdk.ValAddress) error {
	return nil
}

func (StakingHooks) BeforeDelegationSharesModified(context.Context, sdk.AccAddress, sdk.ValAddress) error {
	return nil
}

func (StakingHooks) BeforeDelegationRemoved(context.Context, sdk.AccAddress, sdk.ValAddress) error {
	return nil
}

func (StakingHooks) AfterDelegationModified(context.Context, sdk.AccAddress, sdk.ValAddress) error {
	return nil
}

func (StakingHooks) BeforeValidatorSlashed(context.Context, sdk.ValAddress, math.LegacyDec) error {
	return nil
}

func (StakingHooks) AfterUnbondingInitiated(context.Context, uint64) error { return nil }
