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
// approved or whose MinSelfDelegation is below the on-chain minimum. The
// staking module calls it before the self delegation is made; an error aborts
// the whole message.
//
// It does nothing during InitChain: x/staking also calls it for validators
// imported from genesis state (for example an exported chain, which can hold
// revoked validators), and genesis transactions are checked by the ante
// handler and by the application's genesis validation instead.
func (h StakingHooks) AfterValidatorCreated(goCtx context.Context, valAddr sdk.ValAddress) error {
	ctx := sdk.UnwrapSDKContext(goCtx)
	if isGenesis(ctx) {
		return nil
	}
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
