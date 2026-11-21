package eip7702

import (
	"testing"

	evm "github.com/xitcoin-xtc/pos-chain"
	"github.com/xitcoin-xtc/pos-chain/evmd/tests/integration"
	"github.com/xitcoin-xtc/pos-chain/tests/integration/eip7702"
	testapp "github.com/xitcoin-xtc/pos-chain/testutil/app"
)

func TestEIP7702IntegrationTestSuite(t *testing.T) {
	create := testapp.ToEvmAppCreator[evm.IntegrationNetworkApp](integration.CreateEvmd, "evm.IntegrationNetworkApp")
	eip7702.TestEIP7702IntegrationTestSuite(t, create)
}
