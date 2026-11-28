package types

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/common"

	sdkmath "cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// HexID returns the canonical 0x-prefixed lowercase form of a 32-byte
// identifier (attestation ID, request ID, digest).
func HexID(id [32]byte) string {
	return "0x" + hex.EncodeToString(id[:])
}

// ParseRequestID accepts an outbound request ID as 32 hexadecimal bytes with
// or without 0x, in any letter case.
func ParseRequestID(value string) ([32]byte, string, error) {
	id, canonical, err := ParseAttestationID(value)
	if err != nil {
		return [32]byte{}, "", errors.New("request ID must contain 32 hexadecimal bytes")
	}
	return id, canonical, nil
}

// TxHash returns the hash CometBFT and the Cosmos SDK report for transaction
// bytes: SHA-256 in uppercase hex without 0x. It returns "" for no bytes
// (state written outside a transaction).
func TxHash(txBytes []byte) string {
	if len(txBytes) == 0 {
		return ""
	}
	sum := sha256.Sum256(txBytes)
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

func validTxHash(value string) bool {
	if value == "" {
		return true
	}
	if len(value) != 64 {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if (c < '0' || c > '9') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

// OutboundRequestID is the ID of an outbound transfer and the Cronos
// sourceBurnId: sha256(route ID || 0x00 || sender || 0x00 || lowercase
// 0x-prefixed destination || 0x00 || amount || 0x00 || nonce).
func OutboundRequestID(routeID, sender string, destination common.Address, amount sdkmath.Int, nonce uint64) [32]byte {
	payload := strings.Join([]string{routeID, sender, strings.ToLower(destination.Hex()), amount.String(), fmt.Sprintf("%d", nonce)}, "\x00")
	return sha256.Sum256([]byte(payload))
}

// SameSignerSet reports whether two configured signer lists contain the same
// addresses, whatever their order and spelling. Approvals verified against
// one set are valid against the other.
func SameSignerSet(current, next []string) bool {
	a, errA := configuredSigners(current)
	b, errB := configuredSigners(next)
	if errA != nil || errB != nil || len(a) != len(b) {
		return false
	}
	for address := range a {
		if _, ok := b[address]; !ok {
			return false
		}
	}
	return true
}

// NewAttestationRecord binds a valid attestation, verified on chainID by the
// given approvers of signer set signerSetVersion, to the block height and
// transaction hash that processed it.
func NewAttestationRecord(attestation Attestation, chainID string, signerSetVersion uint64, approvers []common.Address, height int64, txHash string) (AttestationRecord, error) {
	id, err := attestation.ID()
	if err != nil {
		return AttestationRecord{}, err
	}
	payloadHash, err := attestation.PayloadHash(chainID)
	if err != nil {
		return AttestationRecord{}, err
	}
	digest, err := SigningDigest(attestation, chainID)
	if err != nil {
		return AttestationRecord{}, err
	}
	approverText := make([]string, len(approvers))
	for i, approver := range approvers {
		approverText[i] = approver.Hex()
	}
	return AttestationRecord{
		AttestationId:      HexID(id),
		RouteId:            attestation.RouteID,
		Direction:          string(attestation.Direction),
		SourceChainId:      attestation.SourceChainID,
		SourceRef:          normalizeHexReference(attestation.SourceRef),
		Nonce:              attestation.Nonce,
		Recipient:          attestation.Destination,
		Amount:             attestation.Amount,
		DeadlineUnix:       attestation.DeadlineUnix,
		SignerSetVersion:   signerSetVersion,
		Approvers:          approverText,
		DestinationChainId: chainID,
		PayloadHash:        HexID(payloadHash),
		SigningDigest:      HexID(digest),
		Height:             height,
		TxHash:             txHash,
	}, nil
}

// Attestation returns the approved attestation fields of the record.
func (r AttestationRecord) Attestation() Attestation {
	return Attestation{
		RouteID:       r.RouteId,
		Direction:     Direction(r.Direction),
		SourceChainID: r.SourceChainId,
		SourceRef:     r.SourceRef,
		Nonce:         r.Nonce,
		Destination:   r.Recipient,
		Amount:        r.Amount,
		DeadlineUnix:  r.DeadlineUnix,
	}
}

// ID returns the attestation ID of a record whose ID field is canonical.
func (r AttestationRecord) ID() ([32]byte, error) {
	id, canonical, err := ParseAttestationID(r.AttestationId)
	if err != nil || canonical != r.AttestationId {
		return [32]byte{}, errors.New("attestation record ID must be canonical 0x-prefixed lowercase hex")
	}
	return id, nil
}

// Validate checks that every derived field of the record (attestation ID,
// payload hash, signing digest) matches its approved fields, so an imported
// record cannot bind an ID to other fields.
func (r AttestationRecord) Validate() error {
	id, err := r.ID()
	if err != nil {
		return err
	}
	if normalizeHexReference(r.SourceRef) != r.SourceRef {
		return errors.New("attestation record source reference must be lowercase hex without 0x")
	}
	attestation := r.Attestation()
	if attestation.Direction != DirectionCronosToXitcoin {
		return errors.New("attestation record must be an inbound Cronos transfer")
	}
	expected, err := NewAttestationRecord(attestation, r.DestinationChainId, r.SignerSetVersion, nil, r.Height, r.TxHash)
	if err != nil {
		return fmt.Errorf("invalid attestation record: %w", err)
	}
	if expected.AttestationId != HexID(id) {
		return errors.New("attestation record ID does not match its source deposit")
	}
	if expected.PayloadHash != r.PayloadHash || expected.SigningDigest != r.SigningDigest {
		return errors.New("attestation record digests do not match its fields")
	}
	if r.SignerSetVersion == 0 {
		return errors.New("attestation record signer set version must be positive")
	}
	if len(r.Approvers) < RequiredApprovals || len(r.Approvers) > MaxBridgeSigners {
		return fmt.Errorf("attestation record must list %d to %d approvers", RequiredApprovals, MaxBridgeSigners)
	}
	seen := make(map[string]struct{}, len(r.Approvers))
	for _, approver := range r.Approvers {
		if !common.IsHexAddress(approver) || common.HexToAddress(approver).Hex() != approver {
			return errors.New("attestation record approvers must be EIP-55 addresses")
		}
		if _, exists := seen[approver]; exists {
			return errors.New("duplicate attestation record approver")
		}
		seen[approver] = struct{}{}
	}
	if r.Height < 0 || !validTxHash(r.TxHash) {
		return errors.New("attestation record height or transaction hash is invalid")
	}
	return nil
}

// ID returns the request ID of a record whose ID field is canonical.
func (r OutboundTransferRecord) ID() ([32]byte, error) {
	id, canonical, err := ParseRequestID(r.RequestId)
	if err != nil || canonical != r.RequestId {
		return [32]byte{}, errors.New("outbound transfer request ID must be canonical 0x-prefixed lowercase hex")
	}
	return id, nil
}

// Validate checks that the request ID is the one derived from the record's
// fields and that every field is in its canonical form.
func (r OutboundTransferRecord) Validate() error {
	id, err := r.ID()
	if err != nil {
		return err
	}
	if !validRouteID(r.RouteId) {
		return errors.New("invalid outbound transfer route ID")
	}
	sender, err := sdk.AccAddressFromBech32(r.Sender)
	if err != nil || sender.String() != r.Sender {
		return errors.New("invalid outbound transfer sender")
	}
	destination, err := ValidateCronosDestination(r.Destination)
	if err != nil || destination.Hex() != r.Destination {
		return errors.New("outbound transfer destination must be a valid EIP-55 Cronos address")
	}
	amount, err := ParseAmount(r.Amount)
	if err != nil {
		return err
	}
	if r.Nonce == 0 {
		return errors.New("outbound transfer nonce must be positive")
	}
	if OutboundRequestID(r.RouteId, r.Sender, destination, amount, r.Nonce) != id {
		return errors.New("outbound transfer request ID does not match its fields")
	}
	if r.Height < 0 || !validTxHash(r.TxHash) {
		return errors.New("outbound transfer height or transaction hash is invalid")
	}
	return nil
}
