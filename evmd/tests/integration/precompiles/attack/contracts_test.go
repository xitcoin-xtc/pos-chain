package attack

// Adversarial contracts against the precompiles, on the real application
// through signed transactions (ante handler, EVM, StateDB commit, block
// commit). The contracts are in precompiles/testutil/contracts.

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"

	abcitypes "github.com/cometbft/cometbft/abci/types"

	"github.com/xitcoin-xtc/pos-chain/precompiles/gov"
	testcontracts "github.com/xitcoin-xtc/pos-chain/precompiles/testutil/contracts"
	testconstants "github.com/xitcoin-xtc/pos-chain/testutil/constants"
	"github.com/xitcoin-xtc/pos-chain/testutil/integration/base/factory"
	evmfactory "github.com/xitcoin-xtc/pos-chain/testutil/integration/evm/factory"
	testutiltypes "github.com/xitcoin-xtc/pos-chain/testutil/types"
	erc20types "github.com/xitcoin-xtc/pos-chain/x/erc20/types"
	evmtypes "github.com/xitcoin-xtc/pos-chain/x/vm/types"

	"cosmossdk.io/collections"
	sdkmath "cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

var oneXTC = big.NewInt(1e18)

func loadContract(tb testing.TB, load func() (evmtypes.CompiledContract, error)) evmtypes.CompiledContract {
	tb.Helper()
	contract, err := load()
	require.NoError(tb, err)
	return contract
}

// includeEthTx signs an Ethereum transaction and includes it alone in a
// committed block. It returns the result and the reason of a failure:
// rejected by the ante handler or the message handler, or reverted in the
// EVM.
func (c *chain) includeEthTx(tb testing.TB, key testKey, txArgs evmtypes.EvmTxArgs) (abcitypes.ExecTxResult, error) {
	tb.Helper()
	tx, err := c.factory.GenerateSignedEthTx(key.priv, txArgs)
	require.NoError(tb, err)
	bz, err := c.app.GetTxConfig().TxEncoder()(tx)
	require.NoError(tb, err)
	block, err := c.net.NextBlockWithTxs(bz)
	require.NoError(tb, err)
	require.Len(tb, block.TxResults, 1)
	res := *block.TxResults[0]
	if res.Code != 0 {
		return res, fmt.Errorf("transaction failed (code %d): %s", res.Code, res.Log)
	}
	ethRes, err := evmtypes.DecodeTxResponse(res.Data)
	require.NoError(tb, err)
	if ethRes.Failed() {
		return res, fmt.Errorf("EVM execution failed: %s", ethRes.VmError)
	}
	return res, nil
}

func (c *chain) deploy(tb testing.TB, key testKey, contract evmtypes.CompiledContract, value *big.Int, args ...interface{}) common.Address {
	tb.Helper()
	txArgs, err := c.factory.GenerateDeployContractArgs(key.addr, evmtypes.EvmTxArgs{Amount: value, GasLimit: 5_000_000},
		testutiltypes.ContractDeploymentData{Contract: contract, ConstructorArgs: args})
	require.NoError(tb, err)
	_, err = c.includeEthTx(tb, key, txArgs)
	require.NoError(tb, err)
	return crypto.CreateAddress(key.addr, txArgs.Nonce)
}

// send includes a contract call in its own block and reports whether it
// succeeded.
func (c *chain) send(tb testing.TB, key testKey, to common.Address, contractABI abi.ABI, value *big.Int, gasLimit uint64, method string, args ...interface{}) (abcitypes.ExecTxResult, error) {
	tb.Helper()
	input, err := evmfactory.GenerateContractCallArgs(testutiltypes.CallArgs{ContractABI: contractABI, MethodName: method, Args: args})
	require.NoError(tb, err)
	return c.includeEthTx(tb, key, evmtypes.EvmTxArgs{To: &to, Amount: value, GasLimit: gasLimit, Input: input})
}

func (c *chain) query(tb testing.TB, to common.Address, contractABI abi.ABI, method string, args ...interface{}) []interface{} {
	tb.Helper()
	res, err := c.factory.QueryContract(evmtypes.EvmTxArgs{To: &to}, testutiltypes.CallArgs{ContractABI: contractABI, MethodName: method, Args: args}, 0)
	require.NoError(tb, err)
	out, err := contractABI.Unpack(method, res.Ret)
	require.NoError(tb, err)
	return out
}

func (c *chain) delegatedTokens(tb testing.TB, delegator common.Address, validator string) sdkmath.Int {
	tb.Helper()
	ctx := c.net.GetContext()
	valAddr, err := sdk.ValAddressFromBech32(validator)
	require.NoError(tb, err)
	delegation, err := c.app.StakingKeeper.GetDelegation(ctx, delegator.Bytes(), valAddr)
	if err != nil {
		require.ErrorIs(tb, err, stakingtypes.ErrNoDelegation)
		return sdkmath.ZeroInt()
	}
	val, err := c.app.StakingKeeper.GetValidator(ctx, valAddr)
	require.NoError(tb, err)
	return val.TokensFromShares(delegation.Shares).TruncateInt()
}

func (c *chain) bankBalance(tb testing.TB, address common.Address) *big.Int {
	tb.Helper()
	return c.app.BankKeeper.GetBalance(c.net.GetContext(), address.Bytes(), evmtypes.GetEVMCoinDenom()).Amount.BigInt()
}

// requireInvariants checks that the EVM and the bank agree on the balances
// of the given accounts, that the bond-denomination supply equals the sum
// of all balances, and that the staking pools hold exactly the tokens of the
// validators and unbonding delegations they back.
func (c *chain) requireInvariants(tb testing.TB, accounts ...common.Address) {
	tb.Helper()
	ctx := c.net.GetContext()
	denom := c.bondDenom(tb)
	for _, account := range accounts {
		evmBalance := c.app.EVMKeeper.GetBalance(ctx, account).ToBig()
		bank := c.app.BankKeeper.GetBalance(ctx, account.Bytes(), evmtypes.GetEVMCoinExtendedDenom()).Amount.BigInt()
		require.Equal(tb, bank.String(), evmBalance.String(), "EVM and bank disagree on the balance of %s", account)
	}

	sum := sdkmath.ZeroInt()
	for _, balance := range c.app.BankKeeper.GetAccountsBalances(ctx) {
		sum = sum.Add(balance.Coins.AmountOf(denom))
	}
	require.Equal(tb, c.app.BankKeeper.GetSupply(ctx, denom).Amount.String(), sum.String(), "supply != sum of balances")

	bonded, notBonded := sdkmath.ZeroInt(), sdkmath.ZeroInt()
	validators, err := c.app.StakingKeeper.GetAllValidators(ctx)
	require.NoError(tb, err)
	for _, validator := range validators {
		if validator.IsBonded() {
			bonded = bonded.Add(validator.Tokens)
		} else {
			notBonded = notBonded.Add(validator.Tokens)
		}
	}
	require.NoError(tb, c.app.StakingKeeper.IterateUnbondingDelegations(ctx, func(_ int64, ubd stakingtypes.UnbondingDelegation) bool {
		for _, entry := range ubd.Entries {
			notBonded = notBonded.Add(entry.Balance)
		}
		return false
	}))
	pool := func(name string) string {
		return c.app.BankKeeper.GetBalance(ctx, authtypes.NewModuleAddress(name), denom).Amount.String()
	}
	require.Equal(tb, bonded.String(), pool(stakingtypes.BondedPoolName), "bonded pool")
	require.Equal(tb, notBonded.String(), pool(stakingtypes.NotBondedPoolName), "not-bonded pool")
}

// A revert after a precompile call undoes the Cosmos side of the call
// together with the EVM side, whether the whole transaction reverts or the
// reverting frame is caught.
func TestRevertAfterPrecompileCallLeavesNoTrace(t *testing.T) {
	c := newChain(t)
	attackerABI := loadContract(t, testcontracts.LoadPrecompileAttackerContract)
	attacker := c.deploy(t, c.users[0], attackerABI, new(big.Int).Mul(big.NewInt(10), oneXTC))
	validator := c.net.GetValidators()[0].OperatorAddress
	recipient := c.users[2].addr
	amount := big.NewInt(3e17)
	initialAttacker, initialRecipient := c.bankBalance(t, attacker), c.bankBalance(t, recipient)

	_, err := c.send(t, c.users[0], attacker, attackerABI.ABI, nil, 1_000_000, "delegateThenRevert", validator, amount)
	require.Error(t, err)
	_, err = c.send(t, c.users[0], attacker, attackerABI.ABI, nil, 1_000_000, "sendDelegateThenRevert", recipient, oneXTC, validator, amount)
	require.Error(t, err)
	require.True(t, c.delegatedTokens(t, attacker, validator).IsZero(), "a reverted delegation persisted")
	require.Equal(t, initialAttacker, c.bankBalance(t, attacker))
	require.Equal(t, initialRecipient, c.bankBalance(t, recipient))
	c.requireInvariants(t, attacker, recipient)

	_, err = c.send(t, c.users[0], attacker, attackerABI.ABI, nil, 2_000_000, "delegateAfterCaughtReverts", recipient, oneXTC, validator, amount)
	require.NoError(t, err)
	require.Equal(t, amount.String(), c.delegatedTokens(t, attacker, validator).String(), "only the delegation outside the reverted frames persists")
	require.Equal(t, new(big.Int).Sub(initialAttacker, amount).String(), c.bankBalance(t, attacker).String())
	require.Equal(t, initialRecipient, c.bankBalance(t, recipient))
	c.requireInvariants(t, attacker, recipient)
}

// Re-entrant call frames that mix precompile calls, native transfers and
// reverts: exactly the frames whose whole ancestry did not revert persist,
// on both the Cosmos and the EVM side.
func TestReentrantFramesKeepCosmosAndEVMStateInSync(t *testing.T) {
	c := newChain(t)
	attackerABI := loadContract(t, testcontracts.LoadPrecompileAttackerContract)
	attacker := c.deploy(t, c.users[0], attackerABI, new(big.Int).Mul(big.NewInt(20), oneXTC))
	validator := c.net.GetValidators()[1].OperatorAddress
	recipient := c.users[2].addr
	amount := big.NewInt(1e15)
	const depth = 5

	delegated := big.NewInt(0)
	completed := int64(0)
	for _, mask := range []uint64{0, 0b000001, 0b000100, 0b001010, 0b011111, 0b010101} {
		survivors := int64(0)
		for frame := 0; frame <= depth; frame++ {
			if mask>>uint(frame) == 0 { // no reverting frame at this depth or above
				survivors++
			}
		}
		recipientBefore, attackerBefore := c.bankBalance(t, recipient), c.bankBalance(t, attacker)
		_, err := c.send(t, c.users[0], attacker, attackerABI.ABI, nil, 3_000_000, "reenter",
			big.NewInt(depth), new(big.Int).SetUint64(mask), recipient, validator, amount)
		require.NoError(t, err, "mask %b", mask)

		delegated.Add(delegated, new(big.Int).Mul(amount, big.NewInt(survivors)))
		completed += survivors
		require.Equal(t, delegated.String(), c.delegatedTokens(t, attacker, validator).String(), "mask %b", mask)
		require.Equal(t, new(big.Int).Add(recipientBefore, big.NewInt(survivors)).String(), c.bankBalance(t, recipient).String(), "mask %b", mask)
		// The attacker pays the surviving delegations and transfers; adding
		// to an existing delegation also withdraws its accrued rewards to it.
		spent := new(big.Int).Add(new(big.Int).Mul(amount, big.NewInt(survivors)), big.NewInt(survivors))
		rewards := new(big.Int).Sub(c.bankBalance(t, attacker), new(big.Int).Sub(attackerBefore, spent))
		require.True(t, rewards.Sign() >= 0 && rewards.Cmp(amount) < 0, "mask %b: unexplained balance change %s", mask, rewards)
		require.Equal(t, big.NewInt(completed), c.query(t, attacker, attackerABI.ABI, "completed")[0], "mask %b", mask)
		c.requireInvariants(t, attacker, recipient)
	}
}

// GHSA-wm4f-qpxp-rch4: at most 20 precompile calls per transaction.
// Rejected calls do not advance the counter (no uint8 wraparound after 256
// attempts) and reverted calls still count.
func TestPrecompileCallLimitCannotBeBypassed(t *testing.T) {
	c := newChain(t)
	attackerABI := loadContract(t, testcontracts.LoadPrecompileAttackerContract)
	attacker := c.deploy(t, c.users[0], attackerABI, new(big.Int).Mul(big.NewInt(10), oneXTC))
	validator := c.net.GetValidators()[0].OperatorAddress
	amount := big.NewInt(1e15)
	require.EqualValues(t, 20, evmtypes.MaxPrecompileCalls)

	for _, tc := range []struct {
		name     string
		attempts int64
		reverted bool
		want     bool
	}{
		{"19 calls, then the 20th delegates", 19, false, true},
		{"20 calls, then a 21st is refused", 20, false, false},
		{"20 calls in reverted frames still count", 20, true, false},
		{"256+5 attempts do not wrap the counter", 261, false, false},
		{"512+3 attempts in reverted frames", 515, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := c.delegatedTokens(t, attacker, validator)
			_, err := c.send(t, c.users[0], attacker, attackerABI.ABI, nil, 25_000_000, "attemptsThenDelegate",
				big.NewInt(tc.attempts), tc.reverted, validator, amount)
			require.NoError(t, err)
			require.Equal(t, tc.want, c.query(t, attacker, attackerABI.ABI, "lastDelegationSucceeded")[0])
			want := before
			if tc.want {
				want = before.Add(sdkmath.NewIntFromBigInt(amount))
			}
			require.Equal(t, want.String(), c.delegatedTokens(t, attacker, validator).String())
			c.requireInvariants(t, attacker)
		})
	}
}

// GHSA-c6xv-pppp-rjw6: a contract that holds a delegation or an unbonding
// delegation cannot self-destruct (EIP-6780, same transaction as its
// creation), so no staking claim outlives its account.
func TestSelfDestructWithStakingPositionsFails(t *testing.T) {
	c := newChain(t)
	factoryContract := loadContract(t, testcontracts.LoadStakingSelfDestructorFactoryContract)
	factoryAddress := c.deploy(t, c.users[0], factoryContract, nil)
	validator := c.net.GetValidators()[0].OperatorAddress
	valAddr, err := sdk.ValAddressFromBech32(validator)
	require.NoError(t, err)

	for _, tc := range []struct {
		name           string
		amount, unbond *big.Int
		wantOK         bool
	}{
		{"no staking position: the account is deleted", big.NewInt(0), big.NewInt(0), true},
		{"delegation", oneXTC, big.NewInt(0), false},
		{"delegation and unbonding delegation", oneXTC, big.NewInt(4e17), false},
		{"unbonding delegation only", oneXTC, oneXTC, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			salt := crypto.Keccak256Hash([]byte(tc.name))
			child := c.query(t, factoryAddress, factoryContract.ABI, "predict", salt)[0].(common.Address)
			res, err := c.send(t, c.users[1], factoryAddress, factoryContract.ABI, new(big.Int).Mul(big.NewInt(2), oneXTC), 3_000_000,
				"createStakeAndDestroy", salt, validator, tc.amount, tc.unbond)
			if tc.wantOK {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.Contains(t, err.Error()+res.Log, "staking positions")
			}
			ctx := c.net.GetContext()
			require.Nil(t, c.app.AccountKeeper.GetAccount(ctx, child.Bytes()), "the child account survived")
			_, err = c.app.StakingKeeper.GetDelegation(ctx, child.Bytes(), valAddr)
			require.ErrorIs(t, err, stakingtypes.ErrNoDelegation, "orphan delegation")
			_, err = c.app.StakingKeeper.GetUnbondingDelegation(ctx, child.Bytes(), valAddr)
			require.ErrorIs(t, err, stakingtypes.ErrNoUnbondingDelegation, "orphan unbonding delegation")
			c.requireInvariants(t, c.users[1].addr, factoryAddress)
		})
	}
}

// View functions call precompiles with STATICCALL; a transaction made of
// view calls must not change the gov or distribution stores (the tally
// used to delete votes, the reward queries to increment validator periods).
func TestViewCallsDoNotChangeCosmosState(t *testing.T) {
	c := newChain(t)
	attackerABI := loadContract(t, testcontracts.LoadPrecompileAttackerContract)
	attacker := c.deploy(t, c.users[0], attackerABI, new(big.Int).Mul(big.NewInt(10), oneXTC))
	validator := c.net.GetValidators()[0]
	_, err := c.send(t, c.users[0], attacker, attackerABI.ABI, nil, 2_000_000, "delegateAfterCaughtReverts", c.users[2].addr, oneXTC, validator.OperatorAddress, oneXTC)
	require.NoError(t, err)
	c.commitCosmosTx(t, c.users[0], govv1.NewMsgVote(c.users[0].acc, 1, govv1.OptionYes, ""))
	c.commitCosmosTx(t, c.users[1], govv1.NewMsgVote(c.users[1].acc, 1, govv1.OptionNo, ""))
	require.NoError(t, c.net.NextBlock()) // rewards accrue

	snapshot := func() (int, uint64) {
		ctx := c.net.GetContext()
		iterator, err := c.app.GovKeeper.Votes.Iterate(ctx, collections.NewPrefixedPairRange[uint64, sdk.AccAddress](1))
		require.NoError(t, err)
		votes, err := iterator.Keys()
		require.NoError(t, err)
		valBz, err := sdk.ValAddressFromBech32(validator.OperatorAddress)
		require.NoError(t, err)
		current, err := c.app.DistrKeeper.GetValidatorCurrentRewards(ctx, valBz)
		require.NoError(t, err)
		return len(votes), current.Period
	}
	votes, period := snapshot()
	require.Equal(t, 2, votes)

	_, err = c.send(t, c.users[1], attacker, attackerABI.ABI, nil, 2_000_000, "readViews", uint64(1))
	require.NoError(t, err)
	gotVotes, gotPeriod := snapshot()
	require.Equal(t, votes, gotVotes, "a view call deleted votes")
	require.Equal(t, period, gotPeriod, "a view call incremented the validator period")

	// The same query sent directly by an account (CALL, not STATICCALL).
	govAddress := common.HexToAddress(evmtypes.GovPrecompileAddress)
	_, err = c.send(t, c.users[1], govAddress, gov.ABI, nil, 500_000, gov.GetTallyResultMethod, uint64(1))
	require.NoError(t, err)
	gotVotes, _ = snapshot()
	require.Equal(t, votes, gotVotes, "a direct tally call deleted votes")
}

// GHSA-p8x8-4gg6-prcc: an internal EVM call made by a module
// (CallEVMWithData, here the ERC-20 transfer of MsgConvertERC20) is limited
// by the gas left in the transaction, and its gas is charged even when the
// StateDB commit fails after the EVM succeeded. Before the fix the internal
// call ran with 25,000,000 gas and none of it was charged on that path.
func TestInternalCallGasIsCappedAndCharged(t *testing.T) {
	erc20Genesis := erc20types.DefaultGenesisState()
	erc20Genesis.Params.PermissionlessRegistration = true
	erc20Genesis.TokenPairs = testconstants.ExampleTokenPairs
	erc20Genesis.NativePrecompiles = []string{testconstants.WEVMOSContractMainnet}
	c := newChain(t, map[string]interface{}{erc20types.ModuleName: erc20Genesis})
	token := loadContract(t, testcontracts.LoadGasBurningERC20Contract)
	moduleAccount := common.BytesToAddress(authtypes.NewModuleAddress(erc20types.ModuleName))

	convert := func(tokenAddress common.Address, gasLimit uint64) abcitypes.ExecTxResult {
		res, err := c.factory.CommitCosmosTx(c.users[1].priv, factory.CosmosTxArgs{
			Gas: &gasLimit,
			Msgs: []sdk.Msg{&erc20types.MsgConvertERC20{
				ContractAddress: tokenAddress.Hex(), Amount: sdkmath.NewInt(1),
				Receiver: c.users[1].acc.String(), Sender: c.users[1].addr.Hex(),
			}},
		})
		require.NoError(t, err)
		require.NotZero(t, res.Code, "the conversion must fail: the commit refuses the transfer to the module account")
		require.LessOrEqual(t, res.GasUsed, int64(gasLimit)) //nolint:gosec // G115: test gas limits fit in int64
		return res
	}
	register := func(burnGas int64) common.Address {
		tokenAddress := c.deploy(t, c.users[0], token, oneXTC, moduleAccount, big.NewInt(burnGas))
		c.commitCosmosTx(t, c.users[0], &erc20types.MsgRegisterERC20{Signer: c.users[0].acc.String(), Erc20Addresses: []string{tokenAddress.Hex()}})
		return tokenAddress
	}

	t.Run("gas burned before a failing commit is charged", func(t *testing.T) {
		const burn = 1_000_000
		tokenAddress := register(burn)
		res := convert(tokenAddress, 3_000_000)
		require.Contains(t, res.Log, "not allowed to receive funds", "the StateDB commit must be what fails")
		require.GreaterOrEqual(t, res.GasUsed, int64(burn), "the internal EVM gas was dropped: used %d", res.GasUsed)
		c.requireInvariants(t, tokenAddress, moduleAccount)
	})

	t.Run("the internal call cannot use more gas than the transaction has", func(t *testing.T) {
		tokenAddress := register(100_000_000)
		const gasLimit = 700_000
		res := convert(tokenAddress, gasLimit)
		require.GreaterOrEqual(t, res.GasUsed, int64(gasLimit*9/10), "the internal EVM gas was not charged: used %d of %d", res.GasUsed, gasLimit)
		c.requireInvariants(t, tokenAddress, moduleAccount)
	})
}
