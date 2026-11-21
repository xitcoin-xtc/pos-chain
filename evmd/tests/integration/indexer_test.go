package integration

import (
	"testing"

	evm "github.com/xitcoin-xtc/pos-chain"
	"github.com/xitcoin-xtc/pos-chain/tests/integration/indexer"
	testapp "github.com/xitcoin-xtc/pos-chain/testutil/app"
)

func TestKVIndexer(t *testing.T) {
	create := testapp.ToEvmAppCreator[evm.IntegrationNetworkApp](CreateEvmd, "evm.IntegrationNetworkApp")
	indexer.TestKVIndexer(t, create)
}
