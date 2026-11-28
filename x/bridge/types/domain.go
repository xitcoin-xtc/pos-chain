package types

import (
	"strings"

	errorsmod "cosmossdk.io/errors"
)

// Signing domains are derived from the Xitcoin chain ID of the chain that
// verifies the signature, so an approval produced for one network (for
// example xitcoin-testnet-v2-1) can never be replayed on another (xitcoin).
//
// Version history:
//   - v1 used the fixed strings xitcoin-bridge-testnet-attestation-v1,
//     xitcoin-bridge-testnet-control-v1 and xitcoin-bridge-testnet-guardian-pause-v1
//     on every network and did not bind the Xitcoin chain ID.
//   - v2 appends "/<xitcoin chain id>" to a network-neutral prefix.
const (
	AttestationDomainPrefix   = "xitcoin-bridge-attestation-v2"
	ControlDomainPrefix       = "xitcoin-bridge-control-v2"
	GuardianPauseDomainPrefix = "xitcoin-bridge-guardian-pause-v2"

	// MainnetChainID is the Cosmos chain ID of Xitcoin mainnet.
	MainnetChainID = "xitcoin"
	// CronosMainnetChainID and CronosTestnetChainID are EVM chain IDs.
	CronosMainnetChainID = "25"
	CronosTestnetChainID = "338"

	maxChainIDLength = 64
)

// ValidateChainID accepts a non-empty chain identifier without whitespace or
// separator bytes, so it can be embedded unambiguously in a signed payload.
func ValidateChainID(chainID string) error {
	if chainID == "" || len(chainID) > maxChainIDLength {
		return errorsmod.Wrap(ErrInvalidChainID, "bridge chain ID must contain 1 to 64 characters")
	}
	if strings.TrimSpace(chainID) != chainID || strings.ContainsAny(chainID, "\x00/ \t\r\n") {
		return errorsmod.Wrap(ErrInvalidChainID, "bridge chain ID contains a forbidden character")
	}
	return nil
}

// ValidateEVMChainID accepts the canonical decimal form of an EVM chain ID.
func ValidateEVMChainID(chainID string) error {
	if chainID == "" || len(chainID) > 20 || chainID[0] == '0' {
		return errorsmod.Wrap(ErrInvalidChainID, "source chain ID must be a canonical decimal EVM chain ID")
	}
	for _, r := range chainID {
		if r < '0' || r > '9' {
			return errorsmod.Wrap(ErrInvalidChainID, "source chain ID must be a canonical decimal EVM chain ID")
		}
	}
	return nil
}

func networkDomain(prefix, chainID string) (string, error) {
	if err := ValidateChainID(chainID); err != nil {
		return "", err
	}
	return prefix + "/" + chainID, nil
}

// AttestationDomain returns the transfer-approval domain for one network.
func AttestationDomain(chainID string) (string, error) {
	return networkDomain(AttestationDomainPrefix, chainID)
}

// ControlDomain returns the route-governance domain for one network.
func ControlDomain(chainID string) (string, error) {
	return networkDomain(ControlDomainPrefix, chainID)
}

// GuardianPauseDomain returns the guardian-pause domain for one network.
func GuardianPauseDomain(chainID string) (string, error) {
	return networkDomain(GuardianPauseDomainPrefix, chainID)
}

// DefaultSourceChainID is used only when a stored route configuration does
// not name its source chain (configurations written before source_chain_id
// existed). Xitcoin mainnet pairs with Cronos mainnet; every other network
// pairs with Cronos Testnet.
func DefaultSourceChainID(xitcoinChainID string) string {
	if xitcoinChainID == MainnetChainID {
		return CronosMainnetChainID
	}
	return CronosTestnetChainID
}
