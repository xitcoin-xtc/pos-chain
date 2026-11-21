package types

import (
	"crypto/ecdsa"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

func validControlAction() ControlAction {
	return ControlAction{
		RouteID:       "cronos-testnet-xitcoin-testnet",
		Action:        ActionUpdateRouteConfig,
		PayloadHash:   "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Nonce:         1,
		NotBeforeUnix: 1800000000,
		ExpiresUnix:   1800003600,
	}
}

func TestVerifyControlApprovalsRequiresTwoConfiguredSigners(t *testing.T) {
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
	digest, err := ControlDigest(validControlAction(), testChainID)
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
	if _, err := VerifyControlApprovals(validControlAction(), testChainID, signers, [][]byte{first, second}); err != nil {
		t.Fatalf("expected valid governance approvals: %v", err)
	}
}

func TestControlActionRejectsInvalidWindowAndDuplicateSigner(t *testing.T) {
	action := validControlAction()
	action.ExpiresUnix = action.NotBeforeUnix
	if err := action.Validate(); err == nil {
		t.Fatal("invalid control validity window was accepted")
	}

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
	digest, err := ControlDigest(validControlAction(), testChainID)
	if err != nil {
		t.Fatal(err)
	}
	signature, err := crypto.Sign(digest.Bytes(), keys[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyControlApprovals(validControlAction(), testChainID, signers, [][]byte{signature, signature}); err == nil {
		t.Fatal("duplicate signer approval was accepted")
	}
}
