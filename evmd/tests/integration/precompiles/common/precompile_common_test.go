package common

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/xitcoin-xtc/pos-chain/evmd/tests/integration"
	"github.com/xitcoin-xtc/pos-chain/tests/integration/precompiles/common"
)

func TestStaticCallTestSuite(t *testing.T) {
	s := common.NewStaticCallTestSuite(integration.CreateEvmd)
	suite.Run(t, s)
}
