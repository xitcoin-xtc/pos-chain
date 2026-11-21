package types

import (
	"github.com/ethereum/go-ethereum/crypto"
	"testing"
)

func validGuardianPause() GuardianPauseAction {
	return GuardianPauseAction{RouteID: "cronos-testnet-xitcoin-testnet", Nonce: 1, ExpiresUnix: 1800003600}
}

func TestVerifyGuardianPause(t *testing.T) {
	guardian, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	digest, err := GuardianPauseDigest(validGuardianPause(), testChainID)
	if err != nil {
		t.Fatal(err)
	}
	signature, err := crypto.Sign(digest.Bytes(), guardian)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyGuardianPause(validGuardianPause(), testChainID, crypto.PubkeyToAddress(guardian.PublicKey).Hex(), signature); err != nil {
		t.Fatalf("valid guardian pause rejected: %v", err)
	}
}

func TestVerifyGuardianPauseRejectsOtherSigner(t *testing.T) {
	guardian, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	other, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	digest, err := GuardianPauseDigest(validGuardianPause(), testChainID)
	if err != nil {
		t.Fatal(err)
	}
	signature, err := crypto.Sign(digest.Bytes(), other)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyGuardianPause(validGuardianPause(), testChainID, crypto.PubkeyToAddress(guardian.PublicKey).Hex(), signature); err == nil {
		t.Fatal("non-guardian pause was accepted")
	}
}
