package types

import (
	"errors"
)

// ProcessedSourceDeposits lists consumed source-deposit replay keys as
// canonical 0x-prefixed hex so that an exported and re-imported chain keeps
// its replay protection. ControlNonce is the last applied control nonce.
//
// SignerSetVersion is the version of the configured signer set (1 when
// omitted). DailyUsage is the most recent daily-limit bucket of the route, so
// an export and import on the same UTC day does not reset the daily limit.
// AttestationRecords and OutboundTransfers are the processing records that
// the queries return; every attestation record must belong to a listed
// processed source deposit.

// GenesisState deliberately contains no enabled default route. A route may be
// present only when it passes the same validation used at runtime.
type GenesisState struct {
	RouteConfig       *RouteConfig `json:"route_config,omitempty"`
	Paused            bool         `json:"paused"`
	OutstandingAmount string       `json:"outstanding_amount,omitempty"`
	OutboundNonce     uint64       `json:"outbound_nonce,omitempty"`
	ControlNonce      uint64       `json:"control_nonce,omitempty"`

	ProcessedSourceDeposits []string `json:"processed_source_deposits,omitempty"`

	SignerSetVersion   uint64                   `json:"signer_set_version,omitempty"`
	DailyUsage         *DailyUsage              `json:"daily_usage,omitempty"`
	AttestationRecords []AttestationRecord      `json:"attestation_records,omitempty"`
	OutboundTransfers  []OutboundTransferRecord `json:"outbound_transfers,omitempty"`
}

// DailyUsage is the usage of the route's daily limit on one UTC day: the
// number of whole days since 1970-01-01T00:00:00Z and the canonical decimal
// sum of the inbound mints and outbound burns of that day.
type DailyUsage struct {
	UTCDay uint64 `json:"utc_day"`
	Amount string `json:"amount"`
}

func DefaultGenesisState() GenesisState { return GenesisState{} }

func (g GenesisState) Validate() error {
	if g.RouteConfig == nil {
		if g.Paused || g.OutstandingAmount != "" || g.OutboundNonce != 0 || g.ControlNonce != 0 || len(g.ProcessedSourceDeposits) != 0 ||
			g.SignerSetVersion != 0 || g.DailyUsage != nil || len(g.AttestationRecords) != 0 || len(g.OutboundTransfers) != 0 {
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
	if err := g.validateRecords(seen); err != nil {
		return err
	}
	if g.DailyUsage != nil {
		if _, err := ParseAmount(g.DailyUsage.Amount); err != nil {
			return errors.New("bridge daily usage amount must be a canonical positive decimal integer")
		}
	}
	if g.OutstandingAmount == "" {
		return nil
	}
	outstanding, err := ParseNonNegativeAmount(g.OutstandingAmount)
	if err != nil {
		return errors.New("invalid bridge outstanding amount")
	}
	limits, err := g.RouteConfig.Limits()
	if err != nil {
		return err
	}
	if outstanding.GT(limits.MaxOutstandingAmount) {
		return errors.New("bridge outstanding amount exceeds route limit")
	}
	return nil
}

func (g GenesisState) validateRecords(processed map[string]struct{}) error {
	records := make(map[string]struct{}, len(g.AttestationRecords))
	for _, record := range g.AttestationRecords {
		if err := record.Validate(); err != nil {
			return err
		}
		if record.RouteId != g.RouteConfig.RouteID {
			return errors.New("attestation record belongs to another route")
		}
		if _, ok := processed[record.AttestationId]; !ok {
			return errors.New("attestation record without a processed source deposit")
		}
		if _, exists := records[record.AttestationId]; exists {
			return errors.New("duplicate attestation record")
		}
		records[record.AttestationId] = struct{}{}
	}
	requests := make(map[string]struct{}, len(g.OutboundTransfers))
	nonces := make(map[uint64]struct{}, len(g.OutboundTransfers))
	for _, transfer := range g.OutboundTransfers {
		if err := transfer.Validate(); err != nil {
			return err
		}
		if transfer.RouteId != g.RouteConfig.RouteID {
			return errors.New("outbound transfer belongs to another route")
		}
		if transfer.Nonce > g.OutboundNonce {
			return errors.New("outbound transfer nonce exceeds the outbound nonce")
		}
		if _, exists := requests[transfer.RequestId]; exists {
			return errors.New("duplicate outbound transfer")
		}
		if _, exists := nonces[transfer.Nonce]; exists {
			return errors.New("duplicate outbound transfer nonce")
		}
		requests[transfer.RequestId] = struct{}{}
		nonces[transfer.Nonce] = struct{}{}
	}
	return nil
}
