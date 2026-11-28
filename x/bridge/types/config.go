package types

import (
	"errors"

	"github.com/ethereum/go-ethereum/common"

	sdkmath "cosmossdk.io/math"
)

// RouteConfig is the route configuration. Changes after initialization are
// authorized through a 2-of-3 update_route_config control action.
//
// SourceChainID is the EVM chain ID of the Cronos network whose vault feeds
// this route ("25" for Cronos mainnet, "338" for Cronos Testnet). Inbound
// attestations must carry exactly this source chain ID. It is set at
// initialization (from genesis, or derived from the Xitcoin chain ID by
// MsgInitializeRouteConfig) and is preserved by every update. A stored
// configuration without it falls back to DefaultSourceChainID.
type RouteConfig struct {
	RouteID              string   `json:"route_id"`
	BridgeSigners        []string `json:"bridge_signers"`
	Guardian             string   `json:"guardian"`
	MaxTransferAmount    string   `json:"max_transfer_amount"`
	DailyLimit           string   `json:"daily_limit"`
	MaxOutstandingAmount string   `json:"max_outstanding_amount"`
	Enabled              bool     `json:"enabled"`
	SourceChainID        string   `json:"source_chain_id,omitempty"`
}

// EffectiveSourceChainID returns the Cronos chain ID that inbound
// attestations must carry on the Xitcoin chain identified by xitcoinChainID.
func (c RouteConfig) EffectiveSourceChainID(xitcoinChainID string) string {
	if c.SourceChainID != "" {
		return c.SourceChainID
	}
	return DefaultSourceChainID(xitcoinChainID)
}

func (c RouteConfig) Validate() error {
	if !validRouteID(c.RouteID) {
		return errors.New("invalid route ID")
	}
	if _, err := configuredSigners(c.BridgeSigners); err != nil {
		return err
	}
	// Addresses must be spelled exactly, without surrounding whitespace:
	// common.HexToAddress does not trim, so " 0x..." would otherwise pass
	// validation and then resolve to the zero address (or a nibble-shifted
	// one), silently disabling the guardian or one of the three signers.
	if !common.IsHexAddress(c.Guardian) {
		return errors.New("invalid bridge guardian")
	}
	guardian := common.HexToAddress(c.Guardian)
	for _, signer := range c.BridgeSigners {
		if guardian == common.HexToAddress(signer) {
			return errors.New("bridge guardian must use a distinct address")
		}
	}
	if c.SourceChainID != "" {
		if err := ValidateEVMChainID(c.SourceChainID); err != nil {
			return err
		}
	}
	if _, err := ParseAmount(c.MaxTransferAmount); err != nil {
		return errors.New("max transfer amount must be a canonical positive decimal integer")
	}
	if _, err := ParseAmount(c.DailyLimit); err != nil {
		return errors.New("daily limit must be a canonical positive decimal integer")
	}
	if _, err := ParseAmount(c.MaxOutstandingAmount); err != nil {
		return errors.New("max outstanding amount must be a canonical positive decimal integer")
	}
	return nil
}

// RouteLimits are the parsed limits of a valid route configuration.
type RouteLimits struct {
	MaxTransferAmount    sdkmath.Int
	DailyLimit           sdkmath.Int
	MaxOutstandingAmount sdkmath.Int
}

// Limits validates the configuration and parses its limits with ParseAmount,
// so no caller can read a limit with another parser or ignore a parse error.
func (c RouteConfig) Limits() (RouteLimits, error) {
	if err := c.Validate(); err != nil {
		return RouteLimits{}, err
	}
	maxTransfer, err := ParseAmount(c.MaxTransferAmount)
	if err != nil {
		return RouteLimits{}, err
	}
	dailyLimit, err := ParseAmount(c.DailyLimit)
	if err != nil {
		return RouteLimits{}, err
	}
	maxOutstanding, err := ParseAmount(c.MaxOutstandingAmount)
	if err != nil {
		return RouteLimits{}, err
	}
	return RouteLimits{MaxTransferAmount: maxTransfer, DailyLimit: dailyLimit, MaxOutstandingAmount: maxOutstanding}, nil
}
