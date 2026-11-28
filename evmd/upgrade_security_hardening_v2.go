package evmd

import (
	"context"
	"errors"

	bridgekeeper "github.com/xitcoin-xtc/pos-chain/x/bridge/keeper"
	validatoradmissionkeeper "github.com/xitcoin-xtc/pos-chain/x/validatoradmission/keeper"

	sdkmath "cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	upgradetypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"
)

// Events emitted by the xitcoin-security-hardening-v2 upgrade handler.
const (
	// EventTypeMinSelfDelegationRaised: min_self_delegation of a validator
	// was raised to the validator-admission minimum.
	EventTypeMinSelfDelegationRaised = "xitcoin_min_self_delegation_raised"
	// EventTypeMinSelfDelegationBelowMinimum: the validator's
	// min_self_delegation is below the minimum and its self-delegation does
	// not cover the minimum either; it was left unchanged.
	EventTypeMinSelfDelegationBelowMinimum = "xitcoin_min_self_delegation_below_minimum"
	// EventTypeMinSelfDelegationMigrationSkipped: the on-chain minimum is
	// unusable (unparsable, not positive, or not in the bond denomination),
	// so no validator was changed.
	EventTypeMinSelfDelegationMigrationSkipped = "xitcoin_min_self_delegation_migration_skipped"
	// EventTypeBridgeRouteConfigInvalid: the stored bridge route
	// configuration fails the canonical validation of this release.
	EventTypeBridgeRouteConfigInvalid = "xitcoin_bridge_route_config_invalid"
)

// MinSelfDelegationMigration reports the outcome for every validator.
type MinSelfDelegationMigration struct {
	Raised           []string // operator addresses raised to the minimum
	BelowMinimum     []string // operator addresses left unchanged
	AlreadyAtMinimum int
	Skipped          bool
}

func (app EVMD) securityHardeningV2Handler(ctx context.Context, _ upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if _, err := MigrateValidatorMinSelfDelegation(sdkCtx, app.StakingKeeper, app.ValidatorAdmissionKeeper); err != nil {
		return nil, err
	}
	// Reported through an event and the log, never fatal: see
	// CheckBridgeRouteConfig.
	_ = CheckBridgeRouteConfig(sdkCtx, app.BridgeKeeper)
	return app.ModuleManager.RunMigrations(ctx, app.Configurator(), fromVM)
}

// MigrateValidatorMinSelfDelegation raises the min_self_delegation of every
// validator below the x/validatoradmission on-chain minimum to that minimum,
// when the validator's current self-delegation (valued as the staking module
// values it when deciding to jail an operator) already covers the minimum.
//
// Genesis validators are exempt from the min_self_delegation check at
// creation, so they kept the value of their genesis transactions (1 axtc on
// xitcoin-testnet-v2-1). After this migration the staking module enforces
// the admission minimum for them as for every later validator: an operator
// whose self-delegation falls below it is jailed on self-undelegation and
// cannot unjail until it tops up.
//
// A validator whose self-delegation is below the minimum (for example after a
// downtime slash) is left unchanged, with an error log and an
// EventTypeMinSelfDelegationBelowMinimum event. Raising its value would put
// it in breach immediately (any self-undelegation jails it and it could not
// unjail), and failing the upgrade would halt the whole network for one
// validator. The operator, or the admission authority, resolves it after the
// upgrade (top up and MsgEditValidator, or revocation).
//
// Only store errors are returned. The migration is idempotent.
func MigrateValidatorMinSelfDelegation(
	ctx sdk.Context,
	staking *stakingkeeper.Keeper,
	admission validatoradmissionkeeper.Keeper,
) (MinSelfDelegationMigration, error) {
	var report MinSelfDelegationMigration
	logger := ctx.Logger().With("upgrade", SecurityHardeningV2UpgradeName)
	configured := admission.GetMinimumSelfDelegation(ctx)
	skip := func(reason string) (MinSelfDelegationMigration, error) {
		report.Skipped = true
		logger.Error("min_self_delegation migration skipped", "reason", reason, "minimum", configured)
		ctx.EventManager().EmitEvent(sdk.NewEvent(
			EventTypeMinSelfDelegationMigrationSkipped,
			sdk.NewAttribute("reason", reason),
			sdk.NewAttribute("minimum", configured),
		))
		return report, nil
	}

	minimum, err := sdk.ParseCoinNormalized(configured)
	if err != nil || !minimum.IsPositive() {
		return skip("invalid on-chain minimum self delegation")
	}
	bondDenom, err := staking.BondDenom(ctx)
	if err != nil {
		return report, err
	}
	if minimum.Denom != bondDenom {
		return skip("minimum self delegation is not in the bond denomination")
	}

	validators, err := staking.GetAllValidators(ctx)
	if err != nil {
		return report, err
	}
	for _, validator := range validators {
		operator := validator.GetOperator()
		if !validator.MinSelfDelegation.IsNil() && validator.MinSelfDelegation.GTE(minimum.Amount) {
			report.AlreadyAtMinimum++
			continue
		}
		self, err := selfDelegationTokens(ctx, staking, validator)
		if err != nil {
			return report, err
		}
		if self.LT(minimum.Amount) {
			report.BelowMinimum = append(report.BelowMinimum, operator)
			logger.Error(
				"validator self-delegation is below the admission minimum; min_self_delegation left unchanged",
				"validator", operator, "min_self_delegation", validator.MinSelfDelegation.String(),
				"self_delegation", self.String(), "minimum", minimum.Amount.String(),
			)
			ctx.EventManager().EmitEvent(sdk.NewEvent(
				EventTypeMinSelfDelegationBelowMinimum,
				sdk.NewAttribute("validator", operator),
				sdk.NewAttribute("min_self_delegation", validator.MinSelfDelegation.String()),
				sdk.NewAttribute("self_delegation", self.String()),
				sdk.NewAttribute("minimum", minimum.Amount.String()),
			))
			continue
		}
		previous := validator.MinSelfDelegation.String()
		validator.MinSelfDelegation = minimum.Amount
		if err := staking.SetValidator(ctx, validator); err != nil {
			return report, err
		}
		report.Raised = append(report.Raised, operator)
		logger.Info("raised validator min_self_delegation to the admission minimum",
			"validator", operator, "previous", previous, "minimum", minimum.Amount.String())
		ctx.EventManager().EmitEvent(sdk.NewEvent(
			EventTypeMinSelfDelegationRaised,
			sdk.NewAttribute("validator", operator),
			sdk.NewAttribute("previous_min_self_delegation", previous),
			sdk.NewAttribute("min_self_delegation", minimum.Amount.String()),
			sdk.NewAttribute("self_delegation", self.String()),
		))
	}
	return report, nil
}

// selfDelegationTokens values the operator's own delegation exactly as the
// staking module does before jailing an operator on undelegation.
func selfDelegationTokens(ctx sdk.Context, staking *stakingkeeper.Keeper, validator stakingtypes.Validator) (sdkmath.Int, error) {
	operator, err := staking.ValidatorAddressCodec().StringToBytes(validator.GetOperator())
	if err != nil {
		return sdkmath.Int{}, err
	}
	delegation, err := staking.GetDelegation(ctx, sdk.AccAddress(operator), sdk.ValAddress(operator))
	if errors.Is(err, stakingtypes.ErrNoDelegation) {
		return sdkmath.ZeroInt(), nil
	}
	if err != nil {
		return sdkmath.Int{}, err
	}
	if validator.DelegatorShares.IsZero() {
		return sdkmath.ZeroInt(), nil
	}
	return validator.TokensFromShares(delegation.Shares).TruncateInt(), nil
}

// CheckBridgeRouteConfig reports a stored bridge route configuration that the
// canonical validation of this release rejects (a limit that is not a
// canonical decimal, a non-ASCII route ID, or a signer or guardian address
// with surrounding whitespace). It never changes the configuration: a limit
// such as "010" was read as 10 by some paths and 8 by others, so no value can
// be chosen for it without the 2-of-3 signers. Such a route stays unusable
// (fail-closed: no mint, burn or update) until a later upgrade repairs it, so
// operators must check `xitcoind query bridge route` before scheduling this
// upgrade (docs/migrations/xitcoin-security-hardening-v2.md). Failing the
// upgrade instead would halt the network without making a repair possible.
func CheckBridgeRouteConfig(ctx sdk.Context, bridge bridgekeeper.Keeper) error {
	if _, _, err := bridge.GetRouteConfig(ctx); err != nil {
		ctx.Logger().With("upgrade", SecurityHardeningV2UpgradeName).Error(
			"stored bridge route configuration is invalid under canonical validation", "error", err.Error())
		ctx.EventManager().EmitEvent(sdk.NewEvent(EventTypeBridgeRouteConfigInvalid, sdk.NewAttribute("error", err.Error())))
		return err
	}
	return nil
}
