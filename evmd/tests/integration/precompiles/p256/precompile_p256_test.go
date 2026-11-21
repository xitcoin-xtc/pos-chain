package p256

import (
	"testing"

	"github.com/stretchr/testify/suite"

	evm "github.com/xitcoin-xtc/pos-chain"
	"github.com/xitcoin-xtc/pos-chain/evmd/tests/integration"
	"github.com/xitcoin-xtc/pos-chain/tests/integration/precompiles/p256"
	testapp "github.com/xitcoin-xtc/pos-chain/testutil/app"
)

func TestP256PrecompileTestSuite(t *testing.T) {
	create := testapp.ToEvmAppCreator[evm.P256PrecompileApp](integration.CreateEvmd, "evm.P256PrecompileApp")
	s := p256.NewPrecompileTestSuite(create)
	suite.Run(t, s)
}

func TestP256PrecompileIntegrationTestSuite(t *testing.T) {
	create := testapp.ToEvmAppCreator[evm.P256PrecompileApp](integration.CreateEvmd, "evm.P256PrecompileApp")
	p256.TestPrecompileIntegrationTestSuite(t, create)
}
