package evmd

import (
	"encoding/json"
	"fmt"
	"sync/atomic"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/xitcoin-xtc/pos-chain/evmd/config"
)

// TestnetAdministrativeMultisigAddress is the public 2-of-3 administrative
// authority of Xitcoin Testnet. Its private keys are never embedded in the
// application.
const TestnetAdministrativeMultisigAddress = config.TestnetAdministrativeMultisigAddress

// mustAdministrativeAuthority returns the administrative authority of the
// network identified by chainID (see config.AdministrativeAuthority).
func mustAdministrativeAuthority(chainID string) string {
	authority, err := config.AdministrativeAuthority(chainID)
	if err != nil {
		panic(err)
	}
	address, err := sdk.AccAddressFromBech32(authority)
	if err != nil {
		panic(fmt.Errorf("invalid administrative authority for chain %q: %w", chainID, err))
	}
	return address.String()
}

// mustTestnetAdministrativeAuthority is used by the historical testnet
// upgrade handler xitcoin-governance-safeguards-v1.
func mustTestnetAdministrativeAuthority() string {
	return mustAdministrativeAuthority(config.TestnetCosmosChainID)
}

// governanceProposalSubmissionForTests re-enables executable governance
// proposals for legacy integration suites (see
// EnableGovernanceProposalSubmissionForTests). Release binaries never set it,
// so proposal submission stays disabled on every node.
var governanceProposalSubmissionForTests atomic.Bool

func governanceProposalSubmissionEnabled() bool {
	return governanceProposalSubmissionForTests.Load()
}

// validateNetworkAuthorities refuses a mainnet genesis that keeps the testnet
// administrative multisig as a module authority. scripts/verify-xitcoin-
// mainnet-genesis.py applies the same rule before a genesis is published.
func validateNetworkAuthorities(chainID string, genesisState GenesisState) error {
	if chainID != config.MainnetCosmosChainID {
		return nil
	}
	for _, module := range []string{"validator_admission", "validator_incentives", "bridge"} {
		raw, ok := genesisState[module]
		if !ok {
			continue
		}
		var state struct {
			Authority string `json:"authority"`
		}
		if err := json.Unmarshal(raw, &state); err != nil {
			return fmt.Errorf("%s genesis: %w", module, err)
		}
		if state.Authority == config.TestnetAdministrativeMultisigAddress {
			return fmt.Errorf("%s genesis must not use the testnet administrative multisig on %s", module, chainID)
		}
	}
	return nil
}
