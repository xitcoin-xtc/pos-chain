package contracts

import (
	contractutils "github.com/xitcoin-xtc/pos-chain/contracts/utils"
	evmtypes "github.com/xitcoin-xtc/pos-chain/x/vm/types"
)

func LoadGasBurningERC20Contract() (evmtypes.CompiledContract, error) {
	return contractutils.LoadContractFromJSONFile("GasBurningERC20.json")
}
