package mempool

import (
	"testing"

	"github.com/stretchr/testify/suite"

	evm "github.com/xitcoin-xtc/pos-chain"
	"github.com/xitcoin-xtc/pos-chain/evmd/tests/integration"
	"github.com/xitcoin-xtc/pos-chain/tests/integration/mempool"
	testapp "github.com/xitcoin-xtc/pos-chain/testutil/app"
)

func TestMempoolIntegrationTestSuite(t *testing.T) {
	create := testapp.ToEvmAppCreator[evm.IntegrationNetworkApp](integration.CreateEvmd, "evm.IntegrationNetworkApp")
	suite.Run(t, mempool.NewMempoolIntegrationTestSuite(create))
}
