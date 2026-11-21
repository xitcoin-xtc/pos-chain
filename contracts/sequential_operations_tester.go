package contracts

import (
	contractutils "github.com/xitcoin-xtc/pos-chain/contracts/utils"
	evmtypes "github.com/xitcoin-xtc/pos-chain/x/vm/types"
)

func LoadSequentialOperationsTester() (evmtypes.CompiledContract, error) {
	return contractutils.LoadContractFromJSONFile("solidity/SequentialOperationsTester.json")
}
