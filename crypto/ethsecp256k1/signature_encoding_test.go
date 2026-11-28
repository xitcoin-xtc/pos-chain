package ethsecp256k1

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protoregistry"

	"github.com/xitcoin-xtc/pos-chain/ethereum/eip712"

	"cosmossdk.io/math"

	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/codec/address"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	kmultisig "github.com/cosmos/cosmos-sdk/crypto/keys/multisig"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	"github.com/cosmos/cosmos-sdk/crypto/types/multisig"
	sdk "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	txsigning "github.com/cosmos/cosmos-sdk/x/tx/signing"
)

var secp256k1N = crypto.S256().Params().N

func withV(sig []byte, v byte) []byte {
	out := append([]byte{}, sig[:64]...)
	return append(out, v)
}

// highS returns the other valid ECDSA signature of the same (R, S) pair:
// (R, N - S) with the opposite recovery ID.
func highS(sig []byte) []byte {
	s := new(big.Int).SetBytes(sig[32:64])
	s.Sub(secp256k1N, s)
	out := append([]byte{}, sig[:32]...)
	out = append(out, s.FillBytes(make([]byte, 32))...)
	return append(out, sig[64]^1)
}

// acceptedVs returns every V for which R || S || V verifies over msg.
func acceptedVs(pubKey *PubKey, msg, sig []byte) []int {
	var accepted []int
	for v := 0; v < 256; v++ {
		if pubKey.VerifySignature(msg, withV(sig, byte(v))) {
			accepted = append(accepted, v)
		}
	}
	return accepted
}

// directSignDoc returns SIGN_MODE_DIRECT sign bytes that the EIP-712 path can
// convert.
func directSignDoc(t *testing.T) []byte {
	t.Helper()
	registry, err := codectypes.NewInterfaceRegistryWithOptions(codectypes.InterfaceRegistryOptions{
		ProtoFiles: protoregistry.GlobalFiles,
		SigningOptions: txsigning.Options{
			AddressCodec:          address.NewBech32Codec(sdk.Bech32PrefixAccAddr),
			ValidatorAddressCodec: address.NewBech32Codec(sdk.Bech32PrefixValAddr),
		},
	})
	require.NoError(t, err)
	cryptocodec.RegisterInterfaces(registry)
	banktypes.RegisterInterfaces(registry)
	eip712.SetEncodingConfig(codec.NewLegacyAmino(), registry, 9000)

	signer := secp256k1.GenPrivKey().PubKey()
	addr := sdk.AccAddress(signer.Address())
	msgAny, err := codectypes.NewAnyWithValue(banktypes.NewMsgSend(addr, addr, sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, math.NewInt(1)))))
	require.NoError(t, err)
	bodyBz, err := (&txtypes.TxBody{Messages: []*codectypes.Any{msgAny}}).Marshal()
	require.NoError(t, err)
	pubKeyAny, err := codectypes.NewAnyWithValue(signer)
	require.NoError(t, err)
	authInfoBz, err := (&txtypes.AuthInfo{
		SignerInfos: []*txtypes.SignerInfo{{
			PublicKey: pubKeyAny,
			ModeInfo:  &txtypes.ModeInfo{Sum: &txtypes.ModeInfo_Single_{Single: &txtypes.ModeInfo_Single{Mode: signing.SignMode_SIGN_MODE_DIRECT}}},
		}},
		Fee: &txtypes.Fee{Amount: sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, math.NewInt(1000))), GasLimit: 100000},
	}).Marshal()
	require.NoError(t, err)
	signDoc, err := (&txtypes.SignDoc{BodyBytes: bodyBz, AuthInfoBytes: authInfoBz, ChainId: "xitcoin-testnet-v2-1"}).Marshal()
	require.NoError(t, err)
	return signDoc
}

// eip712Signature signs the EIP-712 representation of signDoc as
// eth_signTypedData_v4 does: V is 27 or 28.
func eip712Signature(t *testing.T, privKey *PrivKey, signDoc []byte) []byte {
	t.Helper()
	eip712Bytes, err := eip712.GetEIP712BytesForMsg(signDoc)
	require.NoError(t, err)
	sig, err := privKey.Sign(eip712Bytes)
	require.NoError(t, err)
	sig[crypto.RecoveryIDOffset] += 27
	return sig
}

func TestVerifySignatureAcceptsOnlyTheRecoverableEncoding(t *testing.T) {
	privKey, err := GenerateKey()
	require.NoError(t, err)
	pubKey := privKey.PubKey().(*PubKey)
	other, err := GenerateKey()
	require.NoError(t, err)

	msg := []byte("sign bytes of a cosmos transaction")
	sig, err := privKey.Sign(msg)
	require.NoError(t, err)
	require.Len(t, sig, SignatureLength)
	v := sig[crypto.RecoveryIDOffset]
	require.LessOrEqual(t, v, byte(1))

	require.True(t, pubKey.VerifySignature(msg, sig), "keyring signature")

	for name, encoding := range map[string][]byte{
		"other recovery ID":        withV(sig, v^1),
		"V + 27":                   withV(sig, v+27),
		"other V + 27":             withV(sig, (v^1)+27),
		"V = 2":                    withV(sig, 2),
		"V = 0xff":                 withV(sig, 0xff),
		"64-byte R || S":           sig[:64],
		"66 bytes":                 append(append([]byte{}, sig...), 0),
		"R || S || V with V twice": append(append([]byte{}, sig...), v),
		"high S":                   highS(sig),
		"empty":                    {},
		"nil":                      nil,
	} {
		require.False(t, pubKey.VerifySignature(msg, encoding), name)
	}

	require.Equal(t, []int{int(v)}, acceptedVs(pubKey, msg, sig), "exactly one V verifies")
	require.False(t, pubKey.VerifySignature([]byte("other sign bytes"), sig), "other message")
	otherSig, err := other.Sign(msg)
	require.NoError(t, err)
	require.False(t, pubKey.VerifySignature(msg, otherSig), "other key")
}

func TestVerifySignatureUncompressedKey(t *testing.T) {
	// crypto.VerifySignature parses an uncompressed key: such a key keeps
	// verifying its canonical signature, and only that one.
	privKey, err := GenerateKey()
	require.NoError(t, err)
	key, err := privKey.ToECDSA()
	require.NoError(t, err)
	pubKey := &PubKey{Key: crypto.FromECDSAPub(&key.PublicKey)}
	msg := []byte("sign bytes")
	sig, err := privKey.Sign(msg)
	require.NoError(t, err)
	require.True(t, pubKey.VerifySignature(msg, sig))
	require.Equal(t, []int{int(sig[64])}, acceptedVs(pubKey, msg, sig))
	require.False(t, pubKey.VerifySignature(msg, sig[:64]))
	require.False(t, (&PubKey{Key: []byte{4, 1, 2}}).VerifySignature(msg, sig), "malformed key")
}

func TestVerifySignatureManyKeys(t *testing.T) {
	// Both recovery IDs occur: each keyring signature verifies and its
	// twin with the other V does not.
	seen := map[byte]bool{}
	for i := 0; i < 64; i++ {
		privKey, err := GenerateKey()
		require.NoError(t, err)
		pubKey := privKey.PubKey().(*PubKey)
		msg := []byte{byte(i), 'm', 's', 'g'}
		sig, err := privKey.Sign(msg)
		require.NoError(t, err)
		v := sig[crypto.RecoveryIDOffset]
		seen[v] = true
		require.True(t, pubKey.VerifySignature(msg, sig))
		require.False(t, pubKey.VerifySignature(msg, withV(sig, v^1)))
		require.False(t, pubKey.VerifySignature(msg, sig[:64]))
	}
	require.True(t, seen[0] && seen[1], "both recovery IDs exercised")
}

func TestVerifySignatureEIP712(t *testing.T) {
	privKey, err := GenerateKey()
	require.NoError(t, err)
	pubKey := privKey.PubKey().(*PubKey)
	signDoc := directSignDoc(t)

	// eth_signTypedData_v4 (MetaMask) and the Ledger Ethereum app return V 27
	// or 28: that form verifies against the sign bytes.
	sig := eip712Signature(t, privKey, signDoc)
	v := sig[crypto.RecoveryIDOffset]
	require.Contains(t, []byte{27, 28}, v)
	require.True(t, pubKey.VerifySignature(signDoc, sig))
	require.Equal(t, []int{int(v)}, acceptedVs(pubKey, signDoc, sig), "exactly one V verifies")

	for name, encoding := range map[string][]byte{
		"V - 27 (keyring form)": withV(sig, v-27),
		"other V":               withV(sig, v^3), // 27 <-> 28
		"64-byte R || S":        sig[:64],
		"high S":                highS(sig),
	} {
		require.False(t, pubKey.VerifySignature(signDoc, encoding), name)
	}

	// The EIP-712 bytes themselves are a plain message: their keyring
	// signature (V 0 or 1) verifies over them, the typed-data form does not.
	eip712Bytes, err := eip712.GetEIP712BytesForMsg(signDoc)
	require.NoError(t, err)
	require.True(t, pubKey.VerifySignature(eip712Bytes, withV(sig, v-27)))
	require.False(t, pubKey.VerifySignature(eip712Bytes, sig))

	// A keyring signature over the sign bytes is not an EIP-712 signature.
	direct, err := privKey.Sign(signDoc)
	require.NoError(t, err)
	require.True(t, pubKey.VerifySignature(signDoc, direct))
	require.False(t, pubKey.VerifySignature(signDoc, withV(direct, direct[64]+27)))
}

func TestVerifyMultisignatureWithEthSecp256k1Members(t *testing.T) {
	keys := make([]*PrivKey, 3)
	pubKeys := make([]cryptotypes.PubKey, 3)
	for i := range keys {
		var err error
		keys[i], err = GenerateKey()
		require.NoError(t, err)
		pubKeys[i] = keys[i].PubKey()
	}
	multisigKey := kmultisig.NewLegacyAminoPubKey(2, pubKeys)
	signDoc := directSignDoc(t)
	signBytes := func(signing.SignMode) ([]byte, error) { return signDoc, nil }

	build := func(sigs map[int][]byte) *signing.MultiSignatureData {
		data := multisig.NewMultisig(len(pubKeys))
		for i := range pubKeys {
			if sig, ok := sigs[i]; ok {
				require.NoError(t, multisig.AddSignatureFromPubKey(data,
					&signing.SingleSignatureData{SignMode: signing.SignMode_SIGN_MODE_LEGACY_AMINO_JSON, Signature: sig},
					pubKeys[i], pubKeys))
			}
		}
		return data
	}

	keyringSig := func(i int) []byte {
		sig, err := keys[i].Sign(signDoc)
		require.NoError(t, err)
		return sig
	}

	// Keyring members (tx sign --multisig): V 0 or 1.
	require.NoError(t, multisigKey.VerifyMultisignature(signBytes, build(map[int][]byte{0: keyringSig(0), 2: keyringSig(2)})))
	// A Ledger member signs the EIP-712 representation: V 27 or 28.
	require.NoError(t, multisigKey.VerifyMultisignature(signBytes, build(map[int][]byte{0: keyringSig(0), 1: eip712Signature(t, keys[1], signDoc)})))

	// Re-encoding one member signature is rejected.
	sig := keyringSig(2)
	for name, encoding := range map[string][]byte{
		"64-byte member signature": sig[:64],
		"other recovery ID":        withV(sig, sig[64]^1),
		"V + 27":                   withV(sig, sig[64]+27),
	} {
		require.Error(t, multisigKey.VerifyMultisignature(signBytes, build(map[int][]byte{0: keyringSig(0), 2: encoding})), name)
	}
}

func TestValidateSignatureEncoding(t *testing.T) {
	sig := make([]byte, SignatureLength)
	for v := 0; v < 256; v++ {
		sig[64] = byte(v)
		err := ValidateSignatureEncoding(sig)
		if v == 0 || v == 1 || v == 27 || v == 28 {
			require.NoError(t, err, v)
		} else {
			require.ErrorContains(t, err, "V must be 0 or 1 (27 or 28 for EIP-712 signatures)", v)
		}
	}
	require.ErrorContains(t, ValidateSignatureEncoding(sig[:64]), "must be 65 bytes (R || S || V), got 64")
	require.ErrorContains(t, ValidateSignatureEncoding(nil), "got 0")
	require.ErrorContains(t, ValidateSignatureEncoding(make([]byte, 66)), "got 66")
}
