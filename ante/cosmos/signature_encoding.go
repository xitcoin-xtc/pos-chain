package cosmos

import (
	anteinterfaces "github.com/xitcoin-xtc/pos-chain/ante/interfaces"
	"github.com/xitcoin-xtc/pos-chain/crypto/ethsecp256k1"

	errorsmod "cosmossdk.io/errors"

	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	"github.com/cosmos/cosmos-sdk/crypto/types/multisig"
	sdk "github.com/cosmos/cosmos-sdk/types"
	errortypes "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	authante "github.com/cosmos/cosmos-sdk/x/auth/ante"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
)

// SignatureEncodingDecorator rejects a Cosmos transaction carrying an
// eth_secp256k1 signature, alone or as a multisig member, that is not
// 65 bytes R || S || V with V in {0, 1} or {27, 28}. Signature verification
// (ethsecp256k1.PubKey.VerifySignature) then accepts exactly one V for the
// signed content, so a signed transaction has a single valid encoding and a
// single hash: a 64-byte R || S copy or a copy with another V byte of a
// broadcast transaction can no longer be included in its place.
//
// The decorator reports the encoding error before signature verification,
// which would reject the transaction anyway, so that wallets see what to fix.
//
// CONTRACT: Pubkeys are set in context for all signers before this decorator
// runs (SetPubKeyDecorator).
type SignatureEncodingDecorator struct {
	ak anteinterfaces.AccountKeeper
}

// NewSignatureEncodingDecorator creates a new SignatureEncodingDecorator.
func NewSignatureEncodingDecorator(ak anteinterfaces.AccountKeeper) SignatureEncodingDecorator {
	return SignatureEncodingDecorator{ak: ak}
}

// AnteHandle implements sdk.AnteDecorator.
func (d SignatureEncodingDecorator) AnteHandle(ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler) (sdk.Context, error) {
	// Simulations carry empty signatures and are never verified.
	if simulate {
		return next(ctx, tx, simulate)
	}

	sigTx, ok := tx.(authsigning.SigVerifiableTx)
	if !ok {
		return ctx, errorsmod.Wrap(errortypes.ErrTxDecode, "invalid transaction type")
	}
	sigs, err := sigTx.GetSignaturesV2()
	if err != nil {
		return ctx, err
	}
	signers, err := sigTx.GetSigners()
	if err != nil {
		return ctx, err
	}
	if len(sigs) != len(signers) {
		return ctx, errorsmod.Wrapf(errortypes.ErrUnauthorized, "invalid number of signer;  expected: %d, got %d", len(signers), len(sigs))
	}

	for i, sig := range sigs {
		acc, err := authante.GetSignerAcc(ctx, d.ak, signers[i])
		if err != nil {
			return ctx, err
		}
		pubKey := acc.GetPubKey()
		if pubKey == nil {
			// Reported by signature verification.
			continue
		}
		if err := CheckSignatureEncoding(pubKey, sig.Data); err != nil {
			return ctx, errorsmod.Wrapf(errortypes.ErrUnauthorized, "signature %d: %s", i, err)
		}
	}

	return next(ctx, tx, simulate)
}

// CheckSignatureEncoding checks the encoding of every eth_secp256k1
// signature in data, including the members of a multisig and of nested
// multisigs. Structural mismatches between the key and the signature data
// are left to signature verification, which rejects them.
func CheckSignatureEncoding(pubKey cryptotypes.PubKey, data signing.SignatureData) error {
	switch pk := pubKey.(type) {
	case *ethsecp256k1.PubKey:
		single, ok := data.(*signing.SingleSignatureData)
		if !ok {
			return nil
		}
		return ethsecp256k1.ValidateSignatureEncoding(single.Signature)
	case multisig.PubKey:
		multi, ok := data.(*signing.MultiSignatureData)
		if !ok || multi.BitArray == nil {
			return nil
		}
		members := pk.GetPubKeys()
		if multi.BitArray.Count() != len(members) {
			return nil
		}
		next := 0
		for i, member := range members {
			if !multi.BitArray.GetIndex(i) {
				continue
			}
			if next >= len(multi.Signatures) {
				return nil
			}
			if err := CheckSignatureEncoding(member, multi.Signatures[next]); err != nil {
				return errorsmod.Wrapf(err, "multisig member %d", i)
			}
			next++
		}
	}
	return nil
}
