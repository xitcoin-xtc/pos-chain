package slashing

import (
	"testing"

	"github.com/stretchr/testify/suite"

	evm "github.com/xitcoin-xtc/pos-chain"
	"github.com/xitcoin-xtc/pos-chain/evmd/tests/integration"
	"github.com/xitcoin-xtc/pos-chain/tests/integration/precompiles/slashing"
	testapp "github.com/xitcoin-xtc/pos-chain/testutil/app"
)

func TestSlashingPrecompileTestSuite(t *testing.T) {
	create := testapp.ToEvmAppCreator[evm.SlashingPrecompileApp](integration.CreateEvmd, "evm.SlashingPrecompileApp")
	s := slashing.NewPrecompileTestSuite(create)
	suite.Run(t, s)
}

func TestStakingPrecompileIntegrationTestSuite(t *testing.T) {
	create := testapp.ToEvmAppCreator[evm.SlashingPrecompileApp](integration.CreateEvmd, "evm.SlashingPrecompileApp")
	slashing.TestPrecompileIntegrationTestSuite(t, create)
}
