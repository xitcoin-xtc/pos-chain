package evmd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpgradeNamesAreStableAndUnique(t *testing.T) {
	names := []string{
		UpgradeName,
		GovernanceSafeguardsUpgradeName,
		ValidatorIncentivesDailyV2UpgradeName,
		BridgeInitialRouteUpgradeName,
		SecurityHardeningUpgradeName,
	}

	require.Equal(t, "xitcoin-validator-incentives-daily-v2", ValidatorIncentivesDailyV2UpgradeName)
	require.Equal(t, "xitcoin-bridge-initial-route-v1", BridgeInitialRouteUpgradeName)
	require.Equal(t, "xitcoin-security-hardening-v1", SecurityHardeningUpgradeName)

	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		require.NotEmpty(t, name)
		_, duplicate := seen[name]
		require.False(t, duplicate, "duplicate upgrade name %q", name)
		seen[name] = struct{}{}
	}
}
