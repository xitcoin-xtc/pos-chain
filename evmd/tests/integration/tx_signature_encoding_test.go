package integration

import (
	"context"
	"crypto/sha256"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"

	abci "github.com/cometbft/cometbft/abci/types"

	"github.com/xitcoin-xtc/pos-chain/crypto/ethsecp256k1"
	"github.com/xitcoin-xtc/pos-chain/ethereum/eip712"
	"github.com/xitcoin-xtc/pos-chain/testutil/integration/evm/grpc"
	"github.com/xitcoin-xtc/pos-chain/testutil/integration/evm/network"
	testkeyring "github.com/xitcoin-xtc/pos-chain/testutil/keyring"

	sdkmath "cosmossdk.io/math"

	"github.com/cosmos/cosmos-sdk/client"
	kmultisig "github.com/cosmos/cosmos-sdk/crypto/keys/multisig"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	"github.com/cosmos/cosmos-sdk/crypto/types/multisig"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
)

const signatureTestGas = 300_000

// signatureTestChain is a local chain whose prefunded accounts are the
// keyring keys and the extra addresses.
type signatureTestChain struct {
	nw      *network.UnitTestNetwork
	handler grpc.Handler
	keyring testkeyring.Keyring
}

func newSignatureTestChain(t *testing.T, keyring testkeyring.Keyring, extra ...sdk.AccAddress) signatureTestChain {
	t.Helper()
	nw := network.NewUnitTestNetwork(CreateEvmd, network.WithPreFundedAccounts(append(keyring.GetAllAccAddrs(), extra...)...))
	return signatureTestChain{nw: nw, handler: grpc.NewIntegrationHandler(nw), keyring: keyring}
}

// unsignedSend returns a bank send from `from` with an empty signature of the
// given mode, and the signer data to sign it with.
func (c signatureTestChain) unsignedSend(t *testing.T, from sdk.AccAddress, pubKey cryptotypes.PubKey, mode signing.SignMode) (client.TxBuilder, authsigning.SignerData) {
	t.Helper()
	account, err := c.handler.GetAccount(from.String())
	require.NoError(t, err)
	baseFee, err := c.handler.GetBaseFee()
	require.NoError(t, err)

	builder := c.nw.GetEncodingConfig().TxConfig.NewTxBuilder()
	require.NoError(t, builder.SetMsgs(&banktypes.MsgSend{
		FromAddress: from.String(), ToAddress: c.keyring.GetAccAddr(0).String(),
		Amount: sdk.NewCoins(sdk.NewCoin(c.nw.GetBaseDenom(), sdkmath.NewInt(1000))),
	}))
	builder.SetGasLimit(signatureTestGas)
	builder.SetFeeAmount(sdk.NewCoins(sdk.NewCoin(c.nw.GetBaseDenom(), baseFee.BaseFee.MulInt64(signatureTestGas).Ceil().RoundInt())))

	var data signing.SignatureData = &signing.SingleSignatureData{SignMode: mode}
	if multisigKey, ok := pubKey.(*kmultisig.LegacyAminoPubKey); ok {
		data = multisig.NewMultisig(len(multisigKey.GetPubKeys()))
	}
	require.NoError(t, builder.SetSignatures(signing.SignatureV2{PubKey: pubKey, Data: data, Sequence: account.GetSequence()}))
	return builder, authsigning.SignerData{
		ChainID: c.nw.GetChainID(), AccountNumber: account.GetAccountNumber(), Sequence: account.GetSequence(),
		Address: from.String(), PubKey: pubKey,
	}
}

func (c signatureTestChain) signBytes(t *testing.T, builder client.TxBuilder, signerData authsigning.SignerData, mode signing.SignMode) []byte {
	t.Helper()
	bz, err := authsigning.GetSignBytesAdapter(context.Background(), c.nw.GetEncodingConfig().TxConfig.SignModeHandler(), mode, signerData, builder.GetTx())
	require.NoError(t, err)
	return bz
}

func (c signatureTestChain) encode(t *testing.T, builder client.TxBuilder) []byte {
	t.Helper()
	bz, err := c.nw.GetEncodingConfig().TxConfig.TxEncoder()(builder.GetTx())
	require.NoError(t, err)
	return bz
}

// keyringSignature signs as `xitcoind tx sign` does: V is 0 or 1.
func keyringSignature(t *testing.T, privKey cryptotypes.PrivKey, signBytes []byte) []byte {
	t.Helper()
	sig, err := privKey.Sign(signBytes)
	require.NoError(t, err)
	return sig
}

// typedDataSignature signs the EIP-712 representation of the sign bytes as
// eth_signTypedData_v4 (MetaMask) and the Ledger Ethereum app do: V is 27 or
// 28.
func typedDataSignature(t *testing.T, privKey cryptotypes.PrivKey, signBytes []byte) []byte {
	t.Helper()
	eip712Bytes, err := eip712.GetEIP712BytesForMsg(signBytes)
	require.NoError(t, err)
	sig, err := privKey.Sign(eip712Bytes)
	require.NoError(t, err)
	sig[crypto.RecoveryIDOffset] += 27
	return sig
}

func setSingleSignature(t *testing.T, builder client.TxBuilder, signerData authsigning.SignerData, mode signing.SignMode, sig []byte) {
	t.Helper()
	require.NoError(t, builder.SetSignatures(signing.SignatureV2{
		PubKey: signerData.PubKey, Data: &signing.SingleSignatureData{SignMode: mode, Signature: sig}, Sequence: signerData.Sequence,
	}))
}

// withSignature re-wraps the signed transaction bytes with another
// signature, which is what anyone holding the broadcast transaction can do.
func withSignature(t *testing.T, txBytes, sig []byte) []byte {
	t.Helper()
	var raw txtypes.TxRaw
	require.NoError(t, raw.Unmarshal(txBytes))
	raw.Signatures = [][]byte{sig}
	bz, err := raw.Marshal()
	require.NoError(t, err)
	return bz
}

func withV(sig []byte, v byte) []byte {
	return append(append([]byte{}, sig[:64]...), v)
}

func requireLands(t *testing.T, c signatureTestChain, txBytes []byte) {
	t.Helper()
	res, err := c.nw.CheckTx(txBytes)
	require.NoError(t, err)
	require.Zero(t, res.Code, res.Log)
	block, err := c.nw.NextBlockWithTxs(txBytes)
	require.NoError(t, err)
	require.Len(t, block.TxResults, 1)
	require.Zero(t, block.TxResults[0].Code, block.TxResults[0].Log)
}

func requireUnauthorized(t *testing.T, res *abci.ResponseCheckTx, contains string) {
	t.Helper()
	require.Equal(t, sdkerrors.ErrUnauthorized.ABCICode(), res.Code, res.Log)
	require.Equal(t, sdkerrors.ErrUnauthorized.Codespace(), res.Codespace)
	require.Contains(t, res.Log, contains)
}

// TestReencodedSignatureCannotReplaceTheSignedTransaction reproduces the
// signature malleability of eth_secp256k1 Cosmos transactions and checks the
// fix: before it, every re-encoding below passed CheckTx on a node that had
// not seen the original, and the first one included in a block executed in
// place of the original (with another transaction hash, paid by the signer)
// while the original failed with an account sequence mismatch.
func TestReencodedSignatureCannotReplaceTheSignedTransaction(t *testing.T) {
	keyring := testkeyring.New(2)
	chain := newSignatureTestChain(t, keyring)
	sender := keyring.GetKey(1)

	builder, signerData := chain.unsignedSend(t, sender.AccAddr, sender.Priv.PubKey(), signing.SignMode_SIGN_MODE_DIRECT)
	sig := keyringSignature(t, sender.Priv, chain.signBytes(t, builder, signerData, signing.SignMode_SIGN_MODE_DIRECT))
	setSingleSignature(t, builder, signerData, signing.SignMode_SIGN_MODE_DIRECT, sig)
	signed := chain.encode(t, builder)
	v := sig[crypto.RecoveryIDOffset]

	reencoded := []struct {
		name string
		sig  []byte
		log  string
	}{
		{"64-byte R || S", sig[:64], "eth_secp256k1 signature must be 65 bytes (R || S || V), got 64"},
		{"V 0xff", withV(sig, 0xff), "eth_secp256k1 signature V must be 0 or 1 (27 or 28 for EIP-712 signatures), got 255"},
		{"V 2", withV(sig, 2), "got 2"},
		{"other recovery ID", withV(sig, v^1), "signature verification failed"},
		{"V + 27", withV(sig, v+27), "signature verification failed"},
		{"66 bytes", append(append([]byte{}, sig...), v), "got 66"},
	}

	// Every node rejects every re-encoding; each check runs on a fresh node
	// that has not seen the signed transaction.
	for _, tc := range reencoded {
		node := newSignatureTestChain(t, keyring)
		res, err := node.nw.CheckTx(withSignature(t, signed, tc.sig))
		require.NoError(t, err)
		requireUnauthorized(t, res, tc.log)
	}
	res, err := chain.nw.CheckTx(signed)
	require.NoError(t, err)
	require.Zero(t, res.Code, res.Log)

	// A proposer including a re-encoding first cannot displace the signed
	// transaction: the copy fails and the original executes.
	before, err := chain.handler.GetBalanceFromBank(sender.AccAddr, chain.nw.GetBaseDenom())
	require.NoError(t, err)
	txs := make([][]byte, 0, len(reencoded)+1)
	for _, tc := range reencoded {
		txs = append(txs, withSignature(t, signed, tc.sig))
	}
	txs = append(txs, signed)
	block, err := chain.nw.NextBlockWithTxs(txs...)
	require.NoError(t, err)
	require.Len(t, block.TxResults, len(txs))
	for i, tc := range reencoded {
		require.Equal(t, sdkerrors.ErrUnauthorized.ABCICode(), block.TxResults[i].Code, tc.name)
		require.NotEqual(t, sha256.Sum256(txs[i]), sha256.Sum256(signed), tc.name)
	}
	landed := block.TxResults[len(txs)-1]
	require.Zero(t, landed.Code, landed.Log)

	after, err := chain.handler.GetBalanceFromBank(sender.AccAddr, chain.nw.GetBaseDenom())
	require.NoError(t, err)
	fee := builder.GetTx().GetFee().AmountOf(chain.nw.GetBaseDenom())
	require.Equal(t, before.Balance.Amount.Sub(fee).SubRaw(1000), after.Balance.Amount, "charged once: the original's fee and amount")
	account, err := chain.handler.GetAccount(sender.AccAddr.String())
	require.NoError(t, err)
	require.Equal(t, signerData.Sequence+1, account.GetSequence())
}

// TestKeyringSignaturesStillLand covers both sign modes of the node's
// keyring (and of the bridge relayer's submitter): 65 bytes, V 0 or 1.
func TestKeyringSignaturesStillLand(t *testing.T) {
	keyring := testkeyring.New(2)
	chain := newSignatureTestChain(t, keyring)
	sender := keyring.GetKey(1)
	for _, mode := range []signing.SignMode{signing.SignMode_SIGN_MODE_DIRECT, signing.SignMode_SIGN_MODE_LEGACY_AMINO_JSON} {
		builder, signerData := chain.unsignedSend(t, sender.AccAddr, sender.Priv.PubKey(), mode)
		sig := keyringSignature(t, sender.Priv, chain.signBytes(t, builder, signerData, mode))
		setSingleSignature(t, builder, signerData, mode, sig)
		requireLands(t, chain, chain.encode(t, builder))
	}
}

// TestEIP712SignaturesStillLand covers the eth_signTypedData_v4 (MetaMask)
// and Ledger path: the signature over the EIP-712 representation of the sign
// bytes, with V 27 or 28, verifies through the eth_secp256k1 key; the same
// signature with V 0 or 1, or without V, is another encoding and is rejected.
func TestEIP712SignaturesStillLand(t *testing.T) {
	keyring := testkeyring.New(2)
	chain := newSignatureTestChain(t, keyring)
	sender := keyring.GetKey(1)
	for _, mode := range []signing.SignMode{signing.SignMode_SIGN_MODE_LEGACY_AMINO_JSON, signing.SignMode_SIGN_MODE_DIRECT} {
		builder, signerData := chain.unsignedSend(t, sender.AccAddr, sender.Priv.PubKey(), mode)
		sig := typedDataSignature(t, sender.Priv, chain.signBytes(t, builder, signerData, mode))
		setSingleSignature(t, builder, signerData, mode, sig)
		signed := chain.encode(t, builder)

		v := sig[crypto.RecoveryIDOffset]
		for name, other := range map[string][]byte{
			"V - 27":  withV(sig, v-27),
			"other V": withV(sig, v^3),
			"64-byte": sig[:64],
		} {
			res, err := chain.nw.CheckTx(withSignature(t, signed, other))
			require.NoError(t, err)
			require.Equal(t, sdkerrors.ErrUnauthorized.ABCICode(), res.Code, "%s %s: %s", mode, name, res.Log)
		}
		requireLands(t, chain, signed)
	}
}

// TestEthSecp256k1MultisigStillLands covers a 2-of-3 LegacyAminoPubKey with
// eth_secp256k1 members, like the testnet administrative multisig: members
// signing with the keyring (`tx sign --multisig`) or with a Ledger (EIP-712)
// still verify; a re-encoded member signature does not.
func TestEthSecp256k1MultisigStillLands(t *testing.T) {
	keyring := testkeyring.New(4)
	members := []cryptotypes.PrivKey{keyring.GetPrivKey(1), keyring.GetPrivKey(2), keyring.GetPrivKey(3)}
	memberPubKeys := make([]cryptotypes.PubKey, len(members))
	for i, member := range members {
		memberPubKeys[i] = member.PubKey()
	}
	multisigKey := kmultisig.NewLegacyAminoPubKey(2, memberPubKeys)
	multisigAddr := sdk.AccAddress(multisigKey.Address())
	chain := newSignatureTestChain(t, keyring, multisigAddr)
	mode := signing.SignMode_SIGN_MODE_LEGACY_AMINO_JSON

	build := func(sign map[int]func(cryptotypes.PrivKey, []byte) []byte) []byte {
		builder, signerData := chain.unsignedSend(t, multisigAddr, multisigKey, mode)
		signBytes := chain.signBytes(t, builder, signerData, mode)
		data := multisig.NewMultisig(len(members))
		for i := range members {
			signer, ok := sign[i]
			if !ok {
				continue
			}
			sig := &signing.SingleSignatureData{SignMode: mode, Signature: signer(members[i], signBytes)}
			require.NoError(t, multisig.AddSignatureFromPubKey(data, sig, memberPubKeys[i], memberPubKeys))
		}
		require.NoError(t, builder.SetSignatures(signing.SignatureV2{PubKey: multisigKey, Data: data, Sequence: signerData.Sequence}))
		return chain.encode(t, builder)
	}
	keyringMember := func(priv cryptotypes.PrivKey, bz []byte) []byte { return keyringSignature(t, priv, bz) }
	ledgerMember := func(priv cryptotypes.PrivKey, bz []byte) []byte { return typedDataSignature(t, priv, bz) }

	// Two keyring members.
	signed := build(map[int]func(cryptotypes.PrivKey, []byte) []byte{0: keyringMember, 2: keyringMember})
	requireLands(t, chain, signed)

	// A keyring member and a Ledger member.
	signed = build(map[int]func(cryptotypes.PrivKey, []byte) []byte{0: keyringMember, 1: ledgerMember})
	requireLands(t, chain, signed)

	// A member signature without V, or with the other V, is rejected.
	for name, reencode := range map[string]func([]byte) []byte{
		"64-byte member signature": func(sig []byte) []byte { return sig[:64] },
		"other recovery ID":        func(sig []byte) []byte { return withV(sig, sig[64]^1) },
	} {
		tampered := func(priv cryptotypes.PrivKey, bz []byte) []byte { return reencode(keyringSignature(t, priv, bz)) }
		bad := build(map[int]func(cryptotypes.PrivKey, []byte) []byte{0: keyringMember, 2: tampered})
		res, err := chain.nw.CheckTx(bad)
		require.NoError(t, err)
		require.Equal(t, sdkerrors.ErrUnauthorized.ABCICode(), res.Code, "%s: %s", name, res.Log)
	}
}

// TestSignatureLengthOfTheKeyring pins the encoding the node's keyring
// produces, which every client must match.
func TestSignatureLengthOfTheKeyring(t *testing.T) {
	key, err := ethsecp256k1.GenerateKey()
	require.NoError(t, err)
	sig, err := key.Sign([]byte("sign bytes"))
	require.NoError(t, err)
	require.Len(t, sig, ethsecp256k1.SignatureLength)
	require.LessOrEqual(t, sig[crypto.RecoveryIDOffset], byte(1))
	require.NoError(t, ethsecp256k1.ValidateSignatureEncoding(sig))
}
