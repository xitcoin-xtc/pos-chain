package cosmos_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/xitcoin-xtc/pos-chain/ante/cosmos"
	anteinterfaces "github.com/xitcoin-xtc/pos-chain/ante/interfaces"
	"github.com/xitcoin-xtc/pos-chain/crypto/ethsecp256k1"
	"github.com/xitcoin-xtc/pos-chain/encoding"
	"github.com/xitcoin-xtc/pos-chain/testutil/constants"

	"cosmossdk.io/math"

	kmultisig "github.com/cosmos/cosmos-sdk/crypto/keys/multisig"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	"github.com/cosmos/cosmos-sdk/crypto/types/multisig"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
)

func ethSig(v byte) []byte {
	sig := make([]byte, ethsecp256k1.SignatureLength)
	sig[0] = 1
	sig[64] = v
	return sig
}

func single(sig []byte) *signing.SingleSignatureData {
	return &signing.SingleSignatureData{SignMode: signing.SignMode_SIGN_MODE_DIRECT, Signature: sig}
}

func ethPubKey(t *testing.T) cryptotypes.PubKey {
	t.Helper()
	key, err := ethsecp256k1.GenerateKey()
	require.NoError(t, err)
	return key.PubKey()
}

// multiSig builds the signature data of members whose index is in sigs.
func multiSig(t *testing.T, members []cryptotypes.PubKey, sigs map[int]signing.SignatureData) *signing.MultiSignatureData {
	t.Helper()
	data := multisig.NewMultisig(len(members))
	for i := range members {
		if sig, ok := sigs[i]; ok {
			require.NoError(t, multisig.AddSignatureFromPubKey(data, sig, members[i], members))
		}
	}
	return data
}

func TestCheckSignatureEncoding(t *testing.T) {
	ethKey := ethPubKey(t)
	cosmosKey := secp256k1.GenPrivKey().PubKey()
	members := []cryptotypes.PubKey{ethPubKey(t), ethPubKey(t), cosmosKey}
	multisigKey := kmultisig.NewLegacyAminoPubKey(2, members)
	nestedMembers := []cryptotypes.PubKey{multisigKey, ethPubKey(t)}
	nestedKey := kmultisig.NewLegacyAminoPubKey(1, nestedMembers)

	testCases := []struct {
		name   string
		pubKey cryptotypes.PubKey
		data   signing.SignatureData
		errMsg string
	}{
		{"V 0", ethKey, single(ethSig(0)), ""},
		{"V 1", ethKey, single(ethSig(1)), ""},
		{"V 27 (EIP-712)", ethKey, single(ethSig(27)), ""},
		{"V 28 (EIP-712)", ethKey, single(ethSig(28)), ""},
		{"V 2", ethKey, single(ethSig(2)), "V must be 0 or 1 (27 or 28 for EIP-712 signatures), got 2"},
		{"V 29", ethKey, single(ethSig(29)), "got 29"},
		{"V 0xff", ethKey, single(ethSig(0xff)), "got 255"},
		{"64-byte R || S", ethKey, single(ethSig(0)[:64]), "must be 65 bytes (R || S || V), got 64"},
		{"66 bytes", ethKey, single(append(ethSig(0), 0)), "got 66"},
		{"empty", ethKey, single(nil), "got 0"},
		{"secp256k1 key: left to its own verification", cosmosKey, single(make([]byte, 64)), ""},
		{"multisig, canonical members", multisigKey, multiSig(t, members, map[int]signing.SignatureData{0: single(ethSig(0)), 1: single(ethSig(28))}), ""},
		{"multisig, cosmos member with 64 bytes", multisigKey, multiSig(t, members, map[int]signing.SignatureData{0: single(ethSig(1)), 2: single(make([]byte, 64))}), ""},
		{"multisig, 64-byte member", multisigKey, multiSig(t, members, map[int]signing.SignatureData{0: single(ethSig(0)), 1: single(ethSig(0)[:64])}), "multisig member 1: eth_secp256k1 signature must be 65 bytes"},
		{"multisig, bad V member", multisigKey, multiSig(t, members, map[int]signing.SignatureData{0: single(ethSig(7)), 2: single(make([]byte, 64))}), "multisig member 0: eth_secp256k1 signature V must be"},
		{"nested multisig, bad member", nestedKey, multiSig(t, nestedMembers, map[int]signing.SignatureData{
			0: multiSig(t, members, map[int]signing.SignatureData{0: single(ethSig(0)), 1: single(ethSig(0)[:64])}),
		}), "multisig member 0: multisig member 1: eth_secp256k1 signature must be 65 bytes"},
		{"nested multisig, canonical", nestedKey, multiSig(t, nestedMembers, map[int]signing.SignatureData{1: single(ethSig(1))}), ""},
		{"structural mismatch: left to verification", ethKey, multiSig(t, members, map[int]signing.SignatureData{0: single(ethSig(0)[:64])}), ""},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := cosmos.CheckSignatureEncoding(tc.pubKey, tc.data)
			if tc.errMsg == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.errMsg)
		})
	}
}

type accountKeeperStub struct {
	anteinterfaces.AccountKeeper
	accounts map[string]sdk.AccountI
}

func (ak accountKeeperStub) GetAccount(_ context.Context, addr sdk.AccAddress) sdk.AccountI {
	return ak.accounts[addr.String()]
}

func TestSignatureEncodingDecorator(t *testing.T) {
	txCfg := encoding.MakeConfig(constants.ExampleChainID.EVMChainID).TxConfig

	ethKey := ethPubKey(t)
	members := []cryptotypes.PubKey{ethPubKey(t), ethPubKey(t), ethPubKey(t)}
	multisigKey := kmultisig.NewLegacyAminoPubKey(2, members)
	ak := accountKeeperStub{accounts: map[string]sdk.AccountI{}}
	for i, key := range []cryptotypes.PubKey{ethKey, multisigKey} {
		addr := sdk.AccAddress(key.Address())
		ak.accounts[addr.String()] = authtypes.NewBaseAccount(addr, key, uint64(i), 0)
	}

	buildTx := func(key cryptotypes.PubKey, data signing.SignatureData) sdk.Tx {
		addr := sdk.AccAddress(key.Address())
		builder := txCfg.NewTxBuilder()
		require.NoError(t, builder.SetMsgs(banktypes.NewMsgSend(addr, addr, sdk.NewCoins(sdk.NewCoin("axtc", math.NewInt(1))))))
		require.NoError(t, builder.SetSignatures(signing.SignatureV2{PubKey: key, Data: data}))
		return builder.GetTx()
	}

	decorator := cosmos.NewSignatureEncodingDecorator(ak)
	next := func(ctx sdk.Context, _ sdk.Tx, _ bool) (sdk.Context, error) { return ctx, nil }
	ctx := sdk.Context{}

	testCases := []struct {
		name     string
		tx       sdk.Tx
		simulate bool
		errMsg   string
	}{
		{"keyring signature", buildTx(ethKey, single(ethSig(1))), false, ""},
		{"EIP-712 signature", buildTx(ethKey, single(ethSig(27))), false, ""},
		{"64-byte signature", buildTx(ethKey, single(ethSig(1)[:64])), false, "signature 0: eth_secp256k1 signature must be 65 bytes (R || S || V), got 64"},
		{"V 0xff", buildTx(ethKey, single(ethSig(0xff))), false, "signature 0: eth_secp256k1 signature V must be 0 or 1"},
		{"simulation with an empty signature", buildTx(ethKey, single(nil)), true, ""},
		{"multisig", buildTx(multisigKey, multiSig(t, members, map[int]signing.SignatureData{0: single(ethSig(0)), 2: single(ethSig(1))})), false, ""},
		{"multisig with a 64-byte member", buildTx(multisigKey, multiSig(t, members, map[int]signing.SignatureData{0: single(ethSig(0)), 2: single(ethSig(1)[:64])})), false, "signature 0: multisig member 2: eth_secp256k1 signature must be 65 bytes"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := decorator.AnteHandle(ctx, tc.tx, tc.simulate, next)
			if tc.errMsg == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, sdkerrors.ErrUnauthorized)
			require.ErrorContains(t, err, tc.errMsg)
		})
	}
}
