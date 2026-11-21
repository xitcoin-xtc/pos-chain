// Package types contains bridge admission and settlement primitives.
package types

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"unicode"
)

const (
	MaxBridgeSigners  = 3
	RequiredApprovals = 2
	maxRouteIDLength  = 96
	maxDestinationLen = 256
)

type Direction string

const (
	DirectionCronosToXitcoin Direction = "cronos_to_xitcoin"
	DirectionXitcoinToCronos Direction = "xitcoin_to_cronos"
)

// SourceDepositDomain separates source-deposit replay keys from every other
// hash used by the bridge.
const SourceDepositDomain = "xitcoin-bridge-source-deposit-v1"

// Attestation binds one source-chain event to a destination and amount.
// Amount uses atomic units.
//
// ID is the replay-protection key and identifies only the source deposit
// (source chain ID and source reference, i.e. the Cronos vault depositId,
// which itself commits to the vault, route, depositor, recipient, amount and
// deposit nonce). It deliberately excludes the deadline and every other
// signed field, so two approvals for one deposit share one replay key.
// SigningDigest commits to every field plus the destination chain ID.
type Attestation struct {
	RouteID       string
	Direction     Direction
	SourceChainID string
	SourceRef     string
	Nonce         uint64
	Destination   string
	Amount        string
	DeadlineUnix  int64
}

func (a Attestation) Validate() error {
	if !validRouteID(a.RouteID) {
		return errors.New("invalid route ID")
	}
	if a.Direction != DirectionCronosToXitcoin && a.Direction != DirectionXitcoinToCronos {
		return errors.New("invalid bridge direction")
	}
	if err := ValidateEVMChainID(a.SourceChainID); err != nil {
		return err
	}
	if !validHexReference(a.SourceRef) {
		return errors.New("source reference must be a 32-byte hexadecimal transaction identifier")
	}
	if a.Nonce == 0 {
		return errors.New("nonce must be greater than zero")
	}
	if destination := strings.TrimSpace(a.Destination); destination == "" || len(destination) > maxDestinationLen {
		return errors.New("invalid destination")
	}
	amount, ok := new(big.Int).SetString(a.Amount, 10)
	if !ok || amount.Sign() <= 0 {
		return errors.New("amount must be a positive integer in atomic units")
	}
	if a.DeadlineUnix <= 0 {
		return errors.New("deadline is required")
	}
	return nil
}

// ID returns the source-deposit replay key, only for a valid attestation.
func (a Attestation) ID() ([32]byte, error) {
	if err := a.Validate(); err != nil {
		return [32]byte{}, err
	}
	return SourceDepositKey(a.SourceChainID, a.SourceRef)
}

// SourceDepositKey is sha256(SourceDepositDomain || 0x00 || sourceChainID ||
// 0x00 || lowercase 64-hex source reference without 0x).
func SourceDepositKey(sourceChainID, sourceRef string) ([32]byte, error) {
	if err := ValidateEVMChainID(sourceChainID); err != nil {
		return [32]byte{}, err
	}
	if !validHexReference(sourceRef) {
		return [32]byte{}, errors.New("source reference must be a 32-byte hexadecimal transaction identifier")
	}
	fields := []string{SourceDepositDomain, sourceChainID, normalizeHexReference(sourceRef)}
	return sha256.Sum256([]byte(strings.Join(fields, "\x00"))), nil
}

// PayloadHash commits to every attestation field and to the destination
// (Xitcoin) chain ID: sha256 of the fields joined with 0x00 in this order:
// destination chain ID, route ID, direction, source chain ID, source
// reference (lowercase hex, no 0x), nonce, destination, amount, deadline.
func (a Attestation) PayloadHash(destinationChainID string) ([32]byte, error) {
	if err := a.Validate(); err != nil {
		return [32]byte{}, err
	}
	if err := ValidateChainID(destinationChainID); err != nil {
		return [32]byte{}, err
	}
	fields := []string{
		destinationChainID, a.RouteID, string(a.Direction), a.SourceChainID, normalizeHexReference(a.SourceRef),
		fmt.Sprintf("%d", a.Nonce), strings.TrimSpace(a.Destination), a.Amount, fmt.Sprintf("%d", a.DeadlineUnix),
	}
	return sha256.Sum256([]byte(strings.Join(fields, "\x00"))), nil
}

// ValidateApprovalSet enforces distinct 2-of-3 approvals before the later
// cryptographic verifier validates the actual signatures.
func ValidateApprovalSet(signerIDs []string) error {
	if len(signerIDs) < RequiredApprovals || len(signerIDs) > MaxBridgeSigners {
		return fmt.Errorf("approval count must be between %d and %d", RequiredApprovals, MaxBridgeSigners)
	}
	seen := make(map[string]struct{}, len(signerIDs))
	for _, signerID := range signerIDs {
		normalized := strings.ToLower(strings.TrimSpace(signerID))
		if normalized == "" || len(normalized) > 128 {
			return errors.New("invalid signer ID")
		}
		if _, exists := seen[normalized]; exists {
			return errors.New("duplicate signer approval")
		}
		seen[normalized] = struct{}{}
	}
	return nil
}

func validRouteID(routeID string) bool {
	if routeID == "" || len(routeID) > maxRouteIDLength {
		return false
	}
	for _, r := range routeID {
		if !(unicode.IsLower(r) || unicode.IsDigit(r) || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

func validHexReference(reference string) bool {
	normalized := normalizeHexReference(reference)
	if len(normalized) != 64 {
		return false
	}
	_, err := hex.DecodeString(normalized)
	return err == nil
}

func normalizeHexReference(reference string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(reference)), "0x")
}
