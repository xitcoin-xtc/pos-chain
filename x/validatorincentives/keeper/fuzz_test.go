package keeper

// Stateful attack simulations of the validator incentive release on a real
// x/auth and x/bank keeper. `go test` replays the seed corpus; `go test -fuzz`
// explores further (see .github/workflows/fuzz.yml and
// docs/validator-incentives-attack-simulations.md).
//
// ProcessBlock runs like BeginBlock: in a cached context written only on
// success. In the application a BeginBlock error halts the chain, so the
// harness also checks that ProcessBlock never fails in a state the chain can
// reach.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"math/big"
	"testing"

	"github.com/xitcoin-xtc/pos-chain/x/validatorincentives/types"

	"cosmossdk.io/log/v2"
	sdkmath "cosmossdk.io/math"

	"github.com/cosmos/cosmos-sdk/codec"
	addresscodec "github.com/cosmos/cosmos-sdk/codec/address"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/cosmos/cosmos-sdk/runtime"
	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	sdktestutil "github.com/cosmos/cosmos-sdk/testutil"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authkeeper "github.com/cosmos/cosmos-sdk/x/auth/keeper"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
)

const (
	incentivesFaucet       = "incentives_fuzz_faucet"
	incentivesUserFunds    = 50_000_000_000
	incentivesTreasuryFund = 7_000_000_003
)

// incentivesPeriod is the model of the active calculation period.
type incentivesPeriod struct {
	start, end           uint64
	snapshot, provision  *big.Int
	distributed          *big.Int
	drainedDuringPeriod  bool
	params               types.Params
	eligibleBondedAtOpen *big.Int
}

type incentivesHarness struct {
	t         testing.TB
	ctx       sdk.Context
	keys      []*storetypes.KVStoreKey
	keeper    Keeper
	bank      bankkeeper.BaseKeeper
	treasury  Treasury
	staking   *fuzzStakingKeeper
	server    types.MsgServer
	authority sdk.AccAddress
	users     []sdk.AccAddress
	treasuryA sdk.AccAddress
	feeA      sdk.AccAddress

	// Model, maintained from the operations that succeeded.
	height   uint64
	params   types.Params
	period   *incentivesPeriod
	lifetime *big.Int
	balances map[string]*big.Int
	supply   *big.Int
}

// fuzzStakingKeeper returns the bonded stake chosen by the fuzzer.
type fuzzStakingKeeper struct{ bonded sdkmath.Int }

func (k *fuzzStakingKeeper) TotalValidatorPower(context.Context) (sdkmath.Int, error) {
	return k.bonded, nil
}

func newIncentivesHarness(tb testing.TB, initial types.Params) *incentivesHarness {
	tb.Helper()
	authKey := storetypes.NewKVStoreKey(authtypes.StoreKey)
	bankKey := storetypes.NewKVStoreKey(banktypes.StoreKey)
	incentivesKey := storetypes.NewKVStoreKey(types.StoreKey)
	ctx := sdktestutil.DefaultContextWithKeys(
		map[string]*storetypes.KVStoreKey{"auth": authKey, "bank": bankKey, "incentives": incentivesKey},
		map[string]*storetypes.TransientStoreKey{"transient": storetypes.NewTransientStoreKey("incentives_fuzz_transient")},
		nil,
	).WithBlockHeight(1)

	registry := codectypes.NewInterfaceRegistry()
	authtypes.RegisterInterfaces(registry)
	banktypes.RegisterInterfaces(registry)
	cryptocodec.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)
	prefix := sdk.GetConfig().GetBech32AccountAddrPrefix()
	govAuthority := authtypes.NewModuleAddress("gov").String()
	// The application registers the treasury and the fee collector without
	// mint or burn permission (evmd/config/permissions.go).
	accounts := authkeeper.NewAccountKeeper(
		cdc, runtime.NewKVStoreService(authKey), authtypes.ProtoBaseAccount,
		map[string][]string{
			authtypes.FeeCollectorName: nil,
			types.TreasuryAccountName:  nil,
			incentivesFaucet:           {authtypes.Minter},
		},
		addresscodec.NewBech32Codec(prefix), prefix, govAuthority,
	)
	feeCollector := authtypes.NewModuleAddress(authtypes.FeeCollectorName)
	// Module accounts are blocked except the treasury, which must accept
	// funding transfers (evmd/config.BlockedAddresses).
	blocked := map[string]bool{
		feeCollector.String(): true,
		authtypes.NewModuleAddress(incentivesFaucet).String(): true,
	}
	bank := bankkeeper.NewBaseKeeper(cdc, runtime.NewKVStoreService(bankKey), accounts, blocked, govAuthority, log.NewNopLogger())
	if err := bank.SetParams(ctx, banktypes.DefaultParams()); err != nil {
		tb.Fatal(err)
	}

	h := &incentivesHarness{
		t: tb, ctx: ctx, keys: []*storetypes.KVStoreKey{authKey, bankKey, incentivesKey},
		keeper: NewKeeper(incentivesKey), bank: bank, treasury: NewTreasury(accounts, bank),
		staking:   &fuzzStakingKeeper{bonded: sdkmath.NewInt(1_000_000)},
		treasuryA: authtypes.NewModuleAddress(types.TreasuryAccountName), feeA: feeCollector,
		height: 1, params: initial, lifetime: new(big.Int), balances: map[string]*big.Int{},
	}
	h.server = NewMsgServerImpl(h.keeper)
	for i := 0; i < 3; i++ {
		seed := sha256.Sum256([]byte(fmt.Sprintf("incentives-fuzz/user-%d", i)))
		user := sdk.AccAddress(seed[:20])
		funds := sdk.NewCoins(sdk.NewInt64Coin(IncentiveDenom, incentivesUserFunds))
		if err := bank.MintCoins(ctx, incentivesFaucet, funds); err != nil {
			tb.Fatal(err)
		}
		if err := bank.SendCoinsFromModuleToAccount(ctx, incentivesFaucet, user, funds); err != nil {
			tb.Fatal(err)
		}
		h.users = append(h.users, user)
	}
	h.authority = h.users[0]
	if err := h.keeper.InitGenesis(ctx, types.GenesisState{Authority: h.authority.String(), Params: initial}); err != nil {
		tb.Fatal(err)
	}
	if err := bank.SendCoins(ctx, h.users[2], h.treasuryA, sdk.NewCoins(sdk.NewInt64Coin(IncentiveDenom, incentivesTreasuryFund))); err != nil {
		tb.Fatal(err)
	}
	h.supply = bank.GetSupply(ctx, IncentiveDenom).Amount.BigInt()
	for _, address := range h.tracked() {
		h.balances[address.String()] = h.balance(address)
	}
	return h
}

func (h *incentivesHarness) tracked() []sdk.AccAddress {
	return append([]sdk.AccAddress{h.treasuryA, h.feeA}, h.users...)
}

func (h *incentivesHarness) balance(address sdk.AccAddress) *big.Int {
	return h.bank.GetBalance(h.ctx, address, IncentiveDenom).Amount.BigInt()
}

// digest hashes every key and value of every store of the harness.
func (h *incentivesHarness) digest() [32]byte {
	hash := sha256.New()
	for _, key := range h.keys {
		iterator := h.ctx.KVStore(key).Iterator(nil, nil)
		for ; iterator.Valid(); iterator.Next() {
			hash.Write([]byte(key.Name()))
			hash.Write(iterator.Key())
			hash.Write([]byte{0})
			hash.Write(iterator.Value())
			hash.Write([]byte{0})
		}
		iterator.Close()
	}
	var sum [32]byte
	copy(sum[:], hash.Sum(nil))
	return sum
}

// deliver runs one operation in a cached context written only on success,
// like a transaction or BeginBlock, and fails the test on panic. A failed
// operation must leave the committed stores byte-identical.
func (h *incentivesHarness) deliver(name string, run func(ctx sdk.Context) error) (err error) {
	h.t.Helper()
	before := h.digest()
	cache, write := h.ctx.CacheContext()
	defer func() {
		if recovered := recover(); recovered != nil {
			h.t.Fatalf("%s panicked: %v", name, recovered)
		}
		if err != nil && h.digest() != before {
			h.t.Fatalf("%s failed (%v) but changed committed state", name, err)
		}
	}()
	if err = run(cache); err == nil {
		write()
	}
	return err
}

func (h *incentivesHarness) move(from, to sdk.AccAddress, amount *big.Int) {
	h.balances[from.String()] = new(big.Int).Sub(h.balances[from.String()], amount)
	h.balances[to.String()] = new(big.Int).Add(h.balances[to.String()], amount)
}

func floorMulDiv(value *big.Int, numerator, denominator uint64) *big.Int {
	product := new(big.Int).Mul(value, new(big.Int).SetUint64(numerator))
	return product.Quo(product, new(big.Int).SetUint64(denominator))
}

// processBlock runs ProcessBlock at the current height and checks the
// release against the model: a new period opens when there is none or the
// height reached its end; the block releases floor(P * elapsed / length) -
// distributed, from the treasury to the fee collector only.
func (h *incentivesHarness) processBlock(step int) {
	h.t.Helper()
	period := h.period
	if period == nil || h.height >= period.end {
		bonded := h.staking.bonded.BigInt()
		snapshot := new(big.Int).Set(h.balances[h.treasuryA.String()])
		provision := new(big.Int)
		if bonded.Sign() > 0 {
			annualized := floorMulDiv(snapshot, uint64(h.params.TreasuryReleaseRateBasisPoints), uint64(types.BasisPointDenominator))
			provision = floorMulDiv(annualized, h.params.CalculationPeriodBlocks, h.params.BlocksPerYear)
		}
		period = &incentivesPeriod{
			start: h.height, end: h.height + h.params.CalculationPeriodBlocks,
			snapshot: snapshot, provision: provision, distributed: new(big.Int),
			params: h.params, eligibleBondedAtOpen: bonded,
		}
	}
	target := floorMulDiv(period.provision, h.height-period.start+1, period.end-period.start)
	amount := new(big.Int).Sub(target, period.distributed)
	if amount.Sign() < 0 {
		h.t.Fatalf("step %d: model release is negative", step)
	}
	treasuryBalance := h.balances[h.treasuryA.String()]
	wantOK := amount.Cmp(treasuryBalance) <= 0

	err := h.deliver("ProcessBlock", func(ctx sdk.Context) error {
		height := int64(h.height) //nolint:gosec // G115: the harness stays below a few thousand blocks
		return h.keeper.ProcessBlock(ctx.WithBlockHeight(height), h.staking, h.treasury, authtypes.FeeCollectorName)
	})
	if (err == nil) != wantOK {
		h.t.Fatalf("step %d: ProcessBlock at height %d: err=%v, model expects success=%v (release %s, treasury %s)",
			step, h.height, err, wantOK, amount, treasuryBalance)
	}
	if err != nil {
		if period != h.period || !period.drainedDuringPeriod {
			h.t.Fatalf("step %d: ProcessBlock failed in a reachable state (a BeginBlock error halts the chain): %v", step, err)
		}
		return
	}
	period.distributed.Add(period.distributed, amount)
	h.period = period
	h.lifetime.Add(h.lifetime, amount)
	h.move(h.treasuryA, h.feeA, amount)
}

func (h *incentivesHarness) fund(user sdk.AccAddress, amount *big.Int) {
	h.t.Helper()
	coins := sdk.NewCoins(sdk.NewCoin(IncentiveDenom, sdkmath.NewIntFromBigInt(amount)))
	err := h.deliver("fund", func(ctx sdk.Context) error { return h.bank.SendCoins(ctx, user, h.treasuryA, coins) })
	if (err == nil) != (amount.Cmp(h.balances[user.String()]) <= 0) {
		h.t.Fatalf("funding %s from a balance of %s: err=%v", amount, h.balances[user.String()], err)
	}
	if err == nil {
		h.move(user, h.treasuryA, amount)
	}
}

// drain removes funds from the treasury behind the module's back. The chain
// has no such path (the treasury account has no key, no code and no module
// that spends from it); it models a hypothetical leak to check that the
// release fails closed instead of paying more than the treasury holds.
func (h *incentivesHarness) drain(amount *big.Int) {
	h.t.Helper()
	if balance := h.balances[h.treasuryA.String()]; amount.Cmp(balance) > 0 {
		amount = new(big.Int).Set(balance)
	}
	if amount.Sign() == 0 {
		return
	}
	coins := sdk.NewCoins(sdk.NewCoin(IncentiveDenom, sdkmath.NewIntFromBigInt(amount)))
	if err := h.deliver("drain", func(ctx sdk.Context) error {
		return h.bank.SendCoinsFromModuleToAccount(ctx, types.TreasuryAccountName, h.users[1], coins)
	}); err != nil {
		h.t.Fatalf("drain: %v", err)
	}
	h.move(h.treasuryA, h.users[1], amount)
	if h.period != nil {
		h.period.drainedDuringPeriod = true
	}
}

// updateParams sends MsgUpdateParams signed as caller. Only the exact
// authority with valid parameters may change them, and a change never
// alters the active period.
func (h *incentivesHarness) updateParams(step int, caller string, next types.Params) {
	h.t.Helper()
	msg := &types.MsgUpdateParams{
		Authority:                      caller,
		TreasuryReleaseRateBasisPoints: next.TreasuryReleaseRateBasisPoints,
		BlocksPerYear:                  next.BlocksPerYear,
		CalculationPeriodBlocks:        next.CalculationPeriodBlocks,
	}
	err := h.deliver("UpdateParams", func(ctx sdk.Context) error {
		_, err := h.server.UpdateParams(ctx, msg)
		return err
	})
	wantOK := caller == h.authority.String() && next.Validate() == nil
	if (err == nil) != wantOK {
		h.t.Fatalf("step %d: UpdateParams by %q with %+v: err=%v, want success=%v", step, caller, next, err, wantOK)
	}
	if err == nil {
		h.params = next
	}
}

func (h *incentivesHarness) checkInvariants(step int, name string) {
	h.t.Helper()
	fail := func(format string, args ...any) {
		h.t.Helper()
		h.t.Fatalf("step %d (%s): %s", step, name, fmt.Sprintf(format, args...))
	}
	if got := h.keeper.GetParams(h.ctx); got != h.params {
		fail("params %+v, model %+v", got, h.params)
	}
	if supply := h.bank.GetSupply(h.ctx, IncentiveDenom).Amount.BigInt(); supply.Cmp(h.supply) != 0 {
		fail("supply changed from %s to %s: tokens were minted or burned", h.supply, supply)
	}
	// Every balance of the chain matches the model: the release pays the fee
	// collector only, out of the treasury only.
	seen := map[string]bool{}
	for _, entry := range h.bank.GetAccountsBalances(h.ctx) {
		want, tracked := h.balances[entry.Address]
		if !tracked {
			fail("unexpected balance %s at %s", entry.Coins, entry.Address)
		}
		if got := entry.Coins.AmountOf(IncentiveDenom).BigInt(); got.Cmp(want) != 0 {
			fail("balance of %s is %s, model %s", entry.Address, got, want)
		}
		seen[entry.Address] = true
	}
	for address, want := range h.balances {
		if want.Sign() != 0 && !seen[address] {
			fail("balance of %s missing, model %s", address, want)
		}
	}
	total, err := h.keeper.GetTotalDistributed(h.ctx)
	if err != nil {
		fail("total distributed unreadable: %v", err)
	}
	if total.BigInt().Cmp(h.lifetime) != 0 {
		fail("total distributed %s, sum of releases %s", total, h.lifetime)
	}
	if h.lifetime.Cmp(h.balances[h.feeA.String()]) != 0 {
		fail("fee collector holds %s, releases total %s", h.balances[h.feeA.String()], h.lifetime)
	}

	state, found, err := h.keeper.GetPeriodState(h.ctx)
	if err != nil {
		fail("period state unreadable: %v", err)
	}
	if found != (h.period != nil) {
		fail("period stored=%v, model=%v", found, h.period != nil)
	}
	if !found {
		return
	}
	p := h.period
	provision, _ := types.ParseStoredAtomicAmount(state.PeriodProvisionAtomic)
	distributed, _ := types.ParseStoredAtomicAmount(state.DistributedAtomic)
	snapshot, _ := types.ParseStoredAtomicAmount(state.TreasuryBalanceAtomic)
	if state.StartBlock != p.start || state.EndBlock != p.end || provision.BigInt().Cmp(p.provision) != 0 ||
		distributed.BigInt().Cmp(p.distributed) != 0 || snapshot.BigInt().Cmp(p.snapshot) != 0 {
		fail("period %+v, model start=%d end=%d provision=%s distributed=%s snapshot=%s",
			state, p.start, p.end, p.provision, p.distributed, p.snapshot)
	}
	if state.TreasuryReleaseRateBasisPoints != p.params.TreasuryReleaseRateBasisPoints ||
		state.EligibleBondedAtomic != p.eligibleBondedAtOpen.String() {
		fail("period %+v was not derived from the parameters and stake at its opening", state)
	}
	// distributed <= provision <= treasury snapshot.
	if p.distributed.Cmp(p.provision) > 0 || p.provision.Cmp(p.snapshot) > 0 {
		fail("distributed %s, provision %s, snapshot %s", p.distributed, p.provision, p.snapshot)
	}
	// The treasury always covers what remains to be released, unless it was
	// drained behind the module's back.
	remaining := new(big.Int).Sub(p.provision, p.distributed)
	if !p.drainedDuringPeriod && remaining.Cmp(h.balances[h.treasuryA.String()]) > 0 {
		fail("remaining provision %s exceeds the treasury %s", remaining, h.balances[h.treasuryA.String()])
	}
}

// fuzzParams maps three bytes to parameters, about one in eight invalid.
func fuzzParams(rate, year, period byte) types.Params {
	rates := []uint32{0, 1, 1_000, 3_333, 9_999, types.BasisPointDenominator, types.BasisPointDenominator + 1}
	params := types.Params{
		TreasuryReleaseRateBasisPoints: rates[int(rate)%len(rates)],
		CalculationPeriodBlocks:        uint64(period % 13),
	}
	params.BlocksPerYear = params.CalculationPeriodBlocks * (1 + uint64(year%9))
	if year >= 0xf0 {
		params.BlocksPerYear = params.CalculationPeriodBlocks - 1 // period longer than a year
	}
	return params
}

var fuzzBondedStakes = []int64{0, 1, 1_000, 1_000_000, 7_000_000_003, 1 << 62}

func runIncentivesSequence(t *testing.T, data []byte) {
	initial := types.Params{TreasuryReleaseRateBasisPoints: 1_000, BlocksPerYear: 40, CalculationPeriodBlocks: 5}
	if len(data) >= 3 {
		if params := fuzzParams(data[0], data[1], data[2]); params.Validate() == nil {
			initial = params
		}
		data = data[3:]
	}
	h := newIncentivesHarness(t, initial)
	h.checkInvariants(0, "genesis")
	for step := 1; len(data) >= 3 && step <= 96; step++ {
		op, a, b := data[0], data[1], data[2]
		data = data[3:]
		var name string
		switch op % 9 {
		case 0, 1:
			name = "next block"
			h.height++
			h.processBlock(step)
		case 2:
			name = "skip heights"
			h.height += 2 + uint64(a)%(3*maxUint64(h.params.CalculationPeriodBlocks, 1)+3)
			h.processBlock(step)
		case 3:
			name = "replay ProcessBlock in the same block"
			if h.period != nil {
				h.processBlock(step)
			}
		case 4:
			name = "fund treasury"
			h.fund(h.users[1+int(a)%2], new(big.Int).Lsh(big.NewInt(int64(b)+1), uint(a%34)))
		case 5:
			name = "drain treasury"
			h.drain(new(big.Int).Lsh(big.NewInt(int64(b)+1), uint(a%34)))
		case 6:
			name = "change bonded stake"
			h.staking.bonded = sdkmath.NewInt(fuzzBondedStakes[int(a)%len(fuzzBondedStakes)] + int64(b))
		case 7:
			name = "authority updates params"
			h.updateParams(step, h.authority.String(), fuzzParams(a, b, op/9))
		default:
			name = "outsider updates params"
			callers := []string{
				h.users[1].String(), "", authtypes.NewModuleAddress("gov").String(), h.treasuryA.String(),
				" " + h.authority.String(), h.authority.String() + "\n", sdk.MustBech32ifyAddressBytes("other", h.authority),
				string(bytes.ToUpper([]byte(h.authority.String()))),
			}
			h.updateParams(step, callers[int(a)%len(callers)], fuzzParams(b, a, op/9))
		}
		h.checkInvariants(step, name)
	}
}

func maxUint64(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}

func FuzzIncentiveReleaseSequence(f *testing.F) {
	seeds := [][]byte{
		// two full periods block by block, then a replay in the same block
		{2, 0, 5, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3, 0, 0},
		// skipped heights across several periods (rollover forfeits nothing but the skipped blocks)
		{2, 3, 7, 0, 0, 0, 2, 5, 0, 0, 0, 0, 2, 40, 0, 0, 0, 0, 2, 1, 0},
		// funding mid-period is only used from the next snapshot
		{2, 0, 5, 0, 0, 0, 4, 20, 200, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		// drained treasury: the release fails closed, and recovers after refunding
		{5, 8, 3, 0, 0, 0, 5, 33, 255, 0, 0, 0, 0, 0, 0, 4, 20, 200, 0, 0, 0, 2, 9, 0, 0, 0, 0},
		// parameter changes mid-period by the authority and by outsiders
		{2, 0, 5, 0, 0, 0, 7, 2, 2, 0, 0, 0, 8, 0, 5, 8, 4, 5, 8, 6, 2, 8, 7, 2, 7, 6, 9, 0, 0, 0, 2, 9, 0, 0, 0, 0},
		// invalid authority updates: rate above 100%, period longer than a year, zero period
		{2, 0, 5, 7, 6, 0, 7, 0, 0xf5, 16, 3, 0, 0, 0, 0},
		// bonded stake drops to zero and comes back
		{2, 0, 5, 6, 0, 0, 0, 0, 0, 2, 6, 0, 6, 3, 0, 0, 0, 0, 2, 6, 0, 0, 0, 0},
		// 100% release rate, period equal to one year, huge stake
		{5, 0, 12, 6, 5, 0, 0, 0, 0, 2, 13, 0, 0, 0, 0, 0, 0, 0},
	}
	for _, seed := range seeds {
		f.Add(seed)
	}
	f.Fuzz(runIncentivesSequence)
}
