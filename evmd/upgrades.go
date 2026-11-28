package evmd

import (
	"context"

	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	upgradetypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"

	validatoradmissiontypes "github.com/xitcoin-xtc/pos-chain/x/validatoradmission/types"
	validatorincentivestypes "github.com/xitcoin-xtc/pos-chain/x/validatorincentives/types"
)

// UpgradeName defines the on-chain upgrade name for the sample EVMD upgrade
// from v0.6.0 to v0.7.0.
//
// NOTE: This upgrade defines a reference implementation of what an upgrade
// could look like when an application is migrating from EVMD version
// v0.6.x to v0.7.0.
const UpgradeName = "v0.6.0-to-v0.7.0"

// GovernanceSafeguardsUpgradeName disables executable public governance and
// transfers security-sensitive authorities to the Testnet V2 2-of-3 multisig.
const GovernanceSafeguardsUpgradeName = "xitcoin-governance-safeguards-v1"

// ValidatorIncentivesDailyV2UpgradeName activates the funded daily
// treasury-derived validator incentive policy. This upgrade changes module
// state only and does not add or remove a store.
const ValidatorIncentivesDailyV2UpgradeName = "xitcoin-validator-incentives-daily-v2"

// BridgeInitialRouteUpgradeName is the coordinated binary upgrade that makes
// authority-gated first-route initialization available. It does not configure,
// enable, or resume a route by itself.
const BridgeInitialRouteUpgradeName = "xitcoin-bridge-initial-route-v1"

// SecurityHardeningUpgradeName is the coordinated binary upgrade for the
// November 2026 security release. It runs the module migrations of that
// release, including the Cosmos SDK v0.54.5 x/feegrant queue migration
// (consensus version 2 to 3). It adds or removes no store.
const SecurityHardeningUpgradeName = "xitcoin-security-hardening-v1"

// SecurityHardeningV2UpgradeName is the coordinated binary upgrade that
// changes which x/bridge transactions are valid: amounts and route limits
// must be canonical decimals parsed by one parser for validation, limits,
// minting and burning; route IDs must be ASCII; attestation destinations and
// source references must not contain whitespace; route configuration
// addresses must be spelled exactly; a route update cannot set the
// outstanding cap below the outstanding amount. Its handler raises
// min_self_delegation of existing validators to the x/validatoradmission
// minimum (see MigrateValidatorMinSelfDelegation) and reports a stored bridge
// route configuration that the new validation rejects. It adds or removes no
// store.
const SecurityHardeningV2UpgradeName = "xitcoin-security-hardening-v2"

func (app EVMD) RegisterUpgradeHandlers() {
	app.UpgradeKeeper.SetUpgradeHandler(
		UpgradeName,
		func(ctx context.Context, _ upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
			sdkCtx := sdk.UnwrapSDKContext(ctx)
			sdkCtx.Logger().Debug("this is a debug level message to test that verbose logging mode has properly been enabled during a chain upgrade")
			return app.ModuleManager.RunMigrations(ctx, app.Configurator(), fromVM)
		},
	)

	app.UpgradeKeeper.SetUpgradeHandler(
		GovernanceSafeguardsUpgradeName,
		func(ctx context.Context, _ upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
			sdkCtx := sdk.UnwrapSDKContext(ctx)
			authority := mustTestnetAdministrativeAuthority()
			if fromVM == nil {
				fromVM = module.VersionMap{}
			}

			app.ValidatorAdmissionKeeper.SetAuthority(sdkCtx, authority)
			app.ValidatorAdmissionKeeper.SetMinimumSelfDelegation(
				sdkCtx,
				validatoradmissiontypes.DefaultMinimumSelfDelegation,
			)

			// validator_incentives is introduced by this upgrade on an existing
			// network. Initialize it exactly once without disturbing state on a
			// network that already contains the module.
			if _, exists := fromVM[validatorincentivestypes.ModuleName]; !exists {
				state := validatorincentivestypes.DefaultGenesisState()
				state.Authority = authority
				if err := app.ValidatorIncentivesKeeper.InitGenesis(sdkCtx, state); err != nil {
					return nil, err
				}
				fromVM[validatorincentivestypes.ModuleName] =
					app.ModuleManager.GetVersionMap()[validatorincentivestypes.ModuleName]
			} else {
				app.ValidatorIncentivesKeeper.SetAuthority(sdkCtx, authority)
			}

			return app.ModuleManager.RunMigrations(ctx, app.Configurator(), fromVM)
		},
	)

	app.UpgradeKeeper.SetUpgradeHandler(
		ValidatorIncentivesDailyV2UpgradeName,
		func(ctx context.Context, _ upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
			return app.ModuleManager.RunMigrations(ctx, app.Configurator(), fromVM)
		},
	)

	app.UpgradeKeeper.SetUpgradeHandler(
		BridgeInitialRouteUpgradeName,
		func(ctx context.Context, _ upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
			return app.ModuleManager.RunMigrations(ctx, app.Configurator(), fromVM)
		},
	)

	app.UpgradeKeeper.SetUpgradeHandler(
		SecurityHardeningUpgradeName,
		func(ctx context.Context, _ upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
			return app.ModuleManager.RunMigrations(ctx, app.Configurator(), fromVM)
		},
	)

	app.UpgradeKeeper.SetUpgradeHandler(SecurityHardeningV2UpgradeName, app.securityHardeningV2Handler)

	upgradeInfo, err := app.UpgradeKeeper.ReadUpgradeInfoFromDisk()
	if err != nil {
		panic(err)
	}

	if app.UpgradeKeeper.IsSkipHeight(upgradeInfo.Height) {
		return
	}

	var storeUpgrades *storetypes.StoreUpgrades
	switch upgradeInfo.Name {
	case UpgradeName:
		storeUpgrades = &storetypes.StoreUpgrades{Added: []string{}}
	case GovernanceSafeguardsUpgradeName:
		storeUpgrades = &storetypes.StoreUpgrades{
			Added: []string{validatorincentivestypes.StoreKey},
		}
	}

	if storeUpgrades != nil {
		// configure store loader that checks if version == upgradeHeight and applies store upgrades
		app.SetStoreLoader(upgradetypes.UpgradeStoreLoader(upgradeInfo.Height, storeUpgrades))
	}
}
