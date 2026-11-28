package keeper

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/xitcoin-xtc/pos-chain/x/bridge/types"

	sdkmath "cosmossdk.io/math"

	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// liveRouteID is the route ID stored on xitcoin-testnet-v2-1: the Cronos
// bytes32 route identifier.
const liveRouteID = "0x21121c16b53a726056a6683f00c7eb4da5501ce8a2abc8a4677e06f1e94b5cd9"

const (
	queryBlockUnix = 1800000000 // 2027-01-15T08:00:00Z, UTC day 20833
	queryHeight    = 42
)

type queryFixture struct {
	ctx    sdk.Context
	keeper Keeper
	server types.QueryServer
	msgs   types.MsgServer
	config types.RouteConfig
	keys   []*ecdsa.PrivateKey
}

func newQueryFixture(t *testing.T) queryFixture {
	t.Helper()
	key := storetypes.NewKVStoreKey(types.StoreKey)
	ctx := testContext(key, storetypes.NewTransientStoreKey("bridge_query_records_test")).
		WithBlockTime(time.Unix(queryBlockUnix, 0).UTC()).WithBlockHeight(queryHeight)
	k, err := NewSettlementKeeper(key, &fakeBankKeeper{supply: sdkmath.ZeroInt(), blocked: map[string]bool{}}, "axtc", "1000000", testAuthority())
	if err != nil {
		t.Fatal(err)
	}
	keys := make([]*ecdsa.PrivateKey, 3)
	signers := make([]string, 3)
	for i := range keys {
		seed := sha256.Sum256([]byte{'q', byte(i)})
		keys[i], err = crypto.ToECDSA(seed[:])
		if err != nil {
			t.Fatal(err)
		}
		signers[i] = crypto.PubkeyToAddress(keys[i].PublicKey).Hex()
	}
	config := types.RouteConfig{
		RouteID: liveRouteID, BridgeSigners: signers,
		Guardian:          common.HexToAddress("0x0000000000000000000000000000000000000004").Hex(),
		MaxTransferAmount: "100", DailyLimit: "150", MaxOutstandingAmount: "1000", Enabled: true, SourceChainID: "338",
	}
	// Genesis without signer_set_version: the route starts at version 1.
	k.InitGenesis(ctx, types.GenesisState{RouteConfig: &config})
	return queryFixture{ctx: ctx, keeper: k, server: NewQueryServer(k), msgs: NewMsgServer(k), config: config, keys: keys}
}

func (f queryFixture) submission(t *testing.T, deposit byte, amount string, signers ...*ecdsa.PrivateKey) *types.MsgSubmitAttestation {
	t.Helper()
	msg := validBridgeMessage(t, f.config.RouteID, nil)
	msg.SourceRef = "0x" + strings.Repeat(hex.EncodeToString([]byte{deposit}), 32)
	msg.Amount = amount
	digest, err := types.SigningDigest(msg.Attestation(), testChainID)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range signers {
		signature, err := crypto.Sign(digest.Bytes(), key)
		if err != nil {
			t.Fatal(err)
		}
		msg.Signatures = append(msg.Signatures, signature)
	}
	return &msg
}

func txHashOf(txBytes []byte) string {
	sum := sha256.Sum256(txBytes)
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

func requireCode(t *testing.T, err error, code codes.Code, what string) {
	t.Helper()
	if status.Code(err) != code {
		t.Fatalf("%s: error %v, want code %s", what, err, code)
	}
}

func TestAttestationQueryDistinguishesAbsentFromProcessed(t *testing.T) {
	f := newQueryFixture(t)
	msg := f.submission(t, 0xc1, "60", f.keys[0], f.keys[1])
	id, err := msg.Attestation().ID()
	if err != nil {
		t.Fatal(err)
	}
	idText := types.HexID(id)

	before, err := f.server.Attestation(f.ctx, &types.QueryAttestationRequest{AttestationId: idText})
	if err != nil {
		t.Fatal(err)
	}
	if before.Status != types.AttestationState_ATTESTATION_STATE_ABSENT || before.Processed || before.Record != nil ||
		before.AttestationId != idText || before.Height != queryHeight {
		t.Fatalf("unexpected response before processing: %+v", before)
	}

	txBytes := []byte("signed MsgSubmitAttestation transaction")
	if _, err := f.msgs.SubmitAttestation(f.ctx.WithTxBytes(txBytes), msg); err != nil {
		t.Fatalf("valid attestation rejected: %v", err)
	}

	// Any spelling of the ID gives the canonical answer.
	for _, spelling := range []string{idText, strings.ToUpper(idText), strings.TrimPrefix(idText, "0x")} {
		after, err := f.server.Attestation(f.ctx, &types.QueryAttestationRequest{AttestationId: spelling})
		if err != nil {
			t.Fatal(err)
		}
		if after.Status != types.AttestationState_ATTESTATION_STATE_PROCESSED || !after.Processed || after.AttestationId != idText || after.Record == nil {
			t.Fatalf("unexpected response after processing (%s): %+v", spelling, after)
		}
	}
	after, err := f.server.Attestation(f.ctx, &types.QueryAttestationRequest{AttestationId: idText})
	if err != nil {
		t.Fatal(err)
	}
	payloadHash, err := msg.Attestation().PayloadHash(testChainID)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := types.SigningDigest(msg.Attestation(), testChainID)
	if err != nil {
		t.Fatal(err)
	}
	want := types.AttestationRecord{
		AttestationId: idText, RouteId: liveRouteID, Direction: "cronos_to_xitcoin", SourceChainId: "338",
		SourceRef: strings.Repeat("c1", 32), Nonce: msg.Nonce, Recipient: msg.Destination, Amount: "60",
		DeadlineUnix: msg.DeadlineUnix, SignerSetVersion: 1,
		Approvers:          []string{crypto.PubkeyToAddress(f.keys[0].PublicKey).Hex(), crypto.PubkeyToAddress(f.keys[1].PublicKey).Hex()},
		DestinationChainId: testChainID, PayloadHash: types.HexID(payloadHash), SigningDigest: digest.Hex(),
		Height: queryHeight, TxHash: txHashOf(txBytes),
	}
	if !reflect.DeepEqual(*after.Record, want) {
		t.Fatalf("record\n got %+v\nwant %+v", *after.Record, want)
	}
	if err := after.Record.Validate(); err != nil {
		t.Fatalf("stored record is not self-consistent: %v", err)
	}

	// The source-deposit lookup and the legacy flag agree.
	bySource, err := f.server.SourceDeposit(f.ctx, &types.QuerySourceDepositRequest{SourceChainId: "338", SourceRef: "0X" + strings.Repeat("C1", 32)})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(bySource, after) {
		t.Fatalf("source deposit lookup %+v differs from %+v", bySource, after)
	}
	legacy, err := f.server.AttestationStatus(f.ctx, &types.QueryAttestationStatusRequest{AttestationId: idText})
	if err != nil || !legacy.Processed || legacy.AttestationId != idText {
		t.Fatalf("legacy status %+v, err %v", legacy, err)
	}

	// The same deposit on another source chain is another replay key.
	other, err := f.server.SourceDeposit(f.ctx, &types.QuerySourceDepositRequest{SourceChainId: "25", SourceRef: strings.Repeat("c1", 32)})
	if err != nil || other.Status != types.AttestationState_ATTESTATION_STATE_ABSENT {
		t.Fatalf("other source chain: %+v, err %v", other, err)
	}
}

func TestAttestationQueryReportsAKeyWithoutRecordAsProcessed(t *testing.T) {
	f := newQueryFixture(t)
	id, err := types.SourceDepositKey("338", "0x"+strings.Repeat("ab", 32))
	if err != nil {
		t.Fatal(err)
	}
	// A replay key consumed before records were kept (or imported from
	// processed_source_deposits) must never read as absent.
	f.keeper.MarkProcessed(f.ctx, id)
	response, err := f.server.Attestation(f.ctx, &types.QueryAttestationRequest{AttestationId: types.HexID(id)})
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != types.AttestationState_ATTESTATION_STATE_PROCESSED_WITHOUT_RECORD || !response.Processed || response.Record != nil {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestQueriesRejectMalformedAndUnknownIDs(t *testing.T) {
	f := newQueryFixture(t)
	for _, value := range []string{"", "0x01", strings.Repeat("z", 64), "0x" + strings.Repeat("a", 66)} {
		_, err := f.server.Attestation(f.ctx, &types.QueryAttestationRequest{AttestationId: value})
		requireCode(t, err, codes.InvalidArgument, "attestation ID "+value)
		_, err = f.server.OutboundTransfer(f.ctx, &types.QueryOutboundTransferRequest{RequestId: value})
		requireCode(t, err, codes.InvalidArgument, "request ID "+value)
	}
	for _, request := range []*types.QuerySourceDepositRequest{
		{SourceChainId: "0338", SourceRef: strings.Repeat("ab", 32)},
		{SourceChainId: "", SourceRef: strings.Repeat("ab", 32)},
		{SourceChainId: "338", SourceRef: strings.Repeat("ab", 31)},
		{SourceChainId: "338", SourceRef: " " + strings.Repeat("ab", 32)},
	} {
		_, err := f.server.SourceDeposit(f.ctx, request)
		requireCode(t, err, codes.InvalidArgument, "source deposit "+request.String())
	}
	_, err := f.server.OutboundTransfer(f.ctx, &types.QueryOutboundTransferRequest{RequestId: strings.Repeat("ab", 32)})
	requireCode(t, err, codes.NotFound, "unknown request ID")
	// Only the stored form of the route ID matches: not the relayer label,
	// not another spelling of the bytes32 value.
	for _, routeID := range []string{"cronos-testnet-xitcoin-testnet", strings.ToUpper(liveRouteID), strings.TrimPrefix(liveRouteID, "0x"), ""} {
		_, err := f.server.Route(f.ctx, &types.QueryRouteRequest{RouteId: routeID})
		requireCode(t, err, codes.NotFound, "route "+routeID)
	}

	_, err = f.server.Params(f.ctx, nil)
	requireCode(t, err, codes.InvalidArgument, "nil params request")
	_, err = f.server.Route(f.ctx, nil)
	requireCode(t, err, codes.InvalidArgument, "nil route request")
	_, err = f.server.Outstanding(f.ctx, nil)
	requireCode(t, err, codes.InvalidArgument, "nil outstanding request")
	_, err = f.server.DailyUsage(f.ctx, nil)
	requireCode(t, err, codes.InvalidArgument, "nil daily usage request")
	_, err = f.server.Attestation(f.ctx, nil)
	requireCode(t, err, codes.InvalidArgument, "nil attestation request")
	_, err = f.server.SourceDeposit(f.ctx, nil)
	requireCode(t, err, codes.InvalidArgument, "nil source deposit request")
	_, err = f.server.OutboundTransfer(f.ctx, nil)
	requireCode(t, err, codes.InvalidArgument, "nil outbound transfer request")
}

func TestRouteQueriesWithoutRouteReturnNotFound(t *testing.T) {
	key := storetypes.NewKVStoreKey(types.StoreKey)
	ctx := testContext(key, storetypes.NewTransientStoreKey("bridge_query_no_route_test")).WithBlockTime(time.Unix(queryBlockUnix, 0))
	server := NewQueryServer(NewKeeper(key))
	_, err := server.Route(ctx, &types.QueryRouteRequest{RouteId: liveRouteID})
	requireCode(t, err, codes.NotFound, "route")
	_, err = server.Outstanding(ctx, &types.QueryOutstandingRequest{})
	requireCode(t, err, codes.NotFound, "outstanding")
	_, err = server.DailyUsage(ctx, &types.QueryDailyUsageRequest{})
	requireCode(t, err, codes.NotFound, "daily usage")
	response, err := server.Attestation(ctx, &types.QueryAttestationRequest{AttestationId: strings.Repeat("ab", 32)})
	if err != nil || response.Status != types.AttestationState_ATTESTATION_STATE_ABSENT {
		t.Fatalf("attestation without route: %+v, err %v", response, err)
	}
}

// settle mints 60 and burns 25 on the fixture's UTC day and returns the
// outbound response and transaction bytes.
func (f queryFixture) settle(t *testing.T) (*types.MsgInitiateOutboundTransferResponse, []byte) {
	t.Helper()
	if _, err := f.msgs.SubmitAttestation(f.ctx.WithTxBytes([]byte("mint")), f.submission(t, 0xc1, "60", f.keys[1], f.keys[2])); err != nil {
		t.Fatalf("mint rejected: %v", err)
	}
	outboundTx := []byte("signed MsgInitiateOutboundTransfer transaction")
	response, err := f.msgs.InitiateOutboundTransfer(f.ctx.WithTxBytes(outboundTx).WithBlockHeight(queryHeight+1), &types.MsgInitiateOutboundTransfer{
		Sender: sdk.AccAddress(make([]byte, 20)).String(), RouteId: liveRouteID,
		Destination: "0x00000000000000000000000000000000000000c5", Amount: "25",
	})
	if err != nil {
		t.Fatalf("burn rejected: %v", err)
	}
	return response, outboundTx
}

func TestRouteOutstandingAndDailyUsageQueries(t *testing.T) {
	f := newQueryFixture(t)
	f.settle(t)

	route, err := f.server.Route(f.ctx, &types.QueryRouteRequest{RouteId: liveRouteID})
	if err != nil {
		t.Fatal(err)
	}
	want := types.Route{
		RouteId: liveRouteID, SourceChainId: "338", BridgeSigners: f.config.BridgeSigners, Guardian: f.config.Guardian,
		MaxTransferAmount: "100", DailyLimit: "150", MaxOutstandingAmount: "1000", Enabled: true, Paused: false,
		SignerSetVersion: 1, ControlNonce: 0, OutboundNonce: 1,
	}
	if !reflect.DeepEqual(*route.Route, want) || route.Height != queryHeight {
		t.Fatalf("route\n got %+v (height %d)\nwant %+v", *route.Route, route.Height, want)
	}

	outstanding, err := f.server.Outstanding(f.ctx, &types.QueryOutstandingRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if outstanding.RouteId != liveRouteID || outstanding.OutstandingAmount != "35" || outstanding.MaxOutstandingAmount != "1000" ||
		outstanding.RemainingAmount != "965" || outstanding.Height != queryHeight {
		t.Fatalf("unexpected outstanding response: %+v", outstanding)
	}

	usage, err := f.server.DailyUsage(f.ctx, &types.QueryDailyUsageRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if usage.RouteId != liveRouteID || usage.UtcDay != 20833 || usage.UtcDate != "2027-01-15" || usage.UsedAmount != "85" ||
		usage.DailyLimit != "150" || usage.RemainingAmount != "65" || usage.BlockTimeUnix != queryBlockUnix || usage.Height != queryHeight {
		t.Fatalf("unexpected daily usage response: %+v", usage)
	}

	// The next UTC day starts a new bucket.
	nextDay := f.ctx.WithBlockTime(time.Unix(queryBlockUnix+16*3600, 0).UTC())
	usage, err = f.server.DailyUsage(nextDay, &types.QueryDailyUsageRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if usage.UtcDay != 20834 || usage.UtcDate != "2027-01-16" || usage.UsedAmount != "0" || usage.RemainingAmount != "150" {
		t.Fatalf("unexpected next-day usage: %+v", usage)
	}

	// A context without block time cannot name the UTC day.
	_, err = f.server.DailyUsage(f.ctx.WithBlockTime(time.Time{}), &types.QueryDailyUsageRequest{})
	requireCode(t, err, codes.FailedPrecondition, "daily usage without block time")
}

func TestOutboundTransferQueryReturnsTheBurn(t *testing.T) {
	f := newQueryFixture(t)
	response, outboundTx := f.settle(t)

	for _, spelling := range []string{response.RequestId, "0x" + response.RequestId, strings.ToUpper(response.RequestId)} {
		transfer, err := f.server.OutboundTransfer(f.ctx, &types.QueryOutboundTransferRequest{RequestId: spelling})
		if err != nil {
			t.Fatal(err)
		}
		want := types.OutboundTransferRecord{
			RequestId: "0x" + response.RequestId, RouteId: liveRouteID, Sender: sdk.AccAddress(make([]byte, 20)).String(),
			Destination: common.HexToAddress("0x00000000000000000000000000000000000000c5").Hex(), Amount: "25", Nonce: 1,
			Height: queryHeight + 1, TxHash: txHashOf(outboundTx),
		}
		if !reflect.DeepEqual(*transfer.Transfer, want) || transfer.Height != queryHeight {
			t.Fatalf("transfer\n got %+v\nwant %+v", *transfer.Transfer, want)
		}
		if err := transfer.Transfer.Validate(); err != nil {
			t.Fatalf("stored transfer is not self-consistent: %v", err)
		}
	}
}

func TestParamsQuery(t *testing.T) {
	f := newQueryFixture(t)
	response, err := f.server.Params(f.ctx, &types.QueryParamsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	want := types.Params{
		Authority: testAuthority(), NativeDenom: "axtc", MaxSupply: "1000000", RequiredApprovals: 2, SignerSetSize: 3,
		ChainId: testChainID, AttestationDomain: "xitcoin-bridge-attestation-v2/" + testChainID,
		ControlDomain: "xitcoin-bridge-control-v2/" + testChainID, GuardianPauseDomain: "xitcoin-bridge-guardian-pause-v2/" + testChainID,
		SourceDepositDomain: "xitcoin-bridge-source-deposit-v1",
	}
	if !reflect.DeepEqual(*response.Params, want) {
		t.Fatalf("params\n got %+v\nwant %+v", *response.Params, want)
	}
}

func TestSignerSetVersionFollowsSignerRotation(t *testing.T) {
	ctx, k, current, keys, _ := controlServerSetup(t)
	// A route stored before versions were kept reads as version 1, and the
	// upgrade migration stores it once.
	if k.SignerSetVersion(ctx) != 1 || ctx.KVStore(k.storeKey).Has(types.KeySignerSetVersion) {
		t.Fatal("unversioned route must read as version 1")
	}
	if !k.StoreSignerSetVersion(ctx) || k.StoreSignerSetVersion(ctx) || k.SignerSetVersion(ctx) != 1 {
		t.Fatal("signer set version migration is not idempotent")
	}

	// Limits only, then the same signers in another order: no new version.
	next := current
	next.DailyLimit = "200"
	action, signatures := signedConfigActionWithNonce(t, current, next, keys, 1)
	if err := k.ApplyRouteConfigUpdate(ctx, current, action, next, signatures); err != nil {
		t.Fatal(err)
	}
	reordered := next
	reordered.BridgeSigners = []string{next.BridgeSigners[2], strings.ToLower(next.BridgeSigners[0]), next.BridgeSigners[1]}
	action, signatures = signedConfigActionWithNonce(t, next, reordered, keys, 2)
	if err := k.ApplyRouteConfigUpdate(ctx, next, action, reordered, signatures); err != nil {
		t.Fatal(err)
	}
	if k.SignerSetVersion(ctx) != 1 {
		t.Fatalf("version %d after updates that kept the signer set", k.SignerSetVersion(ctx))
	}

	// Replace one signer: version 2, and approvals are then recorded with it.
	replacement, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	rotated := reordered
	rotated.BridgeSigners = []string{reordered.BridgeSigners[0], reordered.BridgeSigners[1], crypto.PubkeyToAddress(replacement.PublicKey).Hex()}
	action, signatures = signedConfigActionWithNonce(t, reordered, rotated, keys, 3)
	if err := k.ApplyRouteConfigUpdate(ctx, reordered, action, rotated, signatures); err != nil {
		t.Fatal(err)
	}
	if k.SignerSetVersion(ctx) != 2 {
		t.Fatalf("version %d after a signer rotation, want 2", k.SignerSetVersion(ctx))
	}
	route, err := NewQueryServer(k).Route(ctx, &types.QueryRouteRequest{RouteId: rotated.RouteID})
	if err != nil || route.Route.SignerSetVersion != 2 || route.Route.ControlNonce != 3 {
		t.Fatalf("route after rotation: %+v, err %v", route, err)
	}

	attestation := testAttestation()
	attestation.DeadlineUnix = ctx.BlockTime().Unix() + 60
	attestation.Amount = "10"
	digest, err := types.SigningDigest(attestation, testChainID)
	if err != nil {
		t.Fatal(err)
	}
	var approvals [][]byte
	for _, key := range []*ecdsa.PrivateKey{replacement, keys[0]} {
		signature, err := crypto.Sign(digest.Bytes(), key)
		if err != nil {
			t.Fatal(err)
		}
		approvals = append(approvals, signature)
	}
	id, err := k.AdmitAttestation(ctx, rotated, attestation, approvals)
	if err != nil {
		t.Fatalf("attestation approved by the rotated set rejected: %v", err)
	}
	record, found, err := k.GetAttestationRecord(ctx, id)
	if err != nil || !found {
		t.Fatalf("record not stored: found=%v err=%v", found, err)
	}
	if record.SignerSetVersion != 2 || record.Approvers[0] != crypto.PubkeyToAddress(replacement.PublicKey).Hex() {
		t.Fatalf("record does not identify the rotated signer set: %+v", record)
	}
}

func TestInitializeRouteConfigStoresSignerSetVersion(t *testing.T) {
	key := storetypes.NewKVStoreKey(types.StoreKey)
	ctx := testContext(key, storetypes.NewTransientStoreKey("bridge_initialize_version_test"))
	k, err := NewSettlementKeeper(key, &fakeBankKeeper{supply: sdkmath.ZeroInt()}, "axtc", "1000", testAuthority())
	if err != nil {
		t.Fatal(err)
	}
	if k.SignerSetVersion(ctx) != 0 {
		t.Fatal("version without a route must be 0")
	}
	if _, err := NewMsgServer(k).InitializeRouteConfig(ctx, &types.MsgInitializeRouteConfig{
		Authority: testAuthority(), RouteId: liveRouteID,
		BridgeSigners: []string{"0x0000000000000000000000000000000000000001", "0x0000000000000000000000000000000000000002", "0x0000000000000000000000000000000000000003"},
		Guardian:      "0x0000000000000000000000000000000000000004", MaxTransferAmount: "10", DailyLimit: "100", MaxOutstandingAmount: "1000",
	}); err != nil {
		t.Fatal(err)
	}
	if !ctx.KVStore(key).Has(types.KeySignerSetVersion) || k.SignerSetVersion(ctx) != 1 {
		t.Fatal("initialized route has no stored signer set version 1")
	}
	if k.StoreSignerSetVersion(ctx) {
		t.Fatal("migration rewrote a stored version")
	}
}

func TestGenesisRoundTripKeepsRecordsAndDailyUsage(t *testing.T) {
	f := newQueryFixture(t)
	outbound, _ := f.settle(t)

	exported := f.keeper.ExportGenesis(f.ctx)
	if err := exported.Validate(); err != nil {
		t.Fatalf("exported genesis is invalid: %v", err)
	}
	if exported.DailyUsage == nil || *exported.DailyUsage != (types.DailyUsage{UTCDay: 20833, Amount: "85"}) {
		t.Fatalf("exported daily usage = %+v", exported.DailyUsage)
	}
	if exported.SignerSetVersion != 1 || len(exported.AttestationRecords) != 1 || len(exported.OutboundTransfers) != 1 || exported.OutboundNonce != 1 {
		t.Fatalf("unexpected exported genesis: %+v", exported)
	}

	// Through JSON, as the module exports and imports it.
	encoded, err := json.Marshal(exported)
	if err != nil {
		t.Fatal(err)
	}
	var imported types.GenesisState
	if err := json.Unmarshal(encoded, &imported); err != nil {
		t.Fatal(err)
	}
	if err := imported.Validate(); err != nil {
		t.Fatalf("decoded genesis is invalid: %v", err)
	}

	// Imported later on the same UTC day (the export context has no block
	// time), the daily limit keeps counting what was already used.
	key := storetypes.NewKVStoreKey(types.StoreKey)
	ctx := testContext(key, storetypes.NewTransientStoreKey("bridge_query_import_test")).
		WithBlockTime(time.Unix(queryBlockUnix+3600, 0).UTC()).WithBlockHeight(1)
	k := NewKeeper(key)
	k.InitGenesis(ctx, imported)
	reexported, err := json.Marshal(k.ExportGenesis(ctx))
	if err != nil {
		t.Fatal(err)
	}
	if string(reexported) != string(encoded) {
		t.Fatalf("genesis round trip changed the state:\n%s\n%s", encoded, reexported)
	}
	if err := k.CheckAndRecordLimits(ctx, f.config, "70"); err != ErrDailyLimitExceeded {
		t.Fatalf("daily usage was reset by the import: %v", err)
	}
	if err := k.CheckAndRecordLimits(ctx, f.config, "65"); err != nil {
		t.Fatalf("remaining daily capacity rejected: %v", err)
	}

	server := NewQueryServer(k)
	attestation, err := server.Attestation(ctx, &types.QueryAttestationRequest{AttestationId: exported.AttestationRecords[0].AttestationId})
	if err != nil || attestation.Status != types.AttestationState_ATTESTATION_STATE_PROCESSED || !reflect.DeepEqual(*attestation.Record, exported.AttestationRecords[0]) {
		t.Fatalf("imported attestation: %+v, err %v", attestation, err)
	}
	transfer, err := server.OutboundTransfer(ctx, &types.QueryOutboundTransferRequest{RequestId: outbound.RequestId})
	if err != nil || !reflect.DeepEqual(*transfer.Transfer, exported.OutboundTransfers[0]) {
		t.Fatalf("imported transfer: %+v, err %v", transfer, err)
	}

	// On a later UTC day the imported bucket is never read.
	later := ctx.WithBlockTime(time.Unix(queryBlockUnix+86400, 0).UTC())
	if err := k.CheckAndRecordLimits(later, f.config, "100"); err != nil {
		t.Fatalf("next-day transfer rejected: %v", err)
	}
}

func TestExportGenesisSkipsEmptyDailyUsage(t *testing.T) {
	f := newQueryFixture(t)
	if exported := f.keeper.ExportGenesis(f.ctx); exported.DailyUsage != nil || exported.SignerSetVersion != 1 {
		t.Fatalf("unexpected export without transfers: %+v", exported)
	}
}
