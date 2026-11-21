package integration

import (
	"testing"

	"github.com/stretchr/testify/suite"

	evm "github.com/xitcoin-xtc/pos-chain"
	"github.com/xitcoin-xtc/pos-chain/tests/integration/x/ibc"
	testapp "github.com/xitcoin-xtc/pos-chain/testutil/app"
)

func TestIBCKeeperTestSuite(t *testing.T) {
	create := testapp.ToEvmAppCreator[evm.IBCIntegrationApp](CreateEvmd, "evm.IBCIntegrationApp")
	s := ibc.NewKeeperTestSuite(create)
	suite.Run(t, s)
}
