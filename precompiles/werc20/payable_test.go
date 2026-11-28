package werc20

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"

	erc20types "github.com/xitcoin-xtc/pos-chain/x/erc20/types"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// Value sent with a method that is not payable used to be accepted and kept
// at the precompile address, where nothing can spend it: Deposit returns only
// the value of its own call. Found by FuzzWERC20PrecompileCalldata.
func TestNonPayableMethodsRejectValue(t *testing.T) {
	pair := erc20types.NewTokenPair(common.HexToAddress("0xEeeeeEeeeEeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE"), "axtc", erc20types.OWNER_MODULE)
	p := NewPrecompile(pair, nil, nil, nil)
	holder := common.HexToAddress("0x1000000000000000000000000000000000000001")
	calls := map[string][]interface{}{
		"balanceOf":   {holder},
		"totalSupply": {},
		"name":        {},
		"decimals":    {},
		"transfer":    {holder, big.NewInt(1)},
		"approve":     {holder, big.NewInt(1)},
		"withdraw":    {big.NewInt(1)},
	}
	for name, args := range calls {
		t.Run(name, func(t *testing.T) {
			method := p.Methods[name]
			require.False(t, method.IsPayable())
			input, err := p.Pack(name, args...)
			require.NoError(t, err)
			contract := vm.NewPrecompile(holder, p.Address(), uint256.NewInt(1), 1_000_000)
			contract.Input = input
			_, err = p.Execute(sdk.Context{}, nil, contract, false)
			require.ErrorContains(t, err, "cannot receive funds")
		})
	}
	for _, name := range []string{"deposit"} {
		require.True(t, p.Methods[name].IsPayable(), name)
	}
	require.True(t, p.Receive.IsPayable())
	require.True(t, p.Fallback.IsPayable())
}
