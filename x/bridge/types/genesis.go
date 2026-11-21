package types

import (
	"errors"
	"math/big"
)

// ProcessedSourceDeposits lists consumed source-deposit replay keys as
// canonical 0x-prefixed hex so that an exported and re-imported chain keeps
// its replay protection. ControlNonce is the last applied control nonce.

// GenesisState deliberately contains no enabled default route. A route may be
// present only when it passes the same validation used at runtime.
type GenesisState struct {
	RouteConfig       *RouteConfig `json:"route_config,omitempty"`
	Paused            bool         `json:"paused"`
	OutstandingAmount string       `json:"outstanding_amount,omitempty"`
	OutboundNonce     uint64       `json:"outbound_nonce,omitempty"`
	ControlNonce      uint64       `json:"control_nonce,omitempty"`

	ProcessedSourceDeposits []string `json:"processed_source_deposits,omitempty"`
}

func DefaultGenesisState() GenesisState { return GenesisState{} }

func (g GenesisState) Validate() error {
	if g.RouteConfig == nil {
		if g.Paused || g.OutstandingAmount != "" || g.OutboundNonce != 0 || g.ControlNonce != 0 || len(g.ProcessedSourceDeposits) != 0 {
			return errors.New("bridge state requires a route configuration")
		}
		return nil
	}
	if err := g.RouteConfig.Validate(); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(g.ProcessedSourceDeposits))
	for _, value := range g.ProcessedSourceDeposits {
		_, canonical, err := ParseAttestationID(value)
		if err != nil || canonical != value {
			return errors.New("processed source deposit keys must be canonical 0x-prefixed lowercase hex")
		}
		if _, exists := seen[canonical]; exists {
			return errors.New("duplicate processed source deposit key")
		}
		seen[canonical] = struct{}{}
	}
	if g.OutstandingAmount == "" {
		return nil
	}
	outstanding, ok := new(big.Int).SetString(g.OutstandingAmount, 10)
	if !ok || outstanding.Sign() < 0 {
		return errors.New("invalid bridge outstanding amount")
	}
	limit, _ := new(big.Int).SetString(g.RouteConfig.MaxOutstandingAmount, 10)
	if outstanding.Cmp(limit) > 0 {
		return errors.New("bridge outstanding amount exceeds route limit")
	}
	return nil
}
