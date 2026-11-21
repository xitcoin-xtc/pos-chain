package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNetworkAuthorities(t *testing.T) {
	authority, err := AdministrativeAuthority(TestnetCosmosChainID)
	require.NoError(t, err)
	require.Equal(t, TestnetAdministrativeMultisigAddress, authority)

	// Mainnet values are not decided: the mainnet chain ID is refused.
	_, err = AdministrativeAuthority(MainnetCosmosChainID)
	require.ErrorContains(t, err, "not configured")

	// The testnet multisig can never become the mainnet authority.
	saved := Mainnet
	t.Cleanup(func() { Mainnet = saved })
	Mainnet.AdministrativeAuthority = TestnetAdministrativeMultisigAddress
	_, err = AdministrativeAuthority(MainnetCosmosChainID)
	require.ErrorContains(t, err, "must not use the testnet administrative multisig")

	// Development and test chains keep the previous default.
	authority, err = AdministrativeAuthority("xitcoin-local-1")
	require.NoError(t, err)
	require.Equal(t, TestnetAdministrativeMultisigAddress, authority)
}

func TestNetworkEVMChainIDs(t *testing.T) {
	require.Equal(t, uint64(101089), DefaultEVMChainID(TestnetCosmosChainID))
	require.Equal(t, uint64(101088), DefaultEVMChainID(MainnetCosmosChainID))
	require.Equal(t, TestnetEVMChainID, DefaultEVMChainID("xitcoin-local-1"))
	require.Equal(t, TestnetEVMChainID, DefaultEVMChainID(""))

	require.NoError(t, ValidateEVMChainID(TestnetCosmosChainID, 101089))
	require.NoError(t, ValidateEVMChainID(MainnetCosmosChainID, 101088))
	require.NoError(t, ValidateEVMChainID("xitcoin-local-1", 262144))
	require.ErrorContains(t, ValidateEVMChainID(TestnetCosmosChainID, 101088), "requires evm.evm-chain-id = 101089")
	require.ErrorContains(t, ValidateEVMChainID(MainnetCosmosChainID, 101089), "requires evm.evm-chain-id = 101088")
}
