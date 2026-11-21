package types

import (
	"crypto/ecdsa"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

func TestVerifyApprovalsRequiresTwoConfiguredSigners(t *testing.T) {
	keys := make([]*ecdsa.PrivateKey, 3)
	signers := make([]string, 3)
	for i := range keys {
		key, err := crypto.GenerateKey()
		if err != nil {
			t.Fatal(err)
		}
		keys[i] = key
		signers[i] = crypto.PubkeyToAddress(key.PublicKey).Hex()
	}
	digest, err := SigningDigest(validAttestation(), testChainID)
	if err != nil {
		t.Fatal(err)
	}
	first, err := crypto.Sign(digest.Bytes(), keys[0])
	if err != nil {
		t.Fatal(err)
	}
	second, err := crypto.Sign(digest.Bytes(), keys[1])
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := VerifyApprovals(validAttestation(), testChainID, signers, [][]byte{first, second})
	if err != nil {
		t.Fatalf("expected valid two-of-three approvals: %v", err)
	}
	if len(recovered) != 2 {
		t.Fatalf("got %d recovered signers", len(recovered))
	}
}

func TestVerifyApprovalsRejectsDuplicateAndUnauthorized(t *testing.T) {
	keys := make([]*ecdsa.PrivateKey, 4)
	signers := make([]string, 3)
	for i := range keys {
		key, err := crypto.GenerateKey()
		if err != nil {
			t.Fatal(err)
		}
		keys[i] = key
		if i < 3 {
			signers[i] = crypto.PubkeyToAddress(key.PublicKey).Hex()
		}
	}
	digest, err := SigningDigest(validAttestation(), testChainID)
	if err != nil {
		t.Fatal(err)
	}
	first, err := crypto.Sign(digest.Bytes(), keys[0])
	if err != nil {
		t.Fatal(err)
	}
	outsider, err := crypto.Sign(digest.Bytes(), keys[3])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyApprovals(validAttestation(), testChainID, signers, [][]byte{first, first}); err == nil {
		t.Fatal("duplicate signer must be rejected")
	}
	if _, err := VerifyApprovals(validAttestation(), testChainID, signers, [][]byte{first, outsider}); err == nil {
		t.Fatal("unauthorized signer must be rejected")
	}
}
