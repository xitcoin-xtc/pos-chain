package types

import (
	"errors"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
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
	if !common.IsHexAddress(strings.TrimSpace(c.Guardian)) {
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
	if !positiveAmount(c.MaxTransferAmount) {
		return errors.New("max transfer amount must be positive")
	}
	if !positiveAmount(c.DailyLimit) {
		return errors.New("daily limit must be positive")
	}
	if !positiveAmount(c.MaxOutstandingAmount) {
		return errors.New("max outstanding amount must be positive")
	}
	return nil
}

func positiveAmount(value string) bool {
	amount, ok := new(big.Int).SetString(value, 10)
	return ok && amount.Sign() > 0
}
