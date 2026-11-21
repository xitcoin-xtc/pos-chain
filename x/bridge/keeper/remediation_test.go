package keeper

import (
	"bytes"
	"crypto/ecdsa"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/xitcoin-xtc/pos-chain/x/bridge/types"

	sdkmath "cosmossdk.io/math"

	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

type settlementFixture struct {
	ctx    sdk.Context
	keeper Keeper
	bank   *fakeBankKeeper
	config types.RouteConfig
	keys   []*ecdsa.PrivateKey
}

func newSettlementFixture(t *testing.T, chainID string) settlementFixture {
	t.Helper()
	key := storetypes.NewKVStoreKey(types.StoreKey)
	ctx := testContext(key, storetypes.NewTransientStoreKey("bridge_remediation_test")).
		WithChainID(chainID).WithBlockTime(time.Unix(1800000000, 0))
	bank := &fakeBankKeeper{supply: sdkmath.ZeroInt(), blocked: map[string]bool{}}
	k, err := NewSettlementKeeper(key, bank, "axtc", "1000000", testAuthority())
	if err != nil {
		t.Fatal(err)
	}
	keys := make([]*ecdsa.PrivateKey, 3)
	signers := make([]string, 3)
	for i := range keys {
		keys[i], err = crypto.GenerateKey()
		if err != nil {
			t.Fatal(err)
		}
		signers[i] = crypto.PubkeyToAddress(keys[i].PublicKey).Hex()
	}
	config := types.RouteConfig{
		RouteID: "cronos-testnet-xitcoin-testnet", BridgeSigners: signers,
		Guardian:          common.HexToAddress("0x0000000000000000000000000000000000000004").Hex(),
		MaxTransferAmount: "100", DailyLimit: "1000", MaxOutstandingAmount: "1000", Enabled: true,
	}
	if err := k.SetRouteConfig(ctx, config); err != nil {
		t.Fatal(err)
	}
	return settlementFixture{ctx: ctx, keeper: k, bank: bank, config: config, keys: keys}
}

func (f settlementFixture) submission(t *testing.T, chainID string, mutate func(*types.MsgSubmitAttestation)) *types.MsgSubmitAttestation {
	t.Helper()
	msg := validBridgeMessage(t, f.config.RouteID, nil)
	if mutate != nil {
		mutate(&msg)
	}
	digest, err := types.SigningDigest(msg.Attestation(), chainID)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range f.keys[:2] {
		signature, err := crypto.Sign(digest.Bytes(), key)
		if err != nil {
			t.Fatal(err)
		}
		msg.Signatures = append(msg.Signatures, signature)
	}
	return &msg
}

// Audit finding 1: two attestations for one Cronos deposit that differ only
// in their deadline (or any other signed field) must not both mint.
func TestOneSourceDepositMintsOnceWhateverTheDeadline(t *testing.T) {
	f := newSettlementFixture(t, testChainID)
	server := NewMsgServer(f.keeper)
	first := f.submission(t, testChainID, nil)
	if _, err := server.SubmitAttestation(f.ctx, first); err != nil {
		t.Fatalf("first attestation rejected: %v", err)
	}
	for name, mutate := range map[string]func(*types.MsgSubmitAttestation){
		"later deadline": func(m *types.MsgSubmitAttestation) { m.DeadlineUnix += 600 },
		"other destination": func(m *types.MsgSubmitAttestation) {
			m.Destination = sdk.AccAddress(bytes.Repeat([]byte{7}, 20)).String()
		},
		"other nonce": func(m *types.MsgSubmitAttestation) { m.Nonce = 2 },
	} {
		second := f.submission(t, testChainID, mutate)
		if _, err := server.SubmitAttestation(f.ctx, second); err != ErrAttestationAlreadyProcessed {
			t.Fatalf("%s: second attestation for the same deposit error = %v", name, err)
		}
	}
	if got := f.keeper.OutstandingAmount(f.ctx).String(); got != "10" {
		t.Fatalf("outstanding = %s, want exactly one mint of 10", got)
	}
	if got := f.bank.supply.String(); got != "10" {
		t.Fatalf("supply = %s, want 10", got)
	}
}

func TestAttestationSignedForAnotherNetworkIsRejected(t *testing.T) {
	f := newSettlementFixture(t, testChainID)
	msg := f.submission(t, types.MainnetChainID, nil)
	if _, err := NewMsgServer(f.keeper).SubmitAttestation(f.ctx, msg); err == nil {
		t.Fatal("approval signed for mainnet was accepted on testnet")
	}
	if f.bank.supply.Sign() != 0 {
		t.Fatal("rejected attestation minted")
	}
}

func TestAttestationSourceChainMustMatchRoute(t *testing.T) {
	f := newSettlementFixture(t, testChainID)
	msg := f.submission(t, testChainID, func(m *types.MsgSubmitAttestation) { m.SourceChainId = "25" })
	if _, err := NewMsgServer(f.keeper).SubmitAttestation(f.ctx, msg); err != ErrAttestationSourceChainMismatch {
		t.Fatalf("source chain mismatch error = %v", err)
	}

	mainnet := newSettlementFixture(t, types.MainnetChainID)
	accepted := mainnet.submission(t, types.MainnetChainID, func(m *types.MsgSubmitAttestation) { m.SourceChainId = "25" })
	if _, err := NewMsgServer(mainnet.keeper).SubmitAttestation(mainnet.ctx, accepted); err != nil {
		t.Fatalf("mainnet attestation from Cronos mainnet rejected: %v", err)
	}
	testnetSource := mainnet.submission(t, types.MainnetChainID, func(m *types.MsgSubmitAttestation) {
		m.SourceChainId = "338"
		m.SourceRef = "0x" + strings.Repeat("d", 64)
	})
	if _, err := NewMsgServer(mainnet.keeper).SubmitAttestation(mainnet.ctx, testnetSource); err != ErrAttestationSourceChainMismatch {
		t.Fatalf("Cronos Testnet deposit on mainnet error = %v", err)
	}
}

func TestInboundToBlockedModuleAccountIsRejectedWithoutMint(t *testing.T) {
	f := newSettlementFixture(t, testChainID)
	blocked := sdk.AccAddress(bytes.Repeat([]byte{1}, 20))
	f.bank.blocked[blocked.String()] = true
	msg := f.submission(t, testChainID, nil)
	if _, err := NewMsgServer(f.keeper).SubmitAttestation(f.ctx, msg); err != ErrBlockedDestination {
		t.Fatalf("blocked destination error = %v", err)
	}
	if f.bank.supply.Sign() != 0 || f.bank.minted != nil {
		t.Fatal("blocked destination minted")
	}
}

func TestOutboundToVaultRefusedRecipientIsRejectedBeforeBurn(t *testing.T) {
	f := newSettlementFixture(t, testChainID)
	f.keeper.setOutstandingAmount(f.ctx, sdkmath.NewInt(50))
	sender := sdk.AccAddress(bytes.Repeat([]byte{3}, 20))
	for _, destination := range []string{"0x000000000000000000000000000000000000dEaD", "0x0000000000000000000000000000000000000000"} {
		if _, _, err := f.keeper.InitiateOutboundTransfer(f.ctx, f.config, sender, destination, "5"); err != types.ErrInvalidCronosDestination {
			t.Fatalf("destination %s error = %v", destination, err)
		}
	}
	if f.bank.burned != nil || f.keeper.OutstandingAmount(f.ctx).String() != "50" || f.keeper.DailyUsage(f.ctx, f.config.RouteID).Sign() != 0 {
		t.Fatal("rejected outbound transfer changed state")
	}
}

func TestInitializeRouteDerivesAndUpdatePreservesSourceChain(t *testing.T) {
	for chainID, want := range map[string]string{testChainID: "338", types.MainnetChainID: "25"} {
		key := storetypes.NewKVStoreKey(types.StoreKey)
		ctx := testContext(key, storetypes.NewTransientStoreKey("bridge_init_source_test")).WithChainID(chainID)
		k := NewKeeper(key)
		k.authority = testAuthority()
		request := &types.MsgInitializeRouteConfig{
			Authority: testAuthority(), RouteId: "cronos-testnet-xitcoin-testnet",
			BridgeSigners: []string{
				"0x0000000000000000000000000000000000000001",
				"0x0000000000000000000000000000000000000002",
				"0x0000000000000000000000000000000000000003",
			},
			Guardian:          "0x0000000000000000000000000000000000000004",
			MaxTransferAmount: "10", DailyLimit: "100", MaxOutstandingAmount: "1000",
		}
		if _, err := NewMsgServer(k).InitializeRouteConfig(ctx, request); err != nil {
			t.Fatal(err)
		}
		config, _, err := k.GetRouteConfig(ctx)
		if err != nil || config.SourceChainID != want {
			t.Fatalf("%s source chain = %q (err %v), want %s", chainID, config.SourceChainID, err, want)
		}
	}

	ctx, k, current, keys, _ := controlServerSetup(t)
	current.SourceChainID = "338"
	if err := k.SetRouteConfig(ctx, current); err != nil {
		t.Fatal(err)
	}
	next := current
	next.SourceChainID = "25"
	action, signatures := signedConfigActionWithNonce(t, current, next, keys, 1)
	if err := k.ApplyRouteConfigUpdate(ctx, current, action, next, signatures); err != ErrControlPayloadMismatch {
		t.Fatalf("source chain change error = %v", err)
	}
}

func signedConfigActionWithNonce(t *testing.T, current, next types.RouteConfig, keys []*ecdsa.PrivateKey, nonce uint64) (types.ControlAction, [][]byte) {
	t.Helper()
	payloadHash, err := types.RouteConfigPayloadHash(next)
	if err != nil {
		t.Fatal(err)
	}
	action := types.ControlAction{
		RouteID: current.RouteID, Action: types.ActionUpdateRouteConfig,
		PayloadHash: common.BytesToHash(payloadHash[:]).Hex(), Nonce: nonce,
		NotBeforeUnix: 1800000000, ExpiresUnix: 1800003600,
	}
	return action, signControl(t, action, keys)
}

// Audit finding 3: an older signed configuration that is still inside its
// validity window must not be applicable after a newer one.
func TestControlNoncesAreStrictlySequential(t *testing.T) {
	ctx, k, current, keys, _ := controlServerSetup(t)

	older := current
	older.DailyLimit = "40"
	olderAction, olderSignatures := signedConfigActionWithNonce(t, current, older, keys, 1)
	newer := current
	newer.DailyLimit = "60"

	gapAction, gapSignatures := signedConfigActionWithNonce(t, current, newer, keys, 2)
	if err := k.ApplyRouteConfigUpdate(ctx, current, gapAction, newer, gapSignatures); err != ErrControlNonceGap {
		t.Fatalf("skipped nonce error = %v", err)
	}
	if err := k.ApplyRouteConfigUpdate(ctx, current, olderAction, older, olderSignatures); err != nil {
		t.Fatalf("nonce 1 rejected: %v", err)
	}
	newerAction, newerSignatures := signedConfigActionWithNonce(t, older, newer, keys, 2)
	if err := k.ApplyRouteConfigUpdate(ctx, older, newerAction, newer, newerSignatures); err != nil {
		t.Fatalf("nonce 2 rejected: %v", err)
	}
	if err := k.ApplyRouteConfigUpdate(ctx, newer, olderAction, older, olderSignatures); err != ErrControlActionProcessed {
		t.Fatalf("stale signed configuration error = %v", err)
	}
	competing := current
	competing.DailyLimit = "70"
	competingAction, competingSignatures := signedConfigActionWithNonce(t, newer, competing, keys, 1)
	if err := k.ApplyRouteConfigUpdate(ctx, newer, competingAction, competing, competingSignatures); err != ErrControlActionProcessed {
		t.Fatalf("competing action with consumed nonce error = %v", err)
	}
	stored, _, err := k.GetRouteConfig(ctx)
	if err != nil || stored.DailyLimit != "60" || k.ControlNonce(ctx) != 2 {
		t.Fatalf("stored daily limit = %s nonce = %d err = %v", stored.DailyLimit, k.ControlNonce(ctx), err)
	}

	// Resume shares the sequence: a resume signed with nonce 2 is now stale.
	ctx.KVStore(k.storeKey).Set(routeStateKey, []byte(`{"paused":true}`))
	payloadHash, err := RouteStatePayloadHash(newer, RouteState{Paused: false})
	if err != nil {
		t.Fatal(err)
	}
	resume := types.ControlAction{RouteID: newer.RouteID, Action: types.ActionResumeRoute, PayloadHash: common.BytesToHash(payloadHash[:]).Hex(), Nonce: 2, NotBeforeUnix: 1800000000, ExpiresUnix: 1800003600}
	if err := k.ResumeRoute(ctx, newer, resume, signControl(t, resume, keys)); err != ErrControlActionProcessed {
		t.Fatalf("stale resume error = %v", err)
	}
	resume.Nonce = 3
	if err := k.ResumeRoute(ctx, newer, resume, signControl(t, resume, keys)); err != nil {
		t.Fatalf("sequential resume rejected: %v", err)
	}
}

func TestControlSignedForAnotherNetworkIsRejected(t *testing.T) {
	ctx, k, current, keys, _ := controlServerSetup(t)
	next := current
	next.DailyLimit = "40"
	payloadHash, err := types.RouteConfigPayloadHash(next)
	if err != nil {
		t.Fatal(err)
	}
	action := types.ControlAction{RouteID: current.RouteID, Action: types.ActionUpdateRouteConfig, PayloadHash: common.BytesToHash(payloadHash[:]).Hex(), Nonce: 1, NotBeforeUnix: 1800000000, ExpiresUnix: 1800003600}
	digest, err := types.ControlDigest(action, types.MainnetChainID)
	if err != nil {
		t.Fatal(err)
	}
	signatures := make([][]byte, 2)
	for i := range signatures {
		if signatures[i], err = crypto.Sign(digest.Bytes(), keys[i]); err != nil {
			t.Fatal(err)
		}
	}
	if err := k.ApplyRouteConfigUpdate(ctx, current, action, next, signatures); err == nil {
		t.Fatal("control action signed for mainnet was applied on testnet")
	}
	if k.ControlNonce(ctx) != 0 {
		t.Fatal("rejected control action consumed a nonce")
	}
}

func TestGenesisPreservesReplayKeysAndControlNonce(t *testing.T) {
	f := newSettlementFixture(t, testChainID)
	if _, err := NewMsgServer(f.keeper).SubmitAttestation(f.ctx, f.submission(t, testChainID, nil)); err != nil {
		t.Fatal(err)
	}
	f.keeper.setControlNonce(f.ctx, 4)
	exported := f.keeper.ExportGenesis(f.ctx)
	if err := exported.Validate(); err != nil {
		t.Fatalf("exported genesis invalid: %v", err)
	}
	if len(exported.ProcessedSourceDeposits) != 1 || exported.ControlNonce != 4 || exported.RouteConfig.SourceChainID != "" {
		t.Fatalf("unexpected export: %+v", exported)
	}

	key := storetypes.NewKVStoreKey(types.StoreKey)
	ctx := testContext(key, storetypes.NewTransientStoreKey("bridge_genesis_import_test")).WithBlockTime(time.Unix(1800000000, 0))
	bank := &fakeBankKeeper{supply: sdkmath.NewInt(10), blocked: map[string]bool{}}
	imported, err := NewSettlementKeeper(key, bank, "axtc", "1000000", testAuthority())
	if err != nil {
		t.Fatal(err)
	}
	imported.InitGenesis(ctx, exported)
	config, _, err := imported.GetRouteConfig(ctx)
	if err != nil || config.SourceChainID != "338" {
		t.Fatalf("imported source chain = %q err = %v", config.SourceChainID, err)
	}
	if imported.ControlNonce(ctx) != 4 {
		t.Fatal("control nonce lost across genesis")
	}
	replay := f.submission(t, testChainID, func(m *types.MsgSubmitAttestation) { m.DeadlineUnix += 60 })
	if _, err := NewMsgServer(imported).SubmitAttestation(ctx, replay); err != ErrAttestationAlreadyProcessed {
		t.Fatalf("replay after genesis import error = %v", err)
	}
}
