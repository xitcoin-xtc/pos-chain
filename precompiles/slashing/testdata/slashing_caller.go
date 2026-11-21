package testdata

import (
	contractutils "github.com/xitcoin-xtc/pos-chain/contracts/utils"
	evmtypes "github.com/xitcoin-xtc/pos-chain/x/vm/types"
)

func LoadSlashingCallerContract() (evmtypes.CompiledContract, error) {
	return contractutils.LoadContractFromJSONFile("SlashingCaller.json")
}
