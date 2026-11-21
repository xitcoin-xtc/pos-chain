package config

import (
	"fmt"
	"strings"
)

// Network holds the consensus-relevant identity of one Xitcoin network. It is
// compiled into the binary and selected by the Cosmos chain ID, so every node
// of a network uses the same values; none of them is a node-local option.
type Network struct {
	Name          string
	CosmosChainID string
	EVMChainID    uint64
	// AdministrativeAuthority is the account that is the authority of
	// x/upgrade and x/bridge and the default validator_incentives authority
	// of new genesis files. An empty value means it is not decided yet and
	// the binary refuses to run that network.
	AdministrativeAuthority string
}

const (
	// TestnetAdministrativeMultisigAddress is the public 2-of-3
	// administrative authority of Xitcoin Testnet. Its private keys are never
	// embedded in the application.
	TestnetAdministrativeMultisigAddress = "xtc1vza8zsgvrfwmve084ytd8xqdkkm7u9e5csctc2"

	TestnetCosmosChainID        = "xitcoin-testnet-v2-1"
	MainnetCosmosChainID        = "xitcoin"
	MainnetEVMChainID    uint64 = 101088
)

var (
	// Testnet is the active public test network.
	Testnet = Network{
		Name:                    "Xitcoin Testnet",
		CosmosChainID:           TestnetCosmosChainID,
		EVMChainID:              TestnetEVMChainID,
		AdministrativeAuthority: TestnetAdministrativeMultisigAddress,
	}

	// Mainnet authorities are decided before launch. Until they are set
	// here, this release refuses to run the mainnet chain ID.
	Mainnet = Network{
		Name:                    "Xitcoin",
		CosmosChainID:           MainnetCosmosChainID,
		EVMChainID:              MainnetEVMChainID,
		AdministrativeAuthority: "",
	}
)

// KnownNetwork returns the public network that uses chainID, if any.
func KnownNetwork(chainID string) (Network, bool) {
	switch strings.TrimSpace(chainID) {
	case TestnetCosmosChainID:
		return Testnet, true
	case MainnetCosmosChainID:
		return Mainnet, true
	default:
		return Network{}, false
	}
}

// AdministrativeAuthority returns the administrative authority for chainID.
//
// Public networks use their own value. The mainnet chain ID is refused while
// its authority is undecided, and it can never use the testnet multisig.
// Other chain IDs (local development and test chains) keep the previous
// default, the testnet multisig, so their behaviour does not change.
func AdministrativeAuthority(chainID string) (string, error) {
	network, known := KnownNetwork(chainID)
	if !known {
		return TestnetAdministrativeMultisigAddress, nil
	}
	if network.AdministrativeAuthority == "" {
		return "", fmt.Errorf("%s (%s): administrative authority is not configured in this release", network.Name, network.CosmosChainID)
	}
	if network.CosmosChainID == MainnetCosmosChainID && network.AdministrativeAuthority == TestnetAdministrativeMultisigAddress {
		return "", fmt.Errorf("%s must not use the testnet administrative multisig", network.Name)
	}
	return network.AdministrativeAuthority, nil
}

// DefaultEVMChainID returns the EVM chain ID written to app.toml for a new
// node of chainID: the network's own value for a public network and the
// previous default (the testnet value) for any other chain.
func DefaultEVMChainID(chainID string) uint64 {
	if network, known := KnownNetwork(chainID); known {
		return network.EVMChainID
	}
	return TestnetEVMChainID
}

// ValidateEVMChainID refuses a node-local EVM chain ID that differs from the
// value of a public network. Nodes with different EVM chain IDs would
// disagree on transaction validity.
func ValidateEVMChainID(chainID string, evmChainID uint64) error {
	network, known := KnownNetwork(chainID)
	if !known || network.EVMChainID == evmChainID {
		return nil
	}
	return fmt.Errorf(
		"%s (%s) requires evm.evm-chain-id = %d in app.toml, got %d",
		network.Name, network.CosmosChainID, network.EVMChainID, evmChainID,
	)
}
