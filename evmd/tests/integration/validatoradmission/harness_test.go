package validatoradmission

// Shared harness of the validator admission attack simulations: a real evmd
// application (ante handler, message router, staking hooks, EVM and
// precompiles) with deterministic keys, built either as release binaries
// build it (executable governance proposals disabled) or with proposals
// enabled as the legacy integration suites run it.

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"

	evm "github.com/xitcoin-xtc/pos-chain"
	"github.com/xitcoin-xtc/pos-chain/crypto/ethsecp256k1"
	"github.com/xitcoin-xtc/pos-chain/evmd"
	"github.com/xitcoin-xtc/pos-chain/evmd/tests/integration"
	cmn "github.com/xitcoin-xtc/pos-chain/precompiles/common"
	govprecompile "github.com/xitcoin-xtc/pos-chain/precompiles/gov"
	slashingprecompile "github.com/xitcoin-xtc/pos-chain/precompiles/slashing"
	stakingprecompile "github.com/xitcoin-xtc/pos-chain/precompiles/staking"
	testcontracts "github.com/xitcoin-xtc/pos-chain/precompiles/testutil/contracts"
	testconstants "github.com/xitcoin-xtc/pos-chain/testutil/constants"
	"github.com/xitcoin-xtc/pos-chain/testutil/integration/base/factory"
	evmfactory "github.com/xitcoin-xtc/pos-chain/testutil/integration/evm/factory"
	"github.com/xitcoin-xtc/pos-chain/testutil/integration/evm/grpc"
	"github.com/xitcoin-xtc/pos-chain/testutil/integration/evm/network"
	testutiltypes "github.com/xitcoin-xtc/pos-chain/testutil/types"
	validatoradmissiontypes "github.com/xitcoin-xtc/pos-chain/x/validatoradmission/types"
	evmtypes "github.com/xitcoin-xtc/pos-chain/x/vm/types"

	dbm "github.com/cosmos/cosmos-db"

	"cosmossdk.io/log/v2"
	sdkmath "cosmossdk.io/math"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

const (
	bondDenom = testconstants.ExampleAttoDenom
	// txGas is fixed so that a transaction rejected by the ante handler is
	// still built and delivered (gas estimation would fail first).
	txGas = uint64(3_000_000)
)

// minimumSelfDelegation is the admission minimum of the test chains:
// 1,000 tokens of 18 decimals, above the 1-token genesis validators.
var minimumSelfDelegation = sdkmath.NewInt(1_000).Mul(sdkmath.NewInt(1e18))

type testKey struct {
	priv *ethsecp256k1.PrivKey
	acc  sdk.AccAddress
	addr common.Address
}

func deterministicKey(label string) testKey {
	seed := sha256.Sum256([]byte("validator-admission-attack/" + label))
	priv := &ethsecp256k1.PrivKey{Key: seed[:]}
	acc := sdk.AccAddress(priv.PubKey().Address())
	return testKey{priv: priv, acc: acc, addr: common.BytesToAddress(acc)}
}

func (k testKey) valoper() string { return sdk.ValAddress(k.acc).String() }

type chain struct {
	t          *testing.T
	governance bool
	net        *network.UnitTestNetwork
	app        *evmd.EVMD
	factory    evmfactory.TxFactory

	// authority is the admission authority.
	authority testKey
	// attacker was never approved.
	attacker testKey
	// candidate is approved but has no validator yet.
	candidate testKey
	// operator is approved and runs a validator created after genesis.
	operator testKey
	// delegated was never approved; tests give it EIP-7702 code.
	delegated testKey
	// genesisOperators run the genesis validators.
	genesisOperators []testKey
}

// newChain starts a fresh application. With governance set, executable
// proposals are enabled (as in the legacy integration suites); without it,
// the application is built as release binaries build it.
func newChain(t *testing.T, governance bool) *chain {
	t.Helper()
	c := &chain{
		t:          t,
		governance: governance,
		authority:  deterministicKey("authority"),
		attacker:   deterministicKey("attacker"),
		candidate:  deterministicKey("candidate"),
		operator:   deterministicKey("operator"),
		delegated:  deterministicKey("delegated"),
	}
	for i := 0; i < 3; i++ {
		c.genesisOperators = append(c.genesisOperators, deterministicKey(fmt.Sprintf("genesis-operator-%d", i)))
	}

	approved := []string{c.candidate.valoper(), c.operator.valoper()}
	operatorAccounts := make([]sdk.AccAddress, 0, len(c.genesisOperators))
	for _, operator := range c.genesisOperators {
		approved = append(approved, operator.valoper())
		operatorAccounts = append(operatorAccounts, operator.acc)
	}
	admission := validatoradmissiontypes.GenesisState{
		Authority:             c.authority.acc.String(),
		ApprovedValidators:    approved,
		MaxApprovedValidators: validatoradmissiontypes.DefaultMaxApprovedValidators,
		MinimumSelfDelegation: minimumSelfDelegation.String() + bondDenom,
	}

	home := t.TempDir()
	create := func(chainID string, evmChainID uint64, customBaseAppOptions ...func(*baseapp.BaseApp)) evm.EvmApp {
		evmd.EnableGovernanceProposalSubmissionForTests(governance)
		dir, err := os.MkdirTemp(home, "evmd")
		if err != nil {
			panic(err)
		}
		app := evmd.NewExampleApp(
			log.NewNopLogger(), dbm.NewMemDB(), true,
			integration.NewAppOptionsWithFlagHomeAndChainID(filepath.Clean(dir), evmChainID),
			append(customBaseAppOptions, baseapp.SetChainID(chainID))...,
		)
		clientCtx := client.Context{}.WithChainID(testconstants.ExampleChainID.ChainID).WithHeight(1).WithTxConfig(app.GetTxConfig())
		if m, ok := app.GetMempool().(interface{ SetClientCtx(client.Context) }); ok && m != nil {
			m.SetClientCtx(clientCtx)
		}
		return app
	}

	c.net = network.NewUnitTestNetwork(create,
		network.WithPreFundedAccounts(c.authority.acc, c.attacker.acc, c.candidate.acc, c.operator.acc, c.delegated.acc),
		network.WithAmountOfValidators(len(c.genesisOperators)),
		network.WithValidatorOperators(operatorAccounts),
		network.WithCustomGenesis(network.CustomGenesisState{validatoradmissiontypes.ModuleName: admission}),
	)
	app, ok := c.net.App.(*evmd.EVMD)
	require.True(t, ok, "unexpected application type %T", c.net.App)
	c.app = app
	c.factory = evmfactory.New(c.net, grpc.NewIntegrationHandler(c.net))
	require.Equal(t, minimumSelfDelegation.String()+bondDenom, c.app.ValidatorAdmissionKeeper.GetMinimumSelfDelegation(c.ctx()))
	// The genesis operators hold only their bonded tokens; give them fees
	// for their votes.
	for _, operator := range c.genesisOperators {
		c.mustCosmosTx(c.authority, banktypes.NewMsgSend(c.authority.acc, operator.acc, sdk.NewCoins(sdk.NewCoin(bondDenom, sdkmath.NewInt(10).Mul(sdkmath.NewInt(1e18))))))
	}
	return c
}

func (c *chain) ctx() sdk.Context { return c.net.GetContext() }

// cosmosTx signs msgs with key and includes them alone in a committed block.
// It returns the error of a rejected or failed transaction.
func (c *chain) cosmosTx(key testKey, msgs ...sdk.Msg) error {
	c.t.Helper()
	gas := txGas
	res, err := c.factory.CommitCosmosTx(key.priv, factory.CosmosTxArgs{Msgs: msgs, Gas: &gas})
	require.NoError(c.t, err, "building or delivering the transaction failed")
	if res.Code != 0 {
		return fmt.Errorf("transaction failed (code %d): %s", res.Code, res.Log)
	}
	return nil
}

func (c *chain) mustCosmosTx(key testKey, msgs ...sdk.Msg) {
	c.t.Helper()
	require.NoError(c.t, c.cosmosTx(key, msgs...))
}

// ethTx signs an Ethereum transaction and includes it alone in a committed
// block. It returns the decoded response and the error of a transaction that
// was rejected, failed or reverted.
func (c *chain) ethTx(key testKey, args evmtypes.EvmTxArgs) (*evmtypes.MsgEthereumTxResponse, error) {
	c.t.Helper()
	if args.GasLimit == 0 {
		args.GasLimit = txGas
	}
	tx, err := c.factory.GenerateSignedEthTx(key.priv, args)
	require.NoError(c.t, err)
	bz, err := c.app.GetTxConfig().TxEncoder()(tx)
	require.NoError(c.t, err)
	block, err := c.net.NextBlockWithTxs(bz)
	require.NoError(c.t, err)
	require.Len(c.t, block.TxResults, 1)
	res := block.TxResults[0]
	if res.Code != 0 {
		return nil, fmt.Errorf("transaction failed (code %d): %s", res.Code, res.Log)
	}
	ethRes, err := evmtypes.DecodeTxResponse(res.Data)
	require.NoError(c.t, err)
	if ethRes.Failed() {
		return ethRes, fmt.Errorf("EVM execution failed: %s", ethRes.VmError)
	}
	return ethRes, nil
}

func (c *chain) call(key testKey, to common.Address, input []byte) (*evmtypes.MsgEthereumTxResponse, error) {
	c.t.Helper()
	return c.ethTx(key, evmtypes.EvmTxArgs{To: &to, Input: input})
}

func (c *chain) nextBlock() {
	c.t.Helper()
	require.NoError(c.t, c.net.NextBlock())
}

// ---------------------------------------------------------------------------
// Contracts and EIP-7702.

const (
	modeCall         = uint8(0)
	modeDelegateCall = uint8(1)
	modeCallCode     = uint8(2)
)

func attackerContract(t *testing.T) evmtypes.CompiledContract {
	t.Helper()
	contract, err := testcontracts.LoadAdmissionAttackerContract()
	require.NoError(t, err)
	return contract
}

// deployAttacker deploys an AdmissionAttacker funded with value.
func (c *chain) deployAttacker(key testKey, value *big.Int) common.Address {
	c.t.Helper()
	args, err := c.factory.GenerateDeployContractArgs(key.addr, evmtypes.EvmTxArgs{Amount: value, GasLimit: 5_000_000},
		testutiltypes.ContractDeploymentData{Contract: attackerContract(c.t)})
	require.NoError(c.t, err)
	_, err = c.ethTx(key, args)
	require.NoError(c.t, err)
	return crypto.CreateAddress(key.addr, args.Nonce)
}

// delegateCode makes key an EIP-7702 account running code (self-sponsored
// SetCode transaction).
func (c *chain) delegateCode(key testKey, code common.Address) {
	c.t.Helper()
	nonce := c.app.EVMKeeper.GetNonce(c.ctx(), key.addr)
	ecdsaKey, err := key.priv.ToECDSA()
	require.NoError(c.t, err)
	authorization, err := ethtypes.SignSetCode(ecdsaKey, ethtypes.SetCodeAuthorization{
		ChainID: *uint256.MustFromBig(c.net.GetEIP155ChainID()),
		Address: code,
		Nonce:   nonce + 1,
	})
	require.NoError(c.t, err)
	_, err = c.ethTx(key, evmtypes.EvmTxArgs{To: &common.Address{}, AuthorizationList: []ethtypes.SetCodeAuthorization{authorization}})
	require.NoError(c.t, err)
	stored := c.app.EVMKeeper.GetCode(c.ctx(), c.app.EVMKeeper.GetCodeHash(c.ctx(), key.addr))
	target, ok := ethtypes.ParseDelegation(stored)
	require.True(c.t, ok, "SetCode transaction did not delegate %s", key.addr)
	require.Equal(c.t, code, target)
}

func packAttacker(t *testing.T, method string, args ...interface{}) []byte {
	t.Helper()
	input, err := attackerContract(t).ABI.Pack(method, args...)
	require.NoError(t, err)
	return input
}

// forward makes the AdmissionAttacker code at at forward data to target
// with mode, in a transaction sent by sender. It reports whether the
// forwarded call succeeded (from the Forwarded event); the transaction
// itself never reverts.
func (c *chain) forward(sender testKey, at, target common.Address, data []byte, mode uint8) bool {
	c.t.Helper()
	res, err := c.call(sender, at, packAttacker(c.t, "forward", target, data, mode))
	require.NoError(c.t, err)
	return forwardedSuccess(c.t, res)
}

func forwardedSuccess(t *testing.T, res *evmtypes.MsgEthereumTxResponse) bool {
	t.Helper()
	event := attackerContract(t).ABI.Events["Forwarded"]
	var found []bool
	for _, entry := range res.Logs {
		if len(entry.Topics) == 0 || entry.Topics[0] != event.ID.String() {
			continue
		}
		values, err := event.Inputs.NonIndexed().Unpack(entry.Data)
		require.NoError(t, err)
		found = append(found, values[1].(bool))
	}
	require.Len(t, found, 1, "expected one Forwarded event")
	return found[0]
}

// arm stores the call that the AdmissionAttacker code at at makes from its
// IBC callbacks.
func (c *chain) arm(sender testKey, at, target common.Address, data []byte, mode uint8) {
	c.t.Helper()
	_, err := c.call(sender, at, packAttacker(c.t, "arm", target, data, mode))
	require.NoError(c.t, err)
}

// ---------------------------------------------------------------------------
// Precompile calldata.

var (
	stakingAddress  = common.HexToAddress(evmtypes.StakingPrecompileAddress)
	slashingAddress = common.HexToAddress(evmtypes.SlashingPrecompileAddress)
	govAddress      = common.HexToAddress(evmtypes.GovPrecompileAddress)
)

func consensusKey(label string) *ed25519.PrivKey {
	return ed25519.GenPrivKeyFromSecret([]byte("validator-admission-attack/consensus/" + label))
}

func createValidatorInput(t *testing.T, operator common.Address, label string, value, minSelf sdkmath.Int) []byte {
	t.Helper()
	input, err := stakingprecompile.ABI.Pack(stakingprecompile.CreateValidatorMethod,
		stakingprecompile.Description{Moniker: label},
		stakingprecompile.Commission{Rate: big.NewInt(1e17), MaxRate: big.NewInt(2e17), MaxChangeRate: big.NewInt(1e16)},
		minSelf.BigInt(), operator, base64.StdEncoding.EncodeToString(consensusKey(label).PubKey().Bytes()), value.BigInt())
	require.NoError(t, err)
	return input
}

func editValidatorInput(t *testing.T, operator common.Address, minSelf *big.Int) []byte {
	t.Helper()
	keep := "[do-not-modify]"
	input, err := stakingprecompile.ABI.Pack(stakingprecompile.EditValidatorMethod,
		stakingprecompile.Description{Moniker: keep, Identity: keep, Website: keep, SecurityContact: keep, Details: keep},
		operator, big.NewInt(-1), minSelf)
	require.NoError(t, err)
	return input
}

func delegateInput(t *testing.T, delegator common.Address, validator string, amount *big.Int) []byte {
	t.Helper()
	input, err := stakingprecompile.ABI.Pack(stakingprecompile.DelegateMethod, delegator, validator, amount)
	require.NoError(t, err)
	return input
}

func unjailInput(t *testing.T, operator common.Address) []byte {
	t.Helper()
	input, err := slashingprecompile.ABI.Pack(slashingprecompile.UnjailMethod, operator)
	require.NoError(t, err)
	return input
}

// submitProposalInput builds a governance precompile submitProposal call
// carrying msgs.
func (c *chain) submitProposalInput(proposer common.Address, msgs ...sdk.Msg) []byte {
	c.t.Helper()
	encoded := make([]json.RawMessage, 0, len(msgs))
	for _, msg := range msgs {
		bz, err := c.app.AppCodec().MarshalInterfaceJSON(msg)
		require.NoError(c.t, err)
		encoded = append(encoded, bz)
	}
	proposal, err := json.Marshal(map[string]interface{}{
		"messages": encoded, "metadata": "", "title": "attack", "summary": "attack", "expedited": false,
	})
	require.NoError(c.t, err)
	input, err := govprecompile.ABI.Pack(govprecompile.SubmitProposalMethod, proposer, proposal,
		[]cmn.Coin{{Denom: bondDenom, Amount: c.minDeposit().BigInt()}})
	require.NoError(c.t, err)
	return input
}

// ---------------------------------------------------------------------------
// Messages and state.

func (c *chain) createValidatorMsg(key testKey, label string, value, minSelf sdkmath.Int) *stakingtypes.MsgCreateValidator {
	c.t.Helper()
	msg, err := stakingtypes.NewMsgCreateValidator(key.valoper(), consensusKey(label).PubKey(), sdk.NewCoin(bondDenom, value),
		stakingtypes.Description{Moniker: label},
		stakingtypes.NewCommissionRates(sdkmath.LegacyNewDecWithPrec(1, 1), sdkmath.LegacyNewDecWithPrec(2, 1), sdkmath.LegacyNewDecWithPrec(1, 2)),
		minSelf)
	require.NoError(c.t, err)
	return msg
}

func (c *chain) validator(key testKey) (stakingtypes.Validator, bool) {
	c.t.Helper()
	validator, err := c.app.StakingKeeper.GetValidator(c.ctx(), sdk.ValAddress(key.acc))
	if err != nil {
		require.ErrorIs(c.t, err, stakingtypes.ErrNoValidatorFound)
		return stakingtypes.Validator{}, false
	}
	return validator, true
}

func (c *chain) requireNoValidator(key testKey) {
	c.t.Helper()
	_, found := c.validator(key)
	require.False(c.t, found, "a validator exists for %s", key.valoper())
}

func (c *chain) requireJailed(key testKey) {
	c.t.Helper()
	validator, found := c.validator(key)
	require.True(c.t, found, "no validator for %s", key.valoper())
	require.True(c.t, validator.IsJailed(), "validator %s is not jailed", key.valoper())
}

func (c *chain) mintParams() minttypes.Params {
	c.t.Helper()
	params, err := c.app.MintKeeper.Params.Get(c.ctx())
	require.NoError(c.t, err)
	return params
}

func (c *chain) nextProposalID() uint64 {
	c.t.Helper()
	id, err := c.app.GovKeeper.ProposalID.Peek(c.ctx())
	require.NoError(c.t, err)
	return id
}

func (c *chain) minDeposit() sdkmath.Int {
	c.t.Helper()
	params, err := c.app.GovKeeper.Params.Get(c.ctx())
	require.NoError(c.t, err)
	return params.MinDeposit[0].Amount
}

func (c *chain) govAddress() sdk.AccAddress {
	return authtypes.NewModuleAddress(govtypes.ModuleName)
}

// passProposal votes yes with every genesis operator and ends the voting
// period. It returns the final status.
func (c *chain) passProposal(id uint64) govv1.ProposalStatus {
	c.t.Helper()
	for _, operator := range c.genesisOperators {
		c.mustCosmosTx(operator, govv1.NewMsgVote(operator.acc, id, govv1.OptionYes, ""))
	}
	params, err := c.app.GovKeeper.Params.Get(c.ctx())
	require.NoError(c.t, err)
	require.NoError(c.t, c.net.NextBlockAfter(*params.VotingPeriod))
	c.nextBlock()
	proposal, err := c.app.GovKeeper.Proposals.Get(c.ctx(), id)
	require.NoError(c.t, err)
	return proposal.Status
}
