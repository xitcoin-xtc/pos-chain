package cmd

import (
	"testing"

	sdkcrypto "github.com/cosmos/cosmos-sdk/crypto"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/stretchr/testify/require"
	"github.com/xitcoin-xtc/pos-chain/crypto/ethsecp256k1"
	"github.com/xitcoin-xtc/pos-chain/crypto/hd"
	"github.com/xitcoin-xtc/pos-chain/encoding"
)

// Uses the same encoding setup as NewExampleApp, without starting an app,
// opening a database or contacting a node. No key is used for signing.
func TestEVMDKeyArmorCompatibility(t *testing.T) {
	config := encoding.MakeConfig(1)
	kr := keyring.NewInMemory(config.Codec, hd.EthSecp256k1Option())
	key, err := ethsecp256k1.GenerateKey()
	require.NoError(t, err)
	const passphrase = "synthetic-evmd-compatibility"
	armored := sdkcrypto.EncryptArmorPrivKey(key, passphrase, string(hd.EthSecp256k1Type))
	require.NoError(t, kr.ImportPrivKey("synthetic", armored, passphrase))
	exported, err := kr.ExportPrivKeyArmor("synthetic", passphrase)
	require.NoError(t, err)
	decoded, algo, err := sdkcrypto.UnarmorDecryptPrivKey(exported, passphrase)
	require.NoError(t, err)
	require.Equal(t, string(hd.EthSecp256k1Type), algo)
	require.True(t, key.Equals(decoded))
	require.Error(t, kr.ImportPrivKey("incorrect", armored, "wrong-passphrase"))
}
