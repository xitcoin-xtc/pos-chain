package contracts

import (
	contractutils "github.com/xitcoin-xtc/pos-chain/contracts/utils"
	evmtypes "github.com/xitcoin-xtc/pos-chain/x/vm/types"
)

func LoadERC20WithNativeTransfers() (evmtypes.CompiledContract, error) {
	return contractutils.LoadContractFromJSONFile("solidity/ERC20WithNativeTransfers.json")
}
