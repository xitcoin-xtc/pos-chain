package bech32

import (
	"github.com/stretchr/testify/suite"

	"github.com/xitcoin-xtc/pos-chain/precompiles/bech32"
	"github.com/xitcoin-xtc/pos-chain/testutil/integration/evm/network"
	testkeyring "github.com/xitcoin-xtc/pos-chain/testutil/keyring"
)

// PrecompileTestSuite is the implementation of the TestSuite interface for ERC20 precompile
// unit tests.
type PrecompileTestSuite struct {
	suite.Suite

	create  network.CreateEvmApp
	options []network.ConfigOption
	network *network.UnitTestNetwork
	keyring testkeyring.Keyring

	precompile *bech32.Precompile
}

func NewPrecompileTestSuite(create network.CreateEvmApp, options ...network.ConfigOption) *PrecompileTestSuite {
	return &PrecompileTestSuite{
		create:  create,
		options: options,
	}
}

func (s *PrecompileTestSuite) SetupTest() {
	keyring := testkeyring.New(2)
	options := []network.ConfigOption{
		network.WithPreFundedAccounts(keyring.GetAllAccAddrs()...),
	}
	options = append(options, s.options...)
	integrationNetwork := network.NewUnitTestNetwork(s.create, options...)

	s.keyring = keyring
	s.network = integrationNetwork

	precompile, err := bech32.NewPrecompile(6000)
	s.Require().NoError(err, "failed to create bech32 precompile")

	s.precompile = precompile
}
