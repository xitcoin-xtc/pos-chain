package evmd

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"

	testconstants "github.com/xitcoin-xtc/pos-chain/testutil/constants"
	bridgetypes "github.com/xitcoin-xtc/pos-chain/x/bridge/types"
	evmtypes "github.com/xitcoin-xtc/pos-chain/x/vm/types"

	coreheader "cosmossdk.io/core/header"
	sdkmath "cosmossdk.io/math"

	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	upgradetypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"
)

func TestUpgradeNamesAreStableAndUnique(t *testing.T) {
	names := []string{
		UpgradeName,
		GovernanceSafeguardsUpgradeName,
		ValidatorIncentivesDailyV2UpgradeName,
		BridgeInitialRouteUpgradeName,
		SecurityHardeningUpgradeName,
		SecurityHardeningV2UpgradeName,
	}

	require.Equal(t, "xitcoin-validator-incentives-daily-v2", ValidatorIncentivesDailyV2UpgradeName)
	require.Equal(t, "xitcoin-bridge-initial-route-v1", BridgeInitialRouteUpgradeName)
	require.Equal(t, "xitcoin-security-hardening-v1", SecurityHardeningUpgradeName)
	require.Equal(t, "xitcoin-security-hardening-v2", SecurityHardeningV2UpgradeName)

	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		require.NotEmpty(t, name)
		_, duplicate := seen[name]
		require.False(t, duplicate, "duplicate upgrade name %q", name)
		seen[name] = struct{}{}
	}
}

const testSelfDelegationMinimum = "5000000000000000000000000"

type validatorFixture struct {
	operator sdk.ValAddress
	minSelf  sdkmath.Int
}

// upgradeTestApp returns an initialized application and a context on which
// the security-hardening-v2 handler can run, with four validators shaped like
// the xitcoin-testnet-v2-1 genesis validators and edge cases around them.
func upgradeTestApp(t *testing.T) (*EVMD, sdk.Context, map[string]validatorFixture) {
	t.Helper()
	// Every application initializes the process-wide EVM configuration.
	evmtypes.NewEVMConfigurator().ResetTestConfig()
	chainID := testconstants.ExampleChainID
	app := Setup(t, chainID.ChainID, chainID.EVMChainID)
	height, now := app.LastBlockHeight()+1, time.Unix(1800000000, 0).UTC()
	ctx := app.NewContextLegacy(false, cmtproto.Header{Height: height, ChainID: chainID.ChainID, Time: now}).
		WithHeaderInfo(coreheader.Info{Height: height, ChainID: chainID.ChainID, Time: now})
	bondDenom, err := app.StakingKeeper.BondDenom(ctx)
	require.NoError(t, err)
	app.ValidatorAdmissionKeeper.SetMinimumSelfDelegation(ctx, testSelfDelegationMinimum+bondDenom)

	minimum, ok := sdkmath.NewIntFromString(testSelfDelegationMinimum)
	require.True(t, ok)
	one := sdkmath.OneInt()
	fixtures := map[string]validatorFixture{}
	add := func(name string, seed byte, tokens, selfShares, otherShares, minSelf sdkmath.Int) {
		operator := sdk.ValAddress(bytes.Repeat([]byte{seed}, 20))
		validator, err := stakingtypes.NewValidator(operator.String(), ed25519.GenPrivKeyFromSecret([]byte{seed}).PubKey(), stakingtypes.Description{Moniker: name})
		require.NoError(t, err)
		validator.Tokens = tokens
		validator.DelegatorShares = sdkmath.LegacyNewDecFromInt(selfShares.Add(otherShares))
		validator.MinSelfDelegation = minSelf
		require.NoError(t, app.StakingKeeper.SetValidator(ctx, validator))
		if selfShares.IsPositive() {
			require.NoError(t, app.StakingKeeper.SetDelegation(ctx, stakingtypes.NewDelegation(sdk.AccAddress(operator).String(), operator.String(), sdkmath.LegacyNewDecFromInt(selfShares))))
		}
		if otherShares.IsPositive() {
			other := sdk.AccAddress(bytes.Repeat([]byte{seed + 100}, 20))
			require.NoError(t, app.StakingKeeper.SetDelegation(ctx, stakingtypes.NewDelegation(other.String(), operator.String(), sdkmath.LegacyNewDecFromInt(otherShares))))
		}
		fixtures[name] = validatorFixture{operator: operator, minSelf: minSelf}
	}
	// Exactly the minimum self-bonded with min_self_delegation = 1, as the
	// four live testnet validators.
	add("genesis-like", 1, minimum, minimum, sdkmath.ZeroInt(), one)
	// More than the minimum self-bonded, plus an external delegation.
	add("over-minimum", 2, minimum.MulRaw(2), minimum.AddRaw(7), minimum.SubRaw(7), one)
	// Slashed by 1% (testnet downtime fraction): self-delegation 4,950,000 XTC.
	add("slashed", 3, minimum.MulRaw(99).QuoRaw(100), minimum, sdkmath.ZeroInt(), one)
	// Only an external delegation, no self-delegation at all.
	add("no-self-delegation", 4, minimum.MulRaw(3), sdkmath.ZeroInt(), minimum.MulRaw(3), one)
	// Already compliant, and above the minimum.
	add("compliant", 5, minimum, minimum, sdkmath.ZeroInt(), minimum)
	add("above-minimum", 6, minimum.MulRaw(2), minimum.MulRaw(2), sdkmath.ZeroInt(), minimum.AddRaw(1))
	return app, ctx, fixtures
}

func eventsOfType(ctx sdk.Context, eventType string) map[string]map[string]string {
	found := map[string]map[string]string{}
	for _, event := range ctx.EventManager().Events() {
		if event.Type != eventType {
			continue
		}
		attributes := map[string]string{}
		for _, attribute := range event.Attributes {
			attributes[attribute.Key] = attribute.Value
		}
		found[attributes["validator"]] = attributes
	}
	return found
}

func TestSecurityHardeningV2RaisesMinSelfDelegationOfCoveredValidators(t *testing.T) {
	app, ctx, fixtures := upgradeTestApp(t)
	minimum, _ := sdkmath.NewIntFromString(testSelfDelegationMinimum)

	ctx = ctx.WithEventManager(sdk.NewEventManager())
	_, err := app.securityHardeningV2Handler(ctx, upgradetypes.Plan{Name: SecurityHardeningV2UpgradeName, Height: ctx.BlockHeight()}, app.ModuleManager.GetVersionMap())
	require.NoError(t, err)

	want := map[string]sdkmath.Int{
		"genesis-like":       minimum,
		"over-minimum":       minimum,
		"slashed":            sdkmath.OneInt(),
		"no-self-delegation": sdkmath.OneInt(),
		"compliant":          minimum,
		"above-minimum":      minimum.AddRaw(1),
	}
	for name, fixture := range fixtures {
		validator, err := app.StakingKeeper.GetValidator(ctx, fixture.operator)
		require.NoError(t, err)
		require.True(t, validator.MinSelfDelegation.Equal(want[name]), "%s: min_self_delegation %s, want %s", name, validator.MinSelfDelegation, want[name])
	}

	raised := eventsOfType(ctx, EventTypeMinSelfDelegationRaised)
	below := eventsOfType(ctx, EventTypeMinSelfDelegationBelowMinimum)
	require.Contains(t, raised, fixtures["genesis-like"].operator.String())
	require.Contains(t, raised, fixtures["over-minimum"].operator.String())
	require.Equal(t, "1", raised[fixtures["genesis-like"].operator.String()]["previous_min_self_delegation"])
	require.Contains(t, below, fixtures["slashed"].operator.String())
	require.Contains(t, below, fixtures["no-self-delegation"].operator.String())
	require.Equal(t, minimum.MulRaw(99).QuoRaw(100).String(), below[fixtures["slashed"].operator.String()]["self_delegation"])
	for _, name := range []string{"compliant", "above-minimum"} {
		require.NotContains(t, raised, fixtures[name].operator.String())
		require.NotContains(t, below, fixtures[name].operator.String())
	}
	require.Empty(t, eventsOfType(ctx, EventTypeBridgeRouteConfigInvalid))
}

func TestSecurityHardeningV2MigrationIsIdempotent(t *testing.T) {
	app, ctx, fixtures := upgradeTestApp(t)
	first, err := MigrateValidatorMinSelfDelegation(ctx, app.StakingKeeper, app.ValidatorAdmissionKeeper)
	require.NoError(t, err)
	require.Len(t, first.Raised, 2)

	before := map[string]stakingtypes.Validator{}
	for name, fixture := range fixtures {
		before[name], err = app.StakingKeeper.GetValidator(ctx, fixture.operator)
		require.NoError(t, err)
	}
	second, err := MigrateValidatorMinSelfDelegation(ctx.WithEventManager(sdk.NewEventManager()), app.StakingKeeper, app.ValidatorAdmissionKeeper)
	require.NoError(t, err)
	require.Empty(t, second.Raised)
	require.ElementsMatch(t, first.BelowMinimum, second.BelowMinimum)
	require.Equal(t, first.AlreadyAtMinimum+len(first.Raised), second.AlreadyAtMinimum)
	for name, fixture := range fixtures {
		after, err := app.StakingKeeper.GetValidator(ctx, fixture.operator)
		require.NoError(t, err)
		require.Equal(t, before[name], after, name)
	}
}

func TestSecurityHardeningV2RunsThroughTheUpgradeKeeper(t *testing.T) {
	app, ctx, fixtures := upgradeTestApp(t)
	require.True(t, app.UpgradeKeeper.HasHandler(SecurityHardeningV2UpgradeName))
	require.NoError(t, app.UpgradeKeeper.ApplyUpgrade(ctx, upgradetypes.Plan{Name: SecurityHardeningV2UpgradeName, Height: ctx.BlockHeight()}))
	validator, err := app.StakingKeeper.GetValidator(ctx, fixtures["genesis-like"].operator)
	require.NoError(t, err)
	require.Equal(t, testSelfDelegationMinimum, validator.MinSelfDelegation.String())
	done, err := app.UpgradeKeeper.GetDoneHeight(ctx, SecurityHardeningV2UpgradeName)
	require.NoError(t, err)
	require.Equal(t, ctx.BlockHeight(), done)
}

func TestSecurityHardeningV2SkipsAnUnusableMinimum(t *testing.T) {
	for name, minimum := range map[string]string{
		"other denomination": testSelfDelegationMinimum + "uother",
		"unparsable":         "five million",
	} {
		app, ctx, fixtures := upgradeTestApp(t)
		app.ValidatorAdmissionKeeper.SetMinimumSelfDelegation(ctx, minimum)
		ctx = ctx.WithEventManager(sdk.NewEventManager())
		report, err := MigrateValidatorMinSelfDelegation(ctx, app.StakingKeeper, app.ValidatorAdmissionKeeper)
		require.NoError(t, err, name)
		require.True(t, report.Skipped, name)
		require.Empty(t, report.Raised, name)
		validator, err := app.StakingKeeper.GetValidator(ctx, fixtures["genesis-like"].operator)
		require.NoError(t, err)
		require.True(t, validator.MinSelfDelegation.Equal(sdkmath.OneInt()), name)
		require.NotEmpty(t, eventsOfType(ctx, EventTypeMinSelfDelegationMigrationSkipped), name)
	}
}

func TestSecurityHardeningV2ReportsAnInvalidStoredBridgeRoute(t *testing.T) {
	app, ctx, _ := upgradeTestApp(t)
	// A route stored by an earlier binary with a limit this release rejects
	// ("010" was 10 for the limit checks and 8 for the outstanding cap).
	stored := []byte(`{"route_id":"cronos-testnet-xitcoin-testnet","bridge_signers":["0x00000000000000000000000000000000000000A1","0x00000000000000000000000000000000000000A2","0x00000000000000000000000000000000000000A3"],"guardian":"0x00000000000000000000000000000000000000B4","max_transfer_amount":"010","daily_limit":"100","max_outstanding_amount":"1000","enabled":false}`)
	ctx.KVStore(app.GetKey(bridgetypes.StoreKey)).Set(bridgetypes.KeyRouteConfig, stored)
	ctx = ctx.WithEventManager(sdk.NewEventManager())

	_, err := app.securityHardeningV2Handler(ctx, upgradetypes.Plan{Name: SecurityHardeningV2UpgradeName, Height: ctx.BlockHeight()}, app.ModuleManager.GetVersionMap())
	require.NoError(t, err, "an invalid stored route must not halt the upgrade")
	require.NotEmpty(t, eventsOfType(ctx, EventTypeBridgeRouteConfigInvalid))
	require.Equal(t, stored, ctx.KVStore(app.GetKey(bridgetypes.StoreKey)).Get(bridgetypes.KeyRouteConfig), "the stored route must not be rewritten")
}
