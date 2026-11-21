package ante

import (
	"testing"

	evm "github.com/xitcoin-xtc/pos-chain"
	"github.com/xitcoin-xtc/pos-chain/evmd/tests/integration"
	"github.com/xitcoin-xtc/pos-chain/tests/integration/ante"
	testapp "github.com/xitcoin-xtc/pos-chain/testutil/app"
)

func TestAnte_Integration(t *testing.T) {
	create := testapp.ToEvmAppCreator[evm.AnteIntegrationApp](integration.CreateEvmd, "evm.AnteIntegrationApp")
	ante.TestIntegrationAnteHandler(t, create)
}

func BenchmarkAnteHandler(b *testing.B) {
	create := testapp.ToEvmAppCreator[evm.AnteIntegrationApp](integration.CreateEvmd, "evm.AnteIntegrationApp")
	// Run the benchmark with a mock EVM app
	ante.RunBenchmarkAnteHandler(b, create)
}

func TestValidateHandlerOptions(t *testing.T) {
	create := testapp.ToEvmAppCreator[evm.AnteIntegrationApp](integration.CreateEvmd, "evm.AnteIntegrationApp")
	ante.RunValidateHandlerOptionsTest(t, create)
}
