package bank

import (
	"testing"

	"github.com/stretchr/testify/suite"

	evm "github.com/xitcoin-xtc/pos-chain"
	"github.com/xitcoin-xtc/pos-chain/evmd/tests/integration"
	"github.com/xitcoin-xtc/pos-chain/tests/integration/precompiles/bank"
	testapp "github.com/xitcoin-xtc/pos-chain/testutil/app"
)

func TestBankPrecompileTestSuite(t *testing.T) {
	create := testapp.ToEvmAppCreator[evm.BankPrecompileApp](integration.CreateEvmd, "evm.BankPrecompileApp")
	s := bank.NewPrecompileTestSuite(create)
	suite.Run(t, s)
}

func TestBankPrecompileIntegrationTestSuite(t *testing.T) {
	create := testapp.ToEvmAppCreator[evm.BankPrecompileApp](integration.CreateEvmd, "evm.BankPrecompileApp")
	bank.TestIntegrationSuite(t, create)
}
