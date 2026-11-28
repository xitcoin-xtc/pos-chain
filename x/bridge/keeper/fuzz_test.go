package keeper

// Stateful attack simulations of the bridge keeper on a real x/auth and
// x/bank keeper. `go test` replays the seed corpus; `go test -fuzz` explores
// further (see .github/workflows/fuzz.yml). Each operation runs in a cached
// context that is written only on success, as a transaction would be.

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/xitcoin-xtc/pos-chain/x/bridge/types"

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
	harnessDenom      = "axtc"
	harnessFaucet     = "faucet"
	harnessMaxSupply  = 6000
	harnessUserFunds  = 1000
	harnessStartUnix  = 1800000000 // 2027-01-15T08:00:00Z
	harnessDaySeconds = 86400
)

var (
	harnessCanonicalAmount = regexp.MustCompile(`^[1-9][0-9]*$`)
	harnessMaxUint256      = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
)

// oracleAmount is the canonical amount rule, written independently of
// types.ParseAmount.
func oracleAmount(s string) (*big.Int, bool) {
	if !harnessCanonicalAmount.MatchString(s) {
		return nil, false
	}
	value, ok := new(big.Int).SetString(s, 10)
	return value, ok && value.Cmp(harnessMaxUint256) <= 0
}

type bridgeHarness struct {
	t        testing.TB
	ctx      sdk.Context
	keeper   Keeper
	bank     bankkeeper.BaseKeeper
	server   types.MsgServer
	signers  []*ecdsa.PrivateKey // three configured bridge signers
	outsider *ecdsa.PrivateKey
	guardian *ecdsa.PrivateKey
	users    []sdk.AccAddress // funded users; the last entry is the blocked bridge module account
	initial  sdkmath.Int

	// Model, maintained from the operations that succeeded.
	minted, burned *big.Int
	mints          map[[32]byte]int
	dailyUsage     map[int64]*big.Int
	controlNonce   uint64
	pauseNonce     uint64
	usedPauses     []*types.MsgEmergencyPauseRoute
}

func deterministicKey(tb testing.TB, label string) *ecdsa.PrivateKey {
	tb.Helper()
	seed := sha256.Sum256([]byte("bridge-keeper-fuzz/" + label))
	key, err := crypto.ToECDSA(seed[:])
	if err != nil {
		tb.Fatal(err)
	}
	return key
}

func newBridgeHarness(tb testing.TB) *bridgeHarness {
	tb.Helper()
	authKey := storetypes.NewKVStoreKey(authtypes.StoreKey)
	bankKey := storetypes.NewKVStoreKey(banktypes.StoreKey)
	bridgeKey := storetypes.NewKVStoreKey(types.StoreKey)
	ctx := sdktestutil.DefaultContextWithKeys(
		map[string]*storetypes.KVStoreKey{"auth": authKey, "bank": bankKey, "bridge": bridgeKey},
		map[string]*storetypes.TransientStoreKey{"transient": storetypes.NewTransientStoreKey("bridge_fuzz_transient")},
		nil,
	).WithChainID(testChainID).WithBlockHeight(1).WithBlockTime(time.Unix(harnessStartUnix, 0).UTC())

	registry := codectypes.NewInterfaceRegistry()
	authtypes.RegisterInterfaces(registry)
	banktypes.RegisterInterfaces(registry)
	cryptocodec.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)
	prefix := sdk.GetConfig().GetBech32AccountAddrPrefix()
	authority := authtypes.NewModuleAddress("gov").String()
	accounts := authkeeper.NewAccountKeeper(
		cdc, runtime.NewKVStoreService(authKey), authtypes.ProtoBaseAccount,
		map[string][]string{types.ModuleName: {authtypes.Minter, authtypes.Burner}, harnessFaucet: {authtypes.Minter}},
		addresscodec.NewBech32Codec(prefix), prefix, authority,
	)
	bridgeModule := authtypes.NewModuleAddress(types.ModuleName)
	blocked := map[string]bool{bridgeModule.String(): true, authtypes.NewModuleAddress(harnessFaucet).String(): true}
	bank := bankkeeper.NewBaseKeeper(cdc, runtime.NewKVStoreService(bankKey), accounts, blocked, authority, log.NewNopLogger())
	if err := bank.SetParams(ctx, banktypes.DefaultParams()); err != nil {
		tb.Fatal(err)
	}

	h := &bridgeHarness{
		t: tb, ctx: ctx, bank: bank,
		outsider: deterministicKey(tb, "outsider"), guardian: deterministicKey(tb, "guardian"),
		minted: new(big.Int), burned: new(big.Int), mints: map[[32]byte]int{}, dailyUsage: map[int64]*big.Int{},
	}
	for i := 0; i < 3; i++ {
		h.signers = append(h.signers, deterministicKey(tb, fmt.Sprintf("signer-%d", i)))
		user := sdk.AccAddress(crypto.PubkeyToAddress(deterministicKey(tb, fmt.Sprintf("user-%d", i)).PublicKey).Bytes())
		funds := sdk.NewCoins(sdk.NewInt64Coin(harnessDenom, harnessUserFunds))
		if err := bank.MintCoins(ctx, harnessFaucet, funds); err != nil {
			tb.Fatal(err)
		}
		if err := bank.SendCoinsFromModuleToAccount(ctx, harnessFaucet, user, funds); err != nil {
			tb.Fatal(err)
		}
		h.users = append(h.users, user)
	}
	h.users = append(h.users, bridgeModule)
	h.initial = bank.GetSupply(ctx, harnessDenom).Amount

	k, err := NewSettlementKeeper(bridgeKey, bank, harnessDenom, fmt.Sprintf("%d", harnessMaxSupply), authority)
	if err != nil {
		tb.Fatal(err)
	}
	config := types.RouteConfig{
		RouteID: "cronos-testnet-xitcoin-testnet",
		BridgeSigners: []string{
			crypto.PubkeyToAddress(h.signers[0].PublicKey).Hex(),
			crypto.PubkeyToAddress(h.signers[1].PublicKey).Hex(),
			crypto.PubkeyToAddress(h.signers[2].PublicKey).Hex(),
		},
		Guardian:          crypto.PubkeyToAddress(h.guardian.PublicKey).Hex(),
		MaxTransferAmount: "300", DailyLimit: "1000", MaxOutstandingAmount: "2500", Enabled: true,
	}
	if err := k.SetRouteConfig(ctx, config); err != nil {
		tb.Fatal(err)
	}
	h.keeper = k
	h.server = NewMsgServer(k)
	return h
}

func (h *bridgeHarness) now() int64 { return h.ctx.BlockTime().Unix() }
func (h *bridgeHarness) day() int64 { return h.now() / harnessDaySeconds }

func (h *bridgeHarness) config() types.RouteConfig {
	config, found, err := h.keeper.GetRouteConfig(h.ctx)
	if err != nil || !found {
		h.t.Fatalf("route configuration unreadable: found=%v err=%v", found, err)
	}
	return config
}

func (h *bridgeHarness) paused() bool {
	state, err := h.keeper.GetRouteState(h.ctx)
	if err != nil {
		h.t.Fatal(err)
	}
	return state.Paused
}

func (h *bridgeHarness) supply() *big.Int {
	return h.bank.GetSupply(h.ctx, harnessDenom).Amount.BigInt()
}

func (h *bridgeHarness) balance(address sdk.AccAddress) *big.Int {
	return h.bank.GetBalance(h.ctx, address, harnessDenom).Amount.BigInt()
}

func (h *bridgeHarness) usage(day int64) *big.Int {
	if usage, ok := h.dailyUsage[day]; ok {
		return usage
	}
	return new(big.Int)
}

func bigOf(value string) *big.Int {
	parsed, ok := new(big.Int).SetString(value, 10)
	if !ok {
		panic("invalid test number " + value)
	}
	return parsed
}

// deliver runs one message in a cached context that is committed only on
// success, like a transaction, and fails the test on panic.
func (h *bridgeHarness) deliver(name string, run func(ctx sdk.Context) error) (err error) {
	h.t.Helper()
	cache, write := h.ctx.CacheContext()
	cache = cache.WithEventManager(sdk.NewEventManager())
	defer func() {
		if recovered := recover(); recovered != nil {
			h.t.Fatalf("%s panicked: %v", name, recovered)
		}
	}()
	if err = run(cache); err == nil {
		write()
		h.ctx.EventManager().EmitEvents(cache.EventManager().Events())
	}
	return err
}

func (h *bridgeHarness) depositRef(index byte) string {
	sum := sha256.Sum256([]byte{'d', index})
	return "0x" + hex.EncodeToString(sum[:])
}

func (h *bridgeHarness) attestationMsg(deposit byte, amount string, destination sdk.AccAddress) types.MsgSubmitAttestation {
	return types.MsgSubmitAttestation{
		Submitter: h.users[0].String(), RouteId: "cronos-testnet-xitcoin-testnet",
		Direction: string(types.DirectionCronosToXitcoin), SourceChainId: "338", SourceRef: h.depositRef(deposit),
		Nonce: uint64(deposit) + 1, Destination: destination.String(), Amount: amount, DeadlineUnix: h.now() + 3600,
	}
}

func sign(tb testing.TB, digest common.Hash, keys ...*ecdsa.PrivateKey) [][]byte {
	tb.Helper()
	signatures := make([][]byte, 0, len(keys))
	for _, key := range keys {
		signature, err := crypto.Sign(digest.Bytes(), key)
		if err != nil {
			tb.Fatal(err)
		}
		signatures = append(signatures, signature)
	}
	return signatures
}

// signAttestation signs msg for chainID if it has a digest at all.
func (h *bridgeHarness) signAttestation(msg *types.MsgSubmitAttestation, chainID string, keys ...*ecdsa.PrivateKey) {
	digest, err := types.SigningDigest(msg.Attestation(), chainID)
	if err != nil {
		return
	}
	msg.Signatures = sign(h.t, digest, keys...)
}

func (h *bridgeHarness) twoSigners(choice byte) []*ecdsa.PrivateKey {
	switch choice % 4 {
	case 0:
		return []*ecdsa.PrivateKey{h.signers[0], h.signers[1]}
	case 1:
		return []*ecdsa.PrivateKey{h.signers[1], h.signers[2]}
	case 2:
		return []*ecdsa.PrivateKey{h.signers[2], h.signers[0]}
	default:
		return h.signers
	}
}

// snapshot is everything an operation may change.
type snapshot struct {
	supply, outstanding, usage *big.Int
	paused, enabled            bool
	controlNonce               uint64
	config                     types.RouteConfig
}

func (h *bridgeHarness) snapshot() snapshot {
	config := h.config()
	return snapshot{
		supply: h.supply(), outstanding: h.keeper.OutstandingAmount(h.ctx).BigInt(), usage: h.keeper.DailyUsage(h.ctx, config.RouteID),
		paused: h.paused(), enabled: config.Enabled, controlNonce: h.keeper.ControlNonce(h.ctx), config: config,
	}
}

func (h *bridgeHarness) requireUnchanged(name string, before snapshot) {
	h.t.Helper()
	after := h.snapshot()
	if before.supply.Cmp(after.supply) != 0 || before.outstanding.Cmp(after.outstanding) != 0 || before.usage.Cmp(after.usage) != 0 ||
		before.paused != after.paused || before.controlNonce != after.controlNonce {
		h.t.Fatalf("%s failed but changed state: %+v -> %+v", name, before, after)
	}
}

// checkInvariants asserts the bridge safety properties after every operation.
func (h *bridgeHarness) checkInvariants(step int, name string, before snapshot) {
	h.t.Helper()
	after := h.snapshot()
	fail := func(format string, args ...any) {
		h.t.Helper()
		h.t.Fatalf("step %d (%s): %s", step, name, fmt.Sprintf(format, args...))
	}
	netBridged := new(big.Int).Sub(h.minted, h.burned)
	if after.outstanding.Cmp(netBridged) != 0 {
		fail("outstanding %s != mints %s - burns %s", after.outstanding, h.minted, h.burned)
	}
	if want := new(big.Int).Add(h.initial.BigInt(), netBridged); after.supply.Cmp(want) != 0 {
		fail("supply %s != initial %s + mints - burns %s", after.supply, h.initial, netBridged)
	}
	if h.balance(h.users[len(h.users)-1]).Sign() != 0 {
		fail("the bridge module account holds XTC")
	}
	if after.outstanding.Cmp(bigOf(after.config.MaxOutstandingAmount)) > 0 {
		fail("outstanding %s exceeds the cap %s", after.outstanding, after.config.MaxOutstandingAmount)
	}
	if after.supply.Cmp(big.NewInt(harnessMaxSupply)) > 0 {
		fail("supply %s exceeds the maximum supply %d", after.supply, harnessMaxSupply)
	}
	if after.usage.Cmp(h.usage(h.day())) != 0 {
		fail("daily usage %s != accepted transfers of the UTC day %s", after.usage, h.usage(h.day()))
	}
	// Transfers never push the usage of a UTC day beyond the limit in
	// force when they were accepted.
	if after.usage.Cmp(before.usage) > 0 && after.usage.Cmp(bigOf(before.config.DailyLimit)) > 0 {
		fail("daily usage %s exceeds the limit %s", after.usage, before.config.DailyLimit)
	}
	for key, count := range h.mints {
		if count > 1 {
			fail("deposit %x minted %d times", key, count)
		}
	}
	if (before.paused || !before.enabled) && (after.supply.Cmp(before.supply) != 0 || after.outstanding.Cmp(before.outstanding) != 0) {
		fail("mint or burn while paused=%v enabled=%v", before.paused, before.enabled)
	}
	if after.controlNonce != h.controlNonce {
		fail("control nonce %d, model %d", after.controlNonce, h.controlNonce)
	}
	if err := h.keeper.ExportGenesis(h.ctx).Validate(); err != nil {
		fail("exported genesis is invalid: %v", err)
	}
}

func (h *bridgeHarness) recordTransfer(amount *big.Int) {
	h.dailyUsage[h.day()] = new(big.Int).Add(h.usage(h.day()), amount)
}

// fitsLimits is the model of the per-transfer and daily limits.
func (h *bridgeHarness) fitsLimits(before snapshot, amount *big.Int) bool {
	return amount.Cmp(bigOf(before.config.MaxTransferAmount)) <= 0 &&
		new(big.Int).Add(before.usage, amount).Cmp(bigOf(before.config.DailyLimit)) <= 0
}

func amountFrom(b byte) string {
	table := []string{"1", "7", "50", "99", "100", "150", "299", "300", "301", "999", "1000", "2500", "2501", "6000"}
	if int(b) < len(table)*8 {
		return table[int(b)%len(table)]
	}
	return fmt.Sprintf("%d", int(b)*2)
}

func nonCanonical(amount string, variant byte) string {
	switch variant % 6 {
	case 0:
		return "0" + amount
	case 1:
		return "+" + amount
	case 2:
		return amount + " "
	case 3:
		return " " + amount
	case 4:
		return "00" + amount
	default:
		return "0x" + amount
	}
}

// inbound submits a correctly signed attestation and checks the model's
// prediction of acceptance.
func (h *bridgeHarness) inbound(step int, deposit, amountByte, choice byte) {
	before := h.snapshot()
	amount := amountFrom(amountByte)
	destination := h.users[int(choice>>2)%len(h.users)]
	msg := h.attestationMsg(deposit%6, amount, destination)
	if choice&0x40 != 0 {
		// Another spelling of the same deposit reference.
		msg.SourceRef = "0X" + strings.ToUpper(strings.TrimPrefix(msg.SourceRef, "0x"))
	}
	h.signAttestation(&msg, testChainID, h.twoSigners(choice)...)
	key, err := msg.Attestation().ID()
	if err != nil {
		h.t.Fatal(err)
	}
	value := bigOf(amount)
	want := before.enabled && !before.paused && h.mints[key] == 0 && h.fitsLimits(before, value) &&
		!h.bank.BlockedAddr(destination) &&
		new(big.Int).Add(before.outstanding, value).Cmp(bigOf(before.config.MaxOutstandingAmount)) <= 0 &&
		new(big.Int).Add(before.supply, value).Cmp(big.NewInt(harnessMaxSupply)) <= 0
	received := h.balance(destination)
	err = h.deliver("inbound", func(ctx sdk.Context) error { _, err := h.server.SubmitAttestation(ctx, &msg); return err })
	if (err == nil) != want {
		h.t.Fatalf("step %d: inbound %s for deposit %d accepted=%v, model %v (err %v, before %+v)", step, amount, deposit%6, err == nil, want, err, before)
	}
	if err != nil {
		h.requireUnchanged("inbound", before)
		return
	}
	if got := new(big.Int).Sub(h.balance(destination), received); got.Cmp(value) != 0 {
		h.t.Fatalf("step %d: recipient received %s for an attestation of %s", step, got, amount)
	}
	h.minted.Add(h.minted, value)
	h.mints[key]++
	h.recordTransfer(value)
}

// replay resubmits an already used deposit with other signed fields or
// another reference spelling; it must never mint twice.
func (h *bridgeHarness) replay(step int, deposit, variant, choice byte) {
	msg := h.attestationMsg(deposit%6, "7", h.users[0])
	key, err := msg.Attestation().ID()
	if err != nil {
		h.t.Fatal(err)
	}
	if h.mints[key] == 0 {
		h.inbound(step, deposit, 1, choice&0x3) // first use of this deposit
		return
	}
	before := h.snapshot()
	switch variant % 5 {
	case 0:
		msg.DeadlineUnix += 60
	case 1:
		msg.Nonce += 100
	case 2:
		msg.Destination = h.users[1].String()
	case 3:
		msg.Amount = "8"
	default:
		msg.SourceRef = strings.ToUpper(strings.TrimPrefix(msg.SourceRef, "0x"))
	}
	h.signAttestation(&msg, testChainID, h.twoSigners(choice)...)
	err = h.deliver("replay", func(ctx sdk.Context) error { _, err := h.server.SubmitAttestation(ctx, &msg); return err })
	if err == nil {
		h.t.Fatalf("step %d: replay of deposit %d (variant %d) accepted", step, deposit%6, variant%5)
	}
	h.requireUnchanged("replay", before)
}

// tampered submits attestations whose approvals must not count.
func (h *bridgeHarness) tampered(step int, deposit, variant, choice byte) {
	before := h.snapshot()
	msg := h.attestationMsg(deposit%6, amountFrom(choice), h.users[0])
	keys := h.twoSigners(choice)
	switch variant % 12 {
	case 0: // field changed after signing
		h.signAttestation(&msg, testChainID, keys...)
		msg.Amount = "1" + msg.Amount
	case 1:
		h.signAttestation(&msg, testChainID, keys...)
		msg.Destination = h.users[2].String()
	case 2:
		h.signAttestation(&msg, testChainID, keys...)
		msg.DeadlineUnix++
	case 3:
		h.signAttestation(&msg, testChainID, keys...)
		msg.SourceRef = h.depositRef(deposit%6 + 100)
	case 4: // one signer twice, as v and v+27
		h.signAttestation(&msg, testChainID, keys[0])
		twin := append([]byte(nil), msg.Signatures[0]...)
		twin[64] += 27
		msg.Signatures = append(msg.Signatures, twin)
	case 5: // high-s twin of a valid approval
		h.signAttestation(&msg, testChainID, keys...)
		twin := append([]byte(nil), msg.Signatures[1]...)
		s := new(big.Int).SetBytes(twin[32:64])
		s.Sub(crypto.S256().Params().N, s).FillBytes(twin[32:64])
		twin[64] ^= 1
		msg.Signatures[1] = twin
	case 6: // unconfigured signer
		h.signAttestation(&msg, testChainID, keys[0], h.outsider)
	case 7: // the guardian is not a bridge signer
		h.signAttestation(&msg, testChainID, keys[0], h.guardian)
	case 8: // approval for another network
		h.signAttestation(&msg, types.MainnetChainID, keys...)
	case 9: // approval from the control domain
		action := types.ControlAction{RouteID: msg.RouteId, Action: types.ActionResumeRoute, PayloadHash: "0x" + strings.Repeat("33", 32), Nonce: 1, NotBeforeUnix: 1, ExpiresUnix: h.now() + 60}
		digest, err := types.ControlDigest(action, testChainID)
		if err != nil {
			h.t.Fatal(err)
		}
		msg.Signatures = sign(h.t, digest, keys...)
	case 10: // approval from the guardian-pause domain
		digest, err := types.GuardianPauseDigest(types.GuardianPauseAction{RouteID: msg.RouteId, Nonce: 1, ExpiresUnix: h.now() + 60}, testChainID)
		if err != nil {
			h.t.Fatal(err)
		}
		msg.Signatures = sign(h.t, digest, keys...)
	default: // signed non-canonical amount
		msg.Amount = nonCanonical(msg.Amount, choice)
		h.signAttestation(&msg, testChainID, keys...)
	}
	err := h.deliver("tampered inbound", func(ctx sdk.Context) error { _, err := h.server.SubmitAttestation(ctx, &msg); return err })
	if err == nil {
		h.t.Fatalf("step %d: tampered attestation (variant %d) accepted", step, variant%12)
	}
	h.requireUnchanged("tampered inbound", before)
}

func (h *bridgeHarness) outbound(step int, userByte, amountByte, variant byte) {
	before := h.snapshot()
	user := h.users[int(userByte)%3]
	amount := amountFrom(amountByte)
	canonical := variant%4 != 0
	if !canonical {
		amount = nonCanonical(amount, variant>>2)
	}
	destination := []string{"0x00000000000000000000000000000000000000C5", "0x00000000000000000000000000000000000000c5", " 0x00000000000000000000000000000000000000C5"}[int(variant>>4)%3]
	msg := &types.MsgInitiateOutboundTransfer{Sender: user.String(), RouteId: before.config.RouteID, Destination: destination, Amount: amount}
	want := false
	value, ok := oracleAmount(amount)
	if canonical && ok {
		want = before.enabled && !before.paused && h.fitsLimits(before, value) &&
			value.Cmp(before.outstanding) <= 0 && value.Cmp(h.balance(user)) <= 0
	}
	balance := h.balance(user)
	ctx := h.ctx
	err := h.deliver("outbound", func(ctx sdk.Context) error { _, err := h.server.InitiateOutboundTransfer(ctx, msg); return err })
	if (err == nil) != want {
		h.t.Fatalf("step %d: outbound %q accepted=%v, model %v (err %v, before %+v)", step, amount, err == nil, want, err, before)
	}
	if err != nil {
		h.requireUnchanged("outbound", before)
		return
	}
	if spent := new(big.Int).Sub(balance, h.balance(user)); spent.Cmp(value) != 0 {
		h.t.Fatalf("step %d: sender spent %s for a transfer of %s", step, spent, amount)
	}
	events := ctx.EventManager().Events()
	event := events[len(events)-1]
	attributes := map[string]string{}
	for _, attribute := range event.Attributes {
		attributes[attribute.Key] = attribute.Value
	}
	if event.Type != "bridge_outbound_burned" || attributes["amount"] != value.String() ||
		attributes["destination"] != common.HexToAddress(strings.TrimSpace(destination)).Hex() {
		h.t.Fatalf("step %d: outbound event %v does not carry the canonical burned values", step, attributes)
	}
	h.burned.Add(h.burned, value)
	h.recordTransfer(value)
}

func (h *bridgeHarness) pause(step int, variant byte) {
	before := h.snapshot()
	mode := variant % 5
	if mode == 4 && len(h.usedPauses) == 0 {
		mode = 0
	}
	if mode == 4 {
		// Replay of an already applied guardian pause.
		used := h.usedPauses[int(variant)%len(h.usedPauses)]
		if err := h.deliver("pause replay", func(ctx sdk.Context) error { _, err := h.server.EmergencyPauseRoute(ctx, used); return err }); err == nil {
			h.t.Fatalf("step %d: replayed guardian pause accepted", step)
		}
		h.requireUnchanged("pause replay", before)
		return
	}
	h.pauseNonce++
	action := types.GuardianPauseAction{RouteID: before.config.RouteID, Nonce: h.pauseNonce, ExpiresUnix: h.now() + 600}
	signer, chainID := h.guardian, testChainID
	switch mode {
	case 1:
		action.ExpiresUnix = h.now() // expired
	case 2:
		signer = h.signers[0] // not the guardian
	case 3:
		chainID = types.MainnetChainID
	}
	digest, err := types.GuardianPauseDigest(action, chainID)
	if err != nil {
		h.t.Fatal(err)
	}
	msg := &types.MsgEmergencyPauseRoute{Submitter: h.users[0].String(), RouteId: action.RouteID, Nonce: action.Nonce, ExpiresUnix: action.ExpiresUnix, GuardianSignature: sign(h.t, digest, signer)[0]}
	want := mode == 0 && !before.paused
	err = h.deliver("pause", func(ctx sdk.Context) error { _, err := h.server.EmergencyPauseRoute(ctx, msg); return err })
	if (err == nil) != want {
		h.t.Fatalf("step %d: guardian pause variant %d accepted=%v, model %v (err %v)", step, mode, err == nil, want, err)
	}
	if err != nil {
		h.requireUnchanged("pause", before)
		return
	}
	h.usedPauses = append(h.usedPauses, msg)
	if !h.paused() {
		h.t.Fatalf("step %d: accepted pause did not pause", step)
	}
}

// controlNonce returns the nonce a control action uses: the next one, a
// consumed one, or one that skips ahead.
func (h *bridgeHarness) controlNonceFor(variant byte) (uint64, bool) {
	switch variant % 4 {
	case 0, 1:
		return h.controlNonce + 1, true
	case 2:
		return h.controlNonce, false // stale (or zero)
	default:
		return h.controlNonce + 2 + uint64(variant>>2), false // skipped
	}
}

func (h *bridgeHarness) controlWindow(variant byte) (int64, int64, bool) {
	now := h.now()
	switch variant % 8 {
	case 6:
		return now + 1, now + 600, false // not yet valid
	case 7:
		return now - 600, now, false // expired
	default:
		return now - 10, now + 600, true
	}
}

func (h *bridgeHarness) resume(step int, nonceVariant, windowVariant byte) {
	before := h.snapshot()
	nonce, nextNonce := h.controlNonceFor(nonceVariant)
	notBefore, expires, active := h.controlWindow(windowVariant)
	msg := &types.MsgResumeRoute{Submitter: h.users[0].String(), RouteId: before.config.RouteID, Nonce: nonce, NotBeforeUnix: notBefore, ExpiresUnix: expires}
	payloadHash, err := RouteStatePayloadHash(before.config, RouteState{Paused: false})
	if err != nil {
		h.t.Fatal(err)
	}
	if digest, err := types.ControlDigest(msg.Action(common.BytesToHash(payloadHash[:]).Hex()), testChainID); err == nil {
		msg.Signatures = sign(h.t, digest, h.twoSigners(windowVariant>>3)...)
	}
	want := before.paused && nextNonce && active
	err = h.deliver("resume", func(ctx sdk.Context) error { _, err := h.server.ResumeRoute(ctx, msg); return err })
	if (err == nil) != want {
		h.t.Fatalf("step %d: resume nonce %d (last %d) accepted=%v, model %v (err %v)", step, nonce, h.controlNonce, err == nil, want, err)
	}
	if err != nil {
		h.requireUnchanged("resume", before)
		return
	}
	h.controlNonce = nonce
	if h.paused() {
		h.t.Fatalf("step %d: accepted resume did not resume", step)
	}
}

func (h *bridgeHarness) update(step int, nonceVariant, limits, flags byte) {
	before := h.snapshot()
	nonce, nextNonce := h.controlNonceFor(nonceVariant)
	notBefore, expires, active := h.controlWindow(flags)
	caps := []*big.Int{new(big.Int).Sub(before.outstanding, big.NewInt(1)), before.outstanding, big.NewInt(2500), big.NewInt(5000)}
	maxOutstanding := caps[int(limits>>4)%len(caps)]
	if maxOutstanding.Sign() <= 0 {
		maxOutstanding = big.NewInt(1)
	}
	next := before.config
	next.MaxTransferAmount = []string{"50", "300", "1000"}[int(limits)%3]
	next.DailyLimit = []string{"100", "1000", "5000"}[int(limits>>2)%3]
	next.MaxOutstandingAmount = maxOutstanding.String()
	next.Enabled = flags&0x80 == 0
	msg := &types.MsgUpdateRouteConfig{
		Submitter: h.users[0].String(), RouteId: next.RouteID, BridgeSigners: next.BridgeSigners, Guardian: next.Guardian,
		MaxTransferAmount: next.MaxTransferAmount, DailyLimit: next.DailyLimit, MaxOutstandingAmount: next.MaxOutstandingAmount,
		Enabled: next.Enabled, Nonce: nonce, NotBeforeUnix: notBefore, ExpiresUnix: expires,
	}
	payloadHash, err := types.RouteConfigPayloadHash(next)
	if err != nil {
		h.t.Fatal(err)
	}
	if digest, err := types.ControlDigest(msg.Action(common.BytesToHash(payloadHash[:]).Hex()), testChainID); err == nil {
		msg.Signatures = sign(h.t, digest, h.twoSigners(flags>>3)...)
	}
	// A cap below the outstanding amount would leave a state that violates
	// outstanding <= cap and fails genesis validation.
	want := nextNonce && active && maxOutstanding.Cmp(before.outstanding) >= 0
	err = h.deliver("update", func(ctx sdk.Context) error { _, err := h.server.UpdateRouteConfig(ctx, msg); return err })
	if (err == nil) != want {
		h.t.Fatalf("step %d: config update nonce %d (last %d) cap %s (outstanding %s) accepted=%v, model %v (err %v)",
			step, nonce, h.controlNonce, maxOutstanding, before.outstanding, err == nil, want, err)
	}
	if err != nil {
		h.requireUnchanged("update", before)
		return
	}
	h.controlNonce = nonce
}

// warp moves block time forward: seconds, hours, to just before or exactly
// at the next UTC midnight, or several days ahead.
func (h *bridgeHarness) warp(mode, amount byte) {
	now := h.now()
	nextMidnight := (now/harnessDaySeconds + 1) * harnessDaySeconds
	switch mode % 5 {
	case 0:
		now += int64(amount)
	case 1:
		now += int64(amount) * 3600
	case 2:
		now = nextMidnight - 1
	case 3:
		now = nextMidnight
	default:
		now = nextMidnight + int64(amount%7)*harnessDaySeconds + int64(amount)
	}
	if now <= h.now() {
		now = h.now() + 1
	}
	h.ctx = h.ctx.WithBlockTime(time.Unix(now, 0).UTC()).WithBlockHeight(h.ctx.BlockHeight() + 1)
}

func runBridgeSequence(t *testing.T, program []byte) {
	h := newBridgeHarness(t)
	for step := 0; len(program) >= 4 && step < 96; step++ {
		op, a, b, c := program[0], program[1], program[2], program[3]
		program = program[4:]
		before := h.snapshot()
		var name string
		switch op % 10 {
		case 0, 1:
			name = "inbound"
			h.inbound(step, a, b, c)
		case 2:
			name = "replay"
			h.replay(step, a, b, c)
		case 3:
			name = "tampered"
			h.tampered(step, a, b, c)
		case 4, 5:
			name = "outbound"
			h.outbound(step, a, b, c)
		case 6:
			name = "pause"
			h.pause(step, a)
		case 7:
			name = "resume"
			h.resume(step, a, b)
		case 8:
			name = "update"
			h.update(step, a, b, c)
		default:
			name = "warp"
			h.warp(a, b)
		}
		h.checkInvariants(step, name, before)
	}
}

func FuzzBridgeOperationSequence(f *testing.F) {
	seeds := [][]byte{
		// mint, replay, burn
		{0, 0, 4, 0, 2, 0, 0, 0, 4, 0, 3, 1},
		// mint up to the daily limit, cross UTC midnight, mint again
		{0, 0, 7, 0, 0, 1, 7, 4, 0, 2, 7, 0, 0, 3, 7, 0, 9, 2, 0, 0, 9, 3, 0, 0, 0, 4, 7, 0},
		// pause, mint and burn while paused, stale and skipped resume, resume
		{0, 0, 5, 0, 6, 0, 0, 0, 0, 1, 5, 0, 4, 0, 1, 1, 7, 2, 0, 0, 7, 3, 0, 0, 7, 0, 0, 0, 4, 0, 1, 1},
		// lower the cap below the outstanding amount
		{0, 0, 7, 0, 0, 1, 7, 4, 8, 0, 0, 0, 0, 2, 3, 0},
		// disable the route, try to mint and burn, re-enable with the next nonce
		{0, 0, 5, 0, 8, 0, 0, 0x80, 0, 1, 5, 0, 4, 0, 1, 1, 8, 0, 0x10, 0, 0, 1, 5, 0},
		// every tampering variant
		{3, 0, 0, 0, 3, 0, 1, 0, 3, 0, 2, 0, 3, 0, 3, 0, 3, 0, 4, 0, 3, 0, 5, 0, 3, 0, 6, 0, 3, 0, 7, 0, 3, 0, 8, 0, 3, 0, 9, 0, 3, 0, 10, 0, 3, 0, 11, 0},
		// reference spelling variant of a minted deposit, guardian pause replay
		{0, 2, 5, 0, 0, 2, 5, 0x40, 2, 2, 4, 0, 6, 0, 0, 0, 7, 0, 0, 0, 6, 4, 0, 0, 6, 9, 0, 0},
		// non-canonical outbound amounts and supply cap
		{0, 0, 7, 0, 4, 0, 1, 0, 4, 1, 2, 4, 4, 2, 3, 8, 8, 0, 0x32, 0, 0, 3, 11, 0, 0, 4, 11, 0, 9, 4, 3, 0, 0, 5, 13, 0},
	}
	for _, seed := range seeds {
		f.Add(seed)
	}
	f.Fuzz(runBridgeSequence)
}

// FuzzAmountAgreement: for any amount string, validation, limits, minting and
// burning either all reject it or all use the same number.
func FuzzAmountAgreement(f *testing.F) {
	for _, seed := range []string{"1", "10", "010", "+10", "08", "0", "300", "301", " 10", "10\n", "0x1", "1e2", "１"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, amount string) {
		value, canonical := oracleAmount(amount)
		outbound := &types.MsgInitiateOutboundTransfer{RouteId: "cronos-testnet-xitcoin-testnet", Destination: "0x00000000000000000000000000000000000000C5", Amount: amount}

		h := newBridgeHarness(t)
		outbound.Sender = h.users[0].String()
		if err := outbound.ValidateBasic(); (err == nil) != canonical {
			t.Fatalf("ValidateBasic accepted=%v for %q, canonical=%v", err == nil, amount, canonical)
		}
		fits := canonical && value.Cmp(big.NewInt(300)) <= 0

		// Inbound: mint == limit usage == outstanding increase == amount.
		msg := h.attestationMsg(1, amount, h.users[1])
		h.signAttestation(&msg, testChainID, h.signers[0], h.signers[1])
		before := h.snapshot()
		err := h.deliver("inbound", func(ctx sdk.Context) error { _, err := h.server.SubmitAttestation(ctx, &msg); return err })
		if (err == nil) != fits {
			t.Fatalf("inbound %q accepted=%v, want %v (err %v)", amount, err == nil, fits, err)
		}
		if err != nil {
			h.requireUnchanged("inbound", before)
			return
		}
		after := h.snapshot()
		minted := new(big.Int).Sub(after.supply, before.supply)
		if minted.Cmp(value) != 0 || after.usage.Cmp(value) != 0 || new(big.Int).Sub(after.outstanding, before.outstanding).Cmp(value) != 0 {
			t.Fatalf("inbound %q: minted %s, daily usage %s, outstanding +%s", amount, minted, after.usage, new(big.Int).Sub(after.outstanding, before.outstanding))
		}

		// Outbound of the same amount: burn == limit usage == outstanding decrease.
		before = after
		err = h.deliver("outbound", func(ctx sdk.Context) error { _, err := h.server.InitiateOutboundTransfer(ctx, outbound); return err })
		if err != nil {
			t.Fatalf("outbound %q rejected: %v", amount, err)
		}
		after = h.snapshot()
		burned := new(big.Int).Sub(before.supply, after.supply)
		if burned.Cmp(value) != 0 || new(big.Int).Sub(after.usage, before.usage).Cmp(value) != 0 || new(big.Int).Sub(before.outstanding, after.outstanding).Cmp(value) != 0 {
			t.Fatalf("outbound %q: burned %s, daily usage +%s, outstanding -%s", amount, burned, new(big.Int).Sub(after.usage, before.usage), new(big.Int).Sub(before.outstanding, after.outstanding))
		}
		event := findEvent(t, h.ctx.EventManager().Events(), "bridge_outbound_burned")
		if event["amount"] != value.String() {
			t.Fatalf("outbound event amount %q, burned %s", event["amount"], value)
		}
	})
}
