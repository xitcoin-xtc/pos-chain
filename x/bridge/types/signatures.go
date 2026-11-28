package types

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	errorsmod "cosmossdk.io/errors"
)

// SigningDigest is keccak256(AttestationDomain(chainID) || PayloadHash(chainID)).
// chainID is the Xitcoin chain ID of the destination chain that verifies the
// approval, so the digest is network-specific twice: through the domain and
// through the destination chain ID inside the payload.
func SigningDigest(attestation Attestation, chainID string) (common.Hash, error) {
	domain, err := AttestationDomain(chainID)
	if err != nil {
		return common.Hash{}, err
	}
	payloadHash, err := attestation.PayloadHash(chainID)
	if err != nil {
		return common.Hash{}, err
	}
	return crypto.Keccak256Hash([]byte(domain), payloadHash[:]), nil
}

// VerifyApprovals recovers signer addresses and requires two distinct members
// of the configured three-member bridge signer set.
func VerifyApprovals(attestation Attestation, chainID string, bridgeSigners []string, signatures [][]byte) ([]common.Address, error) {
	allowed, err := configuredSigners(bridgeSigners)
	if err != nil {
		return nil, err
	}
	if len(signatures) < RequiredApprovals || len(signatures) > MaxBridgeSigners {
		return nil, errorsmod.Wrapf(ErrInvalidApprovals, "signature count must be between %d and %d", RequiredApprovals, MaxBridgeSigners)
	}
	digest, err := SigningDigest(attestation, chainID)
	if err != nil {
		return nil, err
	}

	recovered := make([]common.Address, 0, len(signatures))
	seen := make(map[common.Address]struct{}, len(signatures))
	for _, signature := range signatures {
		address, err := recoverSigner(digest, signature)
		if err != nil {
			return nil, err
		}
		if _, ok := allowed[address]; !ok {
			return nil, errorsmod.Wrap(ErrInvalidApprovals, "approval is not from a configured bridge signer")
		}
		if _, exists := seen[address]; exists {
			return nil, errorsmod.Wrap(ErrInvalidApprovals, "duplicate signer approval")
		}
		seen[address] = struct{}{}
		recovered = append(recovered, address)
	}
	return recovered, nil
}

func configuredSigners(signers []string) (map[common.Address]struct{}, error) {
	if len(signers) != MaxBridgeSigners {
		return nil, errorsmod.Wrapf(ErrInvalidRouteConfig, "exactly %d bridge signers are required", MaxBridgeSigners)
	}
	allowed := make(map[common.Address]struct{}, MaxBridgeSigners)
	for _, signer := range signers {
		if !common.IsHexAddress(signer) {
			return nil, errorsmod.Wrap(ErrInvalidRouteConfig, "invalid configured bridge signer")
		}
		address := common.HexToAddress(signer)
		if _, exists := allowed[address]; exists {
			return nil, errorsmod.Wrap(ErrInvalidRouteConfig, "duplicate configured bridge signer")
		}
		allowed[address] = struct{}{}
	}
	return allowed, nil
}

func recoverSigner(digest common.Hash, signature []byte) (common.Address, error) {
	if len(signature) != crypto.SignatureLength {
		return common.Address{}, errorsmod.Wrap(ErrInvalidSignature, "invalid signature length")
	}
	sig := append([]byte(nil), signature...)
	if sig[64] == 27 || sig[64] == 28 {
		sig[64] -= 27
	}
	if sig[64] > 1 {
		return common.Address{}, errorsmod.Wrap(ErrInvalidSignature, "invalid signature recovery ID")
	}
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:64])
	if !crypto.ValidateSignatureValues(sig[64], r, s, true) {
		return common.Address{}, errorsmod.Wrap(ErrInvalidSignature, "invalid signature values")
	}
	pub, err := crypto.SigToPub(digest.Bytes(), sig)
	if err != nil {
		return common.Address{}, errorsmod.Wrapf(ErrInvalidSignature, "recover signer: %s", err)
	}
	return crypto.PubkeyToAddress(*pub), nil
}
