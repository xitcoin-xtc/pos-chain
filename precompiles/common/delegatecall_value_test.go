package common_test

import (
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/stretchr/testify/require"

	cmn "github.com/xitcoin-xtc/pos-chain/precompiles/common"
)

// The EVM runs a precompile reached through DELEGATECALL with a nil value.
// SetupABI dereferenced it on empty calldata, so a DELEGATECALL with no
// calldata to any stateful precompile panicked instead of reverting. Found
// by the precompile calldata attack simulations.
func TestSetupABIAcceptsTheNilValueOfADelegateCall(t *testing.T) {
	contractABI, err := abi.JSON(strings.NewReader(`[
		{"type":"receive","stateMutability":"payable"},
		{"type":"function","name":"get","inputs":[],"outputs":[],"stateMutability":"view"}
	]`))
	require.NoError(t, err)
	contract := vm.NewPrecompile(common.Address{1}, common.Address{2}, nil, 100_000)
	require.Nil(t, contract.Value())
	require.Zero(t, cmn.CallValue(contract).Sign())

	isTransaction := func(method *abi.Method) bool { return !method.IsConstant() }
	require.NotPanics(t, func() {
		// No value and no fallback: the empty call reverts.
		_, _, err = cmn.SetupABI(contractABI, contract, true, isTransaction)
	})
	require.ErrorIs(t, err, vm.ErrExecutionReverted)
}
