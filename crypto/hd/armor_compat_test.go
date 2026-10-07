package hd

import (
	"bytes"
	"fmt"
	"io"
	"testing"

	cmtcrypto "github.com/cometbft/cometbft/crypto"
	"github.com/stretchr/testify/require"
	oldarmor "golang.org/x/crypto/openpgp/armor"

	"github.com/cosmos/cosmos-sdk/codec/legacy"
	sdkcrypto "github.com/cosmos/cosmos-sdk/crypto"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/crypto/keys/bcrypt"
	"github.com/cosmos/cosmos-sdk/crypto/xsalsa20symmetric"
	"github.com/xitcoin-xtc/pos-chain/crypto/ethsecp256k1"
)

// This legacy parser is a test oracle only. No test key is used for signing.
func TestHistoricalEthereumKeyArmor(t *testing.T) {
	key, err := ethsecp256k1.GenerateKey()
	require.NoError(t, err)
	const passphrase = "synthetic-compatibility-only"
	encodeOld := func(kind string, headers map[string]string, payload []byte) string {
		var out bytes.Buffer
		writer, err := oldarmor.Encode(&out, kind, headers)
		require.NoError(t, err)
		_, err = writer.Write(payload)
		require.NoError(t, err)
		require.NoError(t, writer.Close())
		return out.String()
	}
	argon := sdkcrypto.EncryptArmorPrivKey(key, passphrase, string(EthSecp256k1Type))
	block, err := oldarmor.Decode(bytes.NewBufferString(argon))
	require.NoError(t, err)
	payload, err := io.ReadAll(block.Body)
	require.NoError(t, err)
	historicalArgon := encodeOld(block.Type, block.Header, payload)
	salt := cmtcrypto.CRandBytes(16)
	derived, err := bcrypt.GenerateFromPassword(salt, []byte(passphrase), sdkcrypto.BcryptSecurityParameter)
	require.NoError(t, err)
	ciphertext := xsalsa20symmetric.EncryptSymmetric(legacy.Cdc.Amino.MustMarshalBinaryBare(key), cmtcrypto.Sha256(derived))
	historicalBcrypt := encodeOld("TENDERMINT PRIVATE KEY", map[string]string{"kdf": "bcrypt", "salt": fmt.Sprintf("%X", salt), "type": string(EthSecp256k1Type)}, ciphertext)
	for name, armored := range map[string]string{"argon2": historicalArgon, "bcrypt": historicalBcrypt} {
		t.Run(name, func(t *testing.T) {
			kr := keyring.NewInMemory(TestCodec, EthSecp256k1Option())
			require.NoError(t, kr.ImportPrivKey("synthetic", armored, passphrase))
			record, err := kr.Key("synthetic")
			require.NoError(t, err)
			public, err := record.GetPubKey()
			require.NoError(t, err)
			require.True(t, key.PubKey().Equals(public))
			exported, err := kr.ExportPrivKeyArmor("synthetic", passphrase)
			require.NoError(t, err)
			old, err := oldarmor.Decode(bytes.NewBufferString(exported))
			require.NoError(t, err)
			data, err := io.ReadAll(old.Body)
			require.NoError(t, err)
			decoded, algo, err := sdkcrypto.UnarmorDecryptPrivKey(encodeOld(old.Type, old.Header, data), passphrase)
			require.NoError(t, err)
			require.Equal(t, string(EthSecp256k1Type), algo)
			require.True(t, key.Equals(decoded))
			require.Error(t, kr.ImportPrivKey("wrong-password", armored, "incorrect"))
		})
	}
}
