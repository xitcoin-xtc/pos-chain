package werc20

import (
	"testing"

	"github.com/stretchr/testify/suite"

	evm "github.com/xitcoin-xtc/pos-chain"
	"github.com/xitcoin-xtc/pos-chain/evmd/tests/integration"
	"github.com/xitcoin-xtc/pos-chain/tests/integration/precompiles/werc20"
	testapp "github.com/xitcoin-xtc/pos-chain/testutil/app"
)

func TestWERC20PrecompileUnitTestSuite(t *testing.T) {
	create := testapp.ToEvmAppCreator[evm.WERC20PrecompileApp](integration.CreateEvmd, "evm.WERC20PrecompileApp")
	s := werc20.NewPrecompileUnitTestSuite(create)
	suite.Run(t, s)
}

func TestWERC20PrecompileIntegrationTestSuite(t *testing.T) {
	create := testapp.ToEvmAppCreator[evm.WERC20PrecompileApp](integration.CreateEvmd, "evm.WERC20PrecompileApp")
	werc20.TestPrecompileIntegrationTestSuite(t, create)
}
