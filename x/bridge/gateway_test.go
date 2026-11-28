package bridge_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	abci "github.com/cometbft/cometbft/abci/types"
	cmtbytes "github.com/cometbft/cometbft/libs/bytes"
	rpcclient "github.com/cometbft/cometbft/rpc/client"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"

	"github.com/xitcoin-xtc/pos-chain/x/bridge"
	"github.com/xitcoin-xtc/pos-chain/x/bridge/keeper"
	"github.com/xitcoin-xtc/pos-chain/x/bridge/types"

	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/log/v2"
	sdkmath "cosmossdk.io/math"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/server/api"
	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	sdktestutil "github.com/cosmos/cosmos-sdk/testutil"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

const liveRouteID = "0x21121c16b53a726056a6683f00c7eb4da5501ce8a2abc8a4677e06f1e94b5cd9"

// routerNode answers the ABCI queries of a client.Context from the module's
// gRPC query router, as a node does, so the module's own gateway
// registration is exercised through the client and query router.
type routerNode struct {
	client.CometRPC
	router *baseapp.GRPCQueryRouter
	ctx    sdk.Context
}

func (n routerNode) ABCIQueryWithOptions(_ context.Context, path string, data cmtbytes.HexBytes, _ rpcclient.ABCIQueryOptions) (*coretypes.ResultABCIQuery, error) {
	handler := n.router.Route(path)
	if handler == nil {
		return &coretypes.ResultABCIQuery{Response: *sdkerrors.QueryResult(sdkerrors.ErrUnknownRequest, false)}, nil
	}
	response, err := handler(n.ctx, &abci.RequestQuery{Path: path, Data: data, Height: n.ctx.BlockHeight()})
	if err != nil {
		code := sdkerrors.ErrUnknownRequest
		switch status.Code(err) {
		case codes.NotFound:
			code = sdkerrors.ErrKeyNotFound
		case codes.InvalidArgument:
			code = sdkerrors.ErrInvalidRequest
		}
		return &coretypes.ResultABCIQuery{Response: *sdkerrors.QueryResult(errorsmod.Wrap(code, err.Error()), false)}, nil
	}
	response.Height = n.ctx.BlockHeight()
	return &coretypes.ResultABCIQuery{Response: *response}, nil
}

type gatewayFixture struct {
	handler  http.Handler
	record   types.AttestationRecord
	transfer types.OutboundTransferRecord
	legacy   string
}

func newGatewayFixture(t *testing.T) gatewayFixture {
	t.Helper()
	key := storetypes.NewKVStoreKey(types.StoreKey)
	ctx := sdktestutil.DefaultContext(key, storetypes.NewTransientStoreKey("bridge_gateway_test")).
		WithChainID("xitcoin-testnet-v2-1").WithBlockHeight(7).WithBlockTime(time.Unix(1800000000, 0).UTC())
	k := keeper.NewKeeper(key)

	attestation := types.Attestation{
		RouteID: liveRouteID, Direction: types.DirectionCronosToXitcoin, SourceChainID: "338",
		SourceRef: strings.Repeat("ab", 32), Nonce: 1, Destination: sdk.AccAddress(bytes.Repeat([]byte{1}, 20)).String(),
		Amount: "60", DeadlineUnix: 1800003600,
	}
	approvers := []common.Address{common.HexToAddress("0x00000000000000000000000000000000000000A1"), common.HexToAddress("0x00000000000000000000000000000000000000A2")}
	record, err := types.NewAttestationRecord(attestation, "xitcoin-testnet-v2-1", 1, approvers, 5, types.TxHash([]byte("mint")))
	if err != nil {
		t.Fatal(err)
	}
	sender := sdk.AccAddress(bytes.Repeat([]byte{2}, 20)).String()
	destination := common.HexToAddress("0x00000000000000000000000000000000000000c5")
	requestID := types.OutboundRequestID(liveRouteID, sender, destination, sdkmath.NewInt(25), 1)
	transfer := types.OutboundTransferRecord{
		RequestId: types.HexID(requestID), RouteId: liveRouteID, Sender: sender, Destination: destination.Hex(),
		Amount: "25", Nonce: 1, Height: 6, TxHash: types.TxHash([]byte("burn")),
	}
	legacy, err := types.SourceDepositKey("338", strings.Repeat("cd", 32))
	if err != nil {
		t.Fatal(err)
	}
	genesis := types.GenesisState{
		RouteConfig: &types.RouteConfig{
			RouteID:       liveRouteID,
			BridgeSigners: []string{"0x00000000000000000000000000000000000000A1", "0x00000000000000000000000000000000000000a2", "0x00000000000000000000000000000000000000A3"},
			Guardian:      "0x00000000000000000000000000000000000000B4", MaxTransferAmount: "100", DailyLimit: "150",
			MaxOutstandingAmount: "1000", Enabled: true, SourceChainID: "338",
		},
		OutstandingAmount:       "35",
		OutboundNonce:           1,
		ProcessedSourceDeposits: []string{record.AttestationId, types.HexID(legacy)},
		DailyUsage:              &types.DailyUsage{UTCDay: 20833, Amount: "85"},
		AttestationRecords:      []types.AttestationRecord{record},
		OutboundTransfers:       []types.OutboundTransferRecord{transfer},
	}
	if err := genesis.Validate(); err != nil {
		t.Fatal(err)
	}
	k.InitGenesis(ctx, genesis)

	registry := codectypes.NewInterfaceRegistry()
	router := baseapp.NewGRPCQueryRouter()
	router.SetInterfaceRegistry(registry)
	types.RegisterQueryServer(router, keeper.NewQueryServer(k))
	clientCtx := client.Context{}.
		WithClient(routerNode{router: router, ctx: ctx}).
		WithCodec(codec.NewProtoCodec(registry)).
		WithInterfaceRegistry(registry)

	// The API server's gateway mux: the marshaler and error handler of a node.
	server := api.New(clientCtx, log.NewNopLogger(), nil)
	bridge.AppModuleBasic{}.RegisterGRPCGatewayRoutes(clientCtx, server.GRPCGatewayRouter)
	return gatewayFixture{handler: server.GRPCGatewayRouter, record: record, transfer: transfer, legacy: types.HexID(legacy)}
}

func (f gatewayFixture) get(t *testing.T, path string) (int, map[string]any) {
	t.Helper()
	recorder := httptest.NewRecorder()
	f.handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("GET %s: invalid JSON %q: %v", path, recorder.Body.String(), err)
	}
	return recorder.Code, body
}

func (f gatewayFixture) getOK(t *testing.T, path string) map[string]any {
	t.Helper()
	code, body := f.get(t, path)
	if code != http.StatusOK {
		t.Fatalf("GET %s: status %d, body %v", path, code, body)
	}
	return body
}

func field(t *testing.T, body map[string]any, path ...string) any {
	t.Helper()
	var value any = body
	for _, name := range path {
		object, ok := value.(map[string]any)
		if !ok {
			t.Fatalf("%v: %s is not an object in %v", path, name, body)
		}
		if value, ok = object[name]; !ok {
			t.Fatalf("%v: missing %s in %v", path, name, body)
		}
	}
	return value
}

func expect(t *testing.T, body map[string]any, want any, path ...string) {
	t.Helper()
	if got := field(t, body, path...); got != want {
		t.Fatalf("%v = %#v, want %#v", path, got, want)
	}
}

func TestGatewayRoutesServeEveryBridgeQuery(t *testing.T) {
	f := newGatewayFixture(t)

	params := f.getOK(t, "/xitcoin/bridge/v1/params")
	expect(t, params, "xitcoin-bridge-attestation-v2/xitcoin-testnet-v2-1", "params", "attestation_domain")
	expect(t, params, float64(2), "params", "required_approvals")

	route := f.getOK(t, "/xitcoin/bridge/v1/routes/"+liveRouteID)
	expect(t, route, liveRouteID, "route", "route_id")
	expect(t, route, "338", "route", "source_chain_id")
	expect(t, route, false, "route", "paused")
	expect(t, route, "1", "route", "signer_set_version")
	expect(t, route, "7", "height")

	outstanding := f.getOK(t, "/xitcoin/bridge/v1/outstanding")
	expect(t, outstanding, "35", "outstanding_amount")
	expect(t, outstanding, "965", "remaining_amount")

	usage := f.getOK(t, "/xitcoin/bridge/v1/daily_usage")
	expect(t, usage, "20833", "utc_day")
	expect(t, usage, "2027-01-15", "utc_date")
	expect(t, usage, "85", "used_amount")
	expect(t, usage, "65", "remaining_amount")

	attestation := f.getOK(t, "/xitcoin/bridge/v1/attestations/"+f.record.AttestationId)
	expect(t, attestation, "ATTESTATION_STATE_PROCESSED", "status")
	expect(t, attestation, true, "processed")
	expect(t, attestation, f.record.TxHash, "record", "tx_hash")
	expect(t, attestation, "5", "record", "height")
	expect(t, attestation, "60", "record", "amount")
	expect(t, attestation, f.record.PayloadHash, "record", "payload_hash")
	expect(t, attestation, "1", "record", "signer_set_version")

	bySource := f.getOK(t, "/xitcoin/bridge/v1/source_deposits/338/0x"+strings.Repeat("AB", 32))
	expect(t, bySource, f.record.AttestationId, "attestation_id")
	expect(t, bySource, "ATTESTATION_STATE_PROCESSED", "status")

	legacy := f.getOK(t, "/xitcoin/bridge/v1/attestations/"+f.legacy)
	expect(t, legacy, "ATTESTATION_STATE_PROCESSED_WITHOUT_RECORD", "status")
	expect(t, legacy, true, "processed")
	expect(t, legacy, nil, "record")

	// Default values are emitted, never omitted: an absent attestation reads
	// processed=false explicitly.
	absent := f.getOK(t, "/xitcoin/bridge/v1/attestations/0x"+strings.Repeat("ef", 32))
	expect(t, absent, "ATTESTATION_STATE_ABSENT", "status")
	expect(t, absent, false, "processed")
	expect(t, absent, nil, "record")

	transfer := f.getOK(t, "/xitcoin/bridge/v1/outbound_transfers/"+f.transfer.RequestId)
	expect(t, transfer, "25", "transfer", "amount")
	expect(t, transfer, f.transfer.Destination, "transfer", "destination")
	expect(t, transfer, "6", "transfer", "height")
	expect(t, transfer, f.transfer.TxHash, "transfer", "tx_hash")

	// The pinned two-field query keeps its path and shape.
	status := f.getOK(t, "/cosmos/evm/bridge/v1/attestations/"+f.record.AttestationId)
	if len(status) != 2 || status["processed"] != true || status["attestation_id"] != f.record.AttestationId {
		t.Fatalf("legacy attestation status changed shape: %v", status)
	}
}

func TestGatewayRoutesReportErrors(t *testing.T) {
	f := newGatewayFixture(t)
	for path, want := range map[string]int{
		"/xitcoin/bridge/v1/routes/cronos-testnet-xitcoin-testnet":            http.StatusNotFound,
		"/xitcoin/bridge/v1/outbound_transfers/0x" + strings.Repeat("ef", 32): http.StatusNotFound,
		"/xitcoin/bridge/v1/attestations/0x01":                                http.StatusBadRequest,
		"/xitcoin/bridge/v1/source_deposits/0338/" + strings.Repeat("ab", 32): http.StatusBadRequest,
		"/xitcoin/bridge/v1/unknown":                                          http.StatusNotImplemented,
	} {
		if code, body := f.get(t, path); code != want {
			t.Fatalf("GET %s: status %d, want %d (%v)", path, code, want, body)
		}
	}
}
