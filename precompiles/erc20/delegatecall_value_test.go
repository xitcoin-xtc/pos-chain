package erc20

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/stretchr/testify/require"

	erc20types "github.com/xitcoin-xtc/pos-chain/x/erc20/types"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// The value check of the ERC-20 precompile dereferenced the nil value of a
// DELEGATECALL: every DELEGATECALL to an ERC-20 precompile panicked instead
// of running read-only. Found by FuzzERC20PrecompileCalldata.
func TestExecuteAcceptsTheNilValueOfADelegateCall(t *testing.T) {
	pair := erc20types.NewTokenPair(common.HexToAddress("0x80b5a32E4F032B2a058b4F29EC95EEfEEB87aDcd"), "ibc/voucher", erc20types.OWNER_MODULE)
	p := NewPrecompile(pair, nil, nil, nil)
	input, err := p.Pack(TransferMethod, common.Address{3}, big.NewInt(1))
	require.NoError(t, err)
	contract := vm.NewPrecompile(common.Address{1}, p.Address(), nil, 100_000)
	contract.Input = input
	require.NotPanics(t, func() {
		_, err = p.Execute(sdk.Context{}, nil, contract, true)
	})
	require.ErrorIs(t, err, vm.ErrWriteProtection)
}
