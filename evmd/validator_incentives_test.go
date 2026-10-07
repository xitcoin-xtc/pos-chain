package evmd

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	evmconfig "github.com/xitcoin-xtc/pos-chain/evmd/config"
	validatorincentivestypes "github.com/xitcoin-xtc/pos-chain/x/validatorincentives/types"
)

func TestValidatorIncentivesWiring(t *testing.T) {
	app, genesis := setup(
		true,
		0,
		"xitcoin-validator-incentives-test",
		101089,
	)

	raw, found := genesis[validatorincentivestypes.ModuleName]
	require.True(t, found)
	require.NotEmpty(t, raw)

	var state validatorincentivestypes.GenesisState
	require.NoError(t, json.Unmarshal(raw, &state))
	require.NoError(t, state.Validate())
	require.Equal(
		t,
		TestnetAdministrativeMultisigAddress,
		state.Authority,
	)
	require.Equal(
		t,
		validatorincentivestypes.DefaultParams(),
		state.Params,
	)

	require.NotNil(
		t,
		app.GetKey(validatorincentivestypes.StoreKey),
	)

	permissions := evmconfig.GetMaccPerms()
	modulePermissions, found := permissions[validatorincentivestypes.TreasuryAccountName]
	require.True(t, found)
	require.Empty(t, modulePermissions)

	treasuryAddress := authtypes.NewModuleAddress(
		validatorincentivestypes.TreasuryAccountName,
	)
	require.NotEmpty(t, treasuryAddress)
	blockedAddresses := evmconfig.BlockedAddresses()
	require.False(t, blockedAddresses[treasuryAddress.String()])
	require.True(
		t,
		blockedAddresses[authtypes.NewModuleAddress(
			authtypes.FeeCollectorName,
		).String()],
	)
}
