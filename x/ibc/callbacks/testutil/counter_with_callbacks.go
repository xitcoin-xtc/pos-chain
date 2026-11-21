package testutil

import (
	contractutils "github.com/xitcoin-xtc/pos-chain/contracts/utils"
	evmtypes "github.com/xitcoin-xtc/pos-chain/x/vm/types"
)

func LoadCounterWithCallbacksContract() (evmtypes.CompiledContract, error) {
	return contractutils.LoadContractFromJSONFile("CounterWithCallbacks.json")
}
