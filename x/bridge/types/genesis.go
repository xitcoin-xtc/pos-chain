package types

import (
	"errors"
	"math"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

// MaxSignerSetVersion is the largest valid signer set version. The next
// version must never wrap to 0 or reach a value that genesis validation
// rejects, so a rotation from MaxSignerSetVersion fails.
const MaxSignerSetVersion uint64 = math.MaxUint64 - 1

// ErrSignerSetVersionExhausted rejects a signer rotation once the signer set
// version has reached MaxSignerSetVersion.
var ErrSignerSetVersionExhausted = errors.New("bridge signer set version cannot be increased")

// NextSignerSetVersion returns the version after current, or an error when
// it would exceed MaxSignerSetVersion.
func NextSignerSetVersion(current uint64) (uint64, error) {
	if current >= MaxSignerSetVersion {
		return 0, ErrSignerSetVersionExhausted
	}
	return current + 1, nil
}

// UTCDay returns the number of whole days between 1970-01-01T00:00:00Z and a
// Unix time: the day of the daily-limit bucket. Block times are later than
// 1970; an earlier time maps to day 0.
func UTCDay(unixTime int64) uint64 {
	day := unixTime / 86400
	if day < 0 {
		return 0
	}
	return uint64(day)
}

// ProcessedSourceDeposits lists consumed source-deposit replay keys as
// canonical 0x-prefixed hex so that an exported and re-imported chain keeps
// its replay protection. ControlNonce is the last applied control nonce.
//
// SignerSetVersion is the version of the configured signer set (1 when
// omitted). DailyUsage is the most recent daily-limit bucket of the route, so
// an export and import on the same UTC day does not reset the daily limit.
// AttestationRecords and OutboundTransfers are the processing records that
// the queries return; every attestation record must belong to a listed
// processed source deposit. Records of the current signer set version must
// list only configured bridge signers as approvers; records of an earlier
// version were verified against a signer set the genesis no longer contains.
// ValidateForChain adds the checks that need the chain ID and genesis time.

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
	if g.SignerSetVersion > MaxSignerSetVersion {
		return errors.New("bridge signer set version exceeds its maximum")
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

// signerSetVersion is the version InitGenesis stores: 1 when omitted.
func (g GenesisState) signerSetVersion() uint64 {
	if g.SignerSetVersion == 0 {
		return 1
	}
	return g.SignerSetVersion
}

func (g GenesisState) validateRecords(processed map[string]struct{}) error {
	signers, err := configuredSigners(g.RouteConfig.BridgeSigners)
	if err != nil {
		return err
	}
	version := g.signerSetVersion()
	records := make(map[string]struct{}, len(g.AttestationRecords))
	for _, record := range g.AttestationRecords {
		if err := record.Validate(); err != nil {
			return err
		}
		if record.RouteId != g.RouteConfig.RouteID {
			return errors.New("attestation record belongs to another route")
		}
		if g.RouteConfig.SourceChainID != "" && record.SourceChainId != g.RouteConfig.SourceChainID {
			return errors.New("attestation record source chain does not match the route")
		}
		if record.SignerSetVersion > version {
			return errors.New("attestation record signer set version exceeds the bridge signer set version")
		}
		if record.SignerSetVersion == version {
			for _, approver := range record.Approvers {
				if _, ok := signers[common.HexToAddress(approver)]; !ok {
					return errors.New("attestation record approver is not a configured bridge signer")
				}
			}
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

// ValidateForChain checks the rules that depend on the chain that imports the
// genesis: every attestation record was processed on chainID from the route's
// source chain, and the daily-usage bucket is not later than the UTC day of
// genesisTime. A record therefore cannot be carried to a chain with another
// chain ID; such a record must be dropped, which leaves its replay key
// reported as processed without a record.
func (g GenesisState) ValidateForChain(chainID string, genesisTime time.Time) error {
	if g.RouteConfig == nil {
		return nil
	}
	sourceChainID := g.RouteConfig.EffectiveSourceChainID(chainID)
	for _, record := range g.AttestationRecords {
		if record.DestinationChainId != chainID {
			return errors.New("attestation record was processed on another chain")
		}
		if record.SourceChainId != sourceChainID {
			return errors.New("attestation record source chain does not match the route")
		}
	}
	if g.DailyUsage != nil && g.DailyUsage.UTCDay > UTCDay(genesisTime.Unix()) {
		return errors.New("bridge daily usage is later than the genesis time")
	}
	return nil
}
