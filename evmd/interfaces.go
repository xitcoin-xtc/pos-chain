package evmd

import (
	cmn "github.com/xitcoin-xtc/pos-chain/precompiles/common"
	evmtypes "github.com/xitcoin-xtc/pos-chain/x/vm/types"
)

type BankKeeper interface {
	evmtypes.BankKeeper
	cmn.BankKeeper
}
