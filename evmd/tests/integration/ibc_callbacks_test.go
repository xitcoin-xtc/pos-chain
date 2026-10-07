package integration

import (
	"testing"

	"github.com/stretchr/testify/suite"

	evm "github.com/xitcoin-xtc/pos-chain"
	"github.com/xitcoin-xtc/pos-chain/tests/integration/x/ibc/callbacks"
	testapp "github.com/xitcoin-xtc/pos-chain/testutil/app"
)

func TestIBCCallback(t *testing.T) {
	create := testapp.ToEvmAppCreator[evm.IBCCallbackIntegrationApp](CreateEvmd, "evm.IBCCallbackIntegrationApp")
	suite.Run(t, callbacks.NewKeeperTestSuite(create))
}
