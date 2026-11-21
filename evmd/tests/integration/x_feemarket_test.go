package integration

import (
	"testing"

	"github.com/stretchr/testify/suite"

	evm "github.com/xitcoin-xtc/pos-chain"
	"github.com/xitcoin-xtc/pos-chain/tests/integration/x/feemarket"
	testapp "github.com/xitcoin-xtc/pos-chain/testutil/app"
)

func TestFeeMarketKeeperTestSuite(t *testing.T) {
	create := testapp.ToEvmAppCreator[evm.IntegrationNetworkApp](CreateEvmd, "evm.IntegrationNetworkApp")
	s := feemarket.NewTestKeeperTestSuite(create)
	suite.Run(t, s)
}
