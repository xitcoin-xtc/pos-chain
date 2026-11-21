package contracts

import (
	contractutils "github.com/xitcoin-xtc/pos-chain/contracts/utils"
	evmtypes "github.com/xitcoin-xtc/pos-chain/x/vm/types"
)

func LoadContractCreationTester() (evmtypes.CompiledContract, error) {
	return contractutils.LoadContractFromJSONFile("solidity/ContractCreationTester.json")
}
