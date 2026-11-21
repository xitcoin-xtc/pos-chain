package vm

import (
	"github.com/xitcoin-xtc/pos-chain/testutil/integration/evm/network"
	evmante "github.com/xitcoin-xtc/pos-chain/x/vm/ante"

	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
)

func (s *EvmAnteTestSuite) TestBuildEvmExecutionCtx() {
	network := network.New(s.create, s.options...)

	ctx := evmante.BuildEvmExecutionCtx(network.GetContext())

	s.Equal(storetypes.GasConfig{}, ctx.KVGasConfig())
	s.Equal(storetypes.GasConfig{}, ctx.TransientKVGasConfig())
}
