package attack

// Shared harness of the precompile attack simulations: a real evmd
// application built as release binaries build it (executable governance
// proposals disabled), with deterministic keys so that a failing fuzz input
// reproduces in another process.

import (
	"crypto/sha256"
	"fmt"
	"math/big"
	"os"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	evm "github.com/xitcoin-xtc/pos-chain"
	"github.com/xitcoin-xtc/pos-chain/crypto/ethsecp256k1"
	"github.com/xitcoin-xtc/pos-chain/evmd"
	"github.com/xitcoin-xtc/pos-chain/evmd/tests/integration"
	testconstants "github.com/xitcoin-xtc/pos-chain/testutil/constants"
	"github.com/xitcoin-xtc/pos-chain/testutil/integration/base/factory"
	evmfactory "github.com/xitcoin-xtc/pos-chain/testutil/integration/evm/factory"
	"github.com/xitcoin-xtc/pos-chain/testutil/integration/evm/grpc"
	"github.com/xitcoin-xtc/pos-chain/testutil/integration/evm/network"
	evmtypes "github.com/xitcoin-xtc/pos-chain/x/vm/types"

	dbm "github.com/cosmos/cosmos-db"

	"cosmossdk.io/log/v2"
	sdkmath "cosmossdk.io/math"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/store/v2/rootmulti"
	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

// testKey is a deterministic account key.
type testKey struct {
	priv *ethsecp256k1.PrivKey
	acc  sdk.AccAddress
	addr common.Address
}

func deterministicKey(label string) testKey {
	seed := sha256.Sum256([]byte("precompile-attack/" + label))
	priv := &ethsecp256k1.PrivKey{Key: seed[:]}
	acc := sdk.AccAddress(priv.PubKey().Address())
	return testKey{priv: priv, acc: acc, addr: common.BytesToAddress(acc)}
}

// newReleaseEvmd builds the application like integration.CreateEvmd, but
// with executable governance proposals disabled as in release binaries.
func newReleaseEvmd(chainID string, evmChainID uint64, customBaseAppOptions ...func(*baseapp.BaseApp)) evm.EvmApp {
	home, err := os.MkdirTemp("", "evmd-attack-home")
	if err != nil {
		panic(err)
	}
	evmd.EnableGovernanceProposalSubmissionForTests(false)
	app := evmd.NewExampleApp(
		log.NewNopLogger(), dbm.NewMemDB(), true,
		integration.NewAppOptionsWithFlagHomeAndChainID(home, evmChainID),
		append(customBaseAppOptions, baseapp.SetChainID(chainID))...,
	)
	clientCtx := client.Context{}.WithChainID(testconstants.ExampleChainID.ChainID).WithHeight(1).WithTxConfig(app.GetTxConfig())
	if m, ok := app.GetMempool().(interface{ SetClientCtx(client.Context) }); ok && m != nil {
		m.SetClientCtx(clientCtx)
	}
	return app
}

type chain struct {
	net     *network.UnitTestNetwork
	app     *evmd.EVMD
	factory evmfactory.TxFactory
	grpc    grpc.Handler
	users   []testKey
	// operators are the validator operator accounts, in the order of
	// net.GetValidators().
	operators []testKey
}

// newChain starts a fresh application with three funded users, three
// validators with deterministic operators, and one governance proposal in
// its voting period (submitted at genesis: release binaries reject new
// proposals).
func newChain(tb testing.TB, extraGenesis ...network.CustomGenesisState) *chain {
	tb.Helper()
	users := []testKey{deterministicKey("user-0"), deterministicKey("user-1"), deterministicKey("user-2")}
	operators := []testKey{deterministicKey("operator-0"), deterministicKey("operator-1"), deterministicKey("operator-2")}

	now := time.Now().UTC()
	end := now.Add(48 * time.Hour)
	deposit := sdk.NewCoins(sdk.NewCoin(testconstants.ExampleAttoDenom, sdkmath.NewInt(100)))
	gov := govv1.DefaultGenesisState()
	gov.StartingProposalId = 2
	gov.Params.MinDeposit = deposit
	gov.Params.ProposalCancelDest = users[2].acc.String()
	gov.Proposals = []*govv1.Proposal{{
		Id: 1, Status: govv1.ProposalStatus_PROPOSAL_STATUS_VOTING_PERIOD,
		SubmitTime: &now, DepositEndTime: &end, VotingStartTime: &now, VotingEndTime: &end,
		FinalTallyResult: &govv1.TallyResult{YesCount: "0", AbstainCount: "0", NoCount: "0", NoWithVetoCount: "0"},
		Title:            "text", Summary: "text", Proposer: users[0].acc.String(), Messages: []*codectypes.Any{},
	}}
	gov.Deposits = []*govv1.Deposit{{ProposalId: 1, Depositor: users[0].acc.String(), Amount: deposit}}

	accounts := make([]sdk.AccAddress, 0, len(users))
	for _, user := range users {
		accounts = append(accounts, user.acc)
	}
	operatorAccounts := make([]sdk.AccAddress, 0, len(operators))
	for _, operator := range operators {
		operatorAccounts = append(operatorAccounts, operator.acc)
	}
	genesis := network.CustomGenesisState{govtypes.ModuleName: gov}
	for _, extra := range extraGenesis {
		for module, state := range extra {
			genesis[module] = state
		}
	}
	net := network.NewUnitTestNetwork(newReleaseEvmd,
		network.WithPreFundedAccounts(accounts...),
		network.WithAmountOfValidators(len(operators)),
		network.WithValidatorOperators(operatorAccounts),
		network.WithAdditionalBalances(banktypes.Balance{Address: authtypes.NewModuleAddress(govtypes.ModuleName).String(), Coins: deposit}),
		network.WithCustomGenesis(genesis),
	)
	app, ok := net.App.(*evmd.EVMD)
	if !ok {
		tb.Fatalf("unexpected application type %T", net.App)
	}
	handler := grpc.NewIntegrationHandler(net)
	c := &chain{net: net, app: app, factory: evmfactory.New(net, handler), grpc: handler, users: users}
	byOperator := map[string]testKey{}
	for _, operator := range operators {
		byOperator[sdk.ValAddress(operator.acc).String()] = operator
	}
	for _, validator := range net.GetValidators() {
		operator, found := byOperator[validator.OperatorAddress]
		if !found {
			tb.Fatalf("validator %s has no deterministic operator", validator.OperatorAddress)
		}
		c.operators = append(c.operators, operator)
	}
	return c
}

func (c *chain) commitCosmosTx(tb testing.TB, key testKey, msgs ...sdk.Msg) {
	tb.Helper()
	res, err := c.factory.CommitCosmosTx(key.priv, factory.CosmosTxArgs{Msgs: msgs})
	if err != nil || res.Code != 0 {
		tb.Fatalf("cosmos tx %v failed: %v (%s)", msgs, err, res.Log)
	}
	if err := c.net.NextBlock(); err != nil {
		tb.Fatal(err)
	}
}

func (c *chain) bondDenom(tb testing.TB) string {
	tb.Helper()
	denom, err := c.app.StakingKeeper.BondDenom(c.net.GetContext())
	if err != nil {
		tb.Fatal(err)
	}
	return denom
}

// kvStoreKeys returns every persistent KV store of the application, sorted
// by name.
func (c *chain) kvStoreKeys(tb testing.TB) []*storetypes.KVStoreKey {
	tb.Helper()
	root, ok := c.app.GetBaseApp().CommitMultiStore().(*rootmulti.Store)
	if !ok {
		tb.Fatalf("unexpected commit multistore %T", c.app.GetBaseApp().CommitMultiStore())
	}
	var keys []*storetypes.KVStoreKey
	for _, key := range root.StoreKeysByName() {
		if kvKey, ok := key.(*storetypes.KVStoreKey); ok {
			keys = append(keys, kvKey)
		}
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].Name() < keys[j].Name() })
	if len(keys) < 15 {
		tb.Fatalf("only %d KV stores found", len(keys))
	}
	return keys
}

// stateDigest hashes every key and value of every persistent KV store.
func stateDigest(ctx sdk.Context, keys []*storetypes.KVStoreKey) [32]byte {
	hash := sha256.New()
	for _, key := range keys {
		iterator := ctx.KVStore(key).Iterator(nil, nil)
		for ; iterator.Valid(); iterator.Next() {
			fmt.Fprintf(hash, "%s/%x=%x;", key.Name(), iterator.Key(), iterator.Value())
		}
		iterator.Close()
	}
	var sum [32]byte
	copy(sum[:], hash.Sum(nil))
	return sum
}

// diffStores names the stores whose content differs between two contexts,
// for failure messages.
func diffStores(a, b sdk.Context, keys []*storetypes.KVStoreKey) []string {
	var changed []string
	for _, key := range keys {
		if stateDigest(a, []*storetypes.KVStoreKey{key}) != stateDigest(b, []*storetypes.KVStoreKey{key}) {
			changed = append(changed, key.Name())
		}
	}
	return changed
}

var (
	precompileHarnessOnce sync.Once
	precompileHarness     *calldataHarness
)

// calldataHarness is a chain frozen in a state where most precompile
// methods have something to act on: a delegation, an unbonding delegation,
// accrued rewards and a proposal in its voting period with two votes.
type calldataHarness struct {
	*chain
	base       sdk.Context
	keys       []*storetypes.KVStoreKey
	baseDigest [32]byte
	addresses  []common.Address
	strings    []string
	amounts    []*big.Int
}

func sharedCalldataHarness(tb testing.TB) *calldataHarness {
	tb.Helper()
	precompileHarnessOnce.Do(func() {
		c := newChain(tb)
		denom := c.bondDenom(tb)
		validator := c.net.GetValidators()[0]
		c.commitCosmosTx(tb, c.users[0], stakingtypes.NewMsgDelegate(c.users[0].acc.String(), validator.OperatorAddress, sdk.NewCoin(denom, sdkmath.NewInt(3e18))))
		c.commitCosmosTx(tb, c.users[0], stakingtypes.NewMsgUndelegate(c.users[0].acc.String(), validator.OperatorAddress, sdk.NewCoin(denom, sdkmath.NewInt(1e18))))
		// Votes on the open proposal, so that a tally that deletes votes
		// shows up as a state change.
		c.commitCosmosTx(tb, c.users[0], govv1.NewMsgVote(c.users[0].acc, 1, govv1.OptionYes, ""))
		c.commitCosmosTx(tb, c.users[1], govv1.NewMsgVote(c.users[1].acc, 1, govv1.OptionNo, ""))
		for i := 0; i < 2; i++ {
			if err := c.net.NextBlock(); err != nil {
				tb.Fatal(err)
			}
		}
		h := &calldataHarness{chain: c, base: c.net.GetContext(), keys: c.kvStoreKeys(tb)}
		// A dynamic ERC-20 precompile for an IBC voucher, and a balance of it.
		if _, err := c.app.Erc20Keeper.RegisterERC20Extension(h.base, ibcVoucherDenom); err != nil {
			tb.Fatalf("register the ERC-20 precompile of %s: %v", ibcVoucherDenom, err)
		}
		voucher := sdk.NewCoins(sdk.NewCoin(ibcVoucherDenom, sdkmath.NewInt(5e18)))
		if err := c.app.BankKeeper.MintCoins(h.base, minttypes.ModuleName, voucher); err != nil {
			tb.Fatal(err)
		}
		if err := c.app.BankKeeper.SendCoinsFromModuleToAccount(h.base, minttypes.ModuleName, c.users[1].acc, voucher); err != nil {
			tb.Fatal(err)
		}
		// Every target gets an (empty) account. x/vm persists an account
		// that an EVM call merely touches (STATICCALL always touches its
		// target) as an empty account, without EIP-161 cleanup. That is an
		// x/vm StateDB behavior for any address, not a write by the
		// precompile, so it is kept out of the read-only properties.
		for _, target := range allTargets {
			if h.app.AccountKeeper.GetAccount(h.base, target.address.Bytes()) == nil {
				h.app.AccountKeeper.SetAccount(h.base, h.app.AccountKeeper.NewAccountWithAddress(h.base, target.address.Bytes()))
			}
		}
		h.baseDigest = stateDigest(h.base.WithGasMeter(storetypes.NewInfiniteGasMeter()), h.keys)

		for _, user := range c.users {
			h.addresses = append(h.addresses, user.addr)
		}
		for _, operator := range c.operators {
			h.addresses = append(h.addresses, operator.addr)
		}
		for _, address := range evmtypes.AvailableStaticPrecompiles {
			h.addresses = append(h.addresses, common.HexToAddress(address))
		}
		h.addresses = append(h.addresses, common.Address{}, common.HexToAddress(testconstants.WEVMOSContractMainnet), erc20Target.address,
			common.BytesToAddress(authtypes.NewModuleAddress(stakingtypes.NotBondedPoolName)))

		for _, v := range c.net.GetValidators() {
			h.strings = append(h.strings, v.OperatorAddress)
		}
		h.strings = append(h.strings,
			c.users[0].acc.String(), c.users[1].acc.String(), denom, "foo", testconstants.ExampleAttoDenom,
			"transfer", "channel-0", "07-tendermint-0", "", " ", "\x00", "-1", "1", "ipfs://CID", ibcVoucherDenom,
			"xitcoinvaloper1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq", "ibc/27394FB092D2ECCD56123C74F36E4C1F926001CEADA9CA97EA622B25F41E5EB2",
			"{\"foo\":1}", "K", string(make([]byte, 300)),
		)
		for _, value := range []string{
			"0", "1", "2", "100", "1000000000000000000", "3000000000000000000", "100000000000000000000000",
			"9223372036854775807", "18446744073709551615", "18446744073709551616",
			"57896044618658097711785492504343953926634992332820282019728792003956564819967",
			"115792089237316195423570985008687907853269984665640564039457584007913129639935",
		} {
			amount, _ := new(big.Int).SetString(value, 10)
			h.amounts = append(h.amounts, amount)
		}
		precompileHarness = h
	})
	if precompileHarness == nil {
		tb.Fatal("precompile harness setup failed")
	}
	return precompileHarness
}
