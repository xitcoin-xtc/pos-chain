package ics20

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/xitcoin-xtc/pos-chain"
	"github.com/xitcoin-xtc/pos-chain/evmd/tests/integration"
	"github.com/xitcoin-xtc/pos-chain/tests/integration/precompiles/ics20"
	testapp "github.com/xitcoin-xtc/pos-chain/testutil/app"
)

var ibcAppCreator = testapp.ToIBCAppCreator[evm.ICS20PrecompileApp](integration.SetupEvmd, "evm.ICS20PrecompileApp")

func TestICS20PrecompileTestSuite(t *testing.T) {
	s := ics20.NewPrecompileTestSuite(t, ibcAppCreator)
	suite.Run(t, s)
}

func TestICS20PrecompileIntegrationTestSuite(t *testing.T) {
	ics20.TestPrecompileIntegrationTestSuite(t, ibcAppCreator)
}
