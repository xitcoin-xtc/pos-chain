package clients

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/xitcoin-xtc/pos-chain/crypto/hd"

	"github.com/cosmos/go-bip39"
)

const (
	ChainID    = "local-4221"
	EVMChainID = 4221

	JsonRPCUrl0 = "http://127.0.0.1:8545"
	JsonRPCUrl1 = "http://127.0.0.1:8555"
	JsonRPCUrl2 = "http://127.0.0.1:8565"
	JsonRPCUrl3 = "http://127.0.0.1:8575"

	NodeRPCUrl0 = "http://127.0.0.1:26657"
	NodeRPCUrl1 = "http://127.0.0.1:26658"
	NodeRPCUrl2 = "http://127.0.0.1:26659"
	NodeRPCUrl3 = "http://127.0.0.1:26660"
)

type Config struct {
	ChainID     string
	EVMChainID  *big.Int
	PrivKeys    []string
	JsonRPCUrls []string
	NodeRPCUrls []string
}

// NewConfig creates a new Config instance.
func NewConfig() (*Config, error) {
	// The four accounts funded by "xitcoind testnet init-files" (see
	// addExtraAccounts in evmd/cmd/evmd/cmd/testnet.go), derived the same way:
	// a mnemonic from fixed, local-test-only entropy, on the Ethereum BIP44
	// path. They are the genesis accounts 1 to 4 of the system test network.
	privKeys := make([]string, 4)
	for i := range privKeys {
		entropy := sha256.Sum256([]byte(fmt.Sprintf("xitcoin-local-test-only:extra-account-%d", i)))
		mnemonic, err := bip39.NewMnemonic(entropy[:])
		if err != nil {
			return nil, err
		}
		key, err := hd.EthSecp256k1.Derive()(mnemonic, "", "m/44'/60'/0'/0/0")
		if err != nil {
			return nil, err
		}
		privKeys[i] = hex.EncodeToString(key)
	}

	// jsonrpc urls of testnet nodes
	jsonRPCUrls := []string{JsonRPCUrl0, JsonRPCUrl1, JsonRPCUrl2, JsonRPCUrl3}

	// rpc urls of test nodes
	nodeRPCUrls := []string{NodeRPCUrl0, NodeRPCUrl1, NodeRPCUrl2, NodeRPCUrl3}

	return &Config{
		ChainID:     ChainID,
		EVMChainID:  big.NewInt(EVMChainID),
		PrivKeys:    privKeys,
		JsonRPCUrls: jsonRPCUrls,
		NodeRPCUrls: nodeRPCUrls,
	}, nil
}
