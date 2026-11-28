package integration

import (
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"

	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/cometbft/cometbft/crypto/merkle"
	cmtbytes "github.com/cometbft/cometbft/libs/bytes"
	rpcclient "github.com/cometbft/cometbft/rpc/client"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"

	"github.com/xitcoin-xtc/pos-chain/evmd"
	"github.com/xitcoin-xtc/pos-chain/testutil/integration/base/factory"
	evmfactory "github.com/xitcoin-xtc/pos-chain/testutil/integration/evm/factory"
	"github.com/xitcoin-xtc/pos-chain/testutil/integration/evm/grpc"
	"github.com/xitcoin-xtc/pos-chain/testutil/integration/evm/network"
	testkeyring "github.com/xitcoin-xtc/pos-chain/testutil/keyring"
	bridgekeeper "github.com/xitcoin-xtc/pos-chain/x/bridge/keeper"
	bridgetypes "github.com/xitcoin-xtc/pos-chain/x/bridge/types"

	"cosmossdk.io/log/v2"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/server/api"
	serverconfig "github.com/cosmos/cosmos-sdk/server/config"
	"github.com/cosmos/cosmos-sdk/store/v2/rootmulti"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// liveBridgeRouteID is the route ID stored on xitcoin-testnet-v2-1: the
// Cronos bytes32 route identifier.
const liveBridgeRouteID = "0x21121c16b53a726056a6683f00c7eb4da5501ce8a2abc8a4677e06f1e94b5cd9"

// appNode answers client ABCI queries with the application's own ABCI Query,
// as the node behind the REST API does.
type appNode struct {
	client.CometRPC
	app *baseapp.BaseApp
}

func (n appNode) ABCIQueryWithOptions(ctx context.Context, path string, data cmtbytes.HexBytes, opts rpcclient.ABCIQueryOptions) (*coretypes.ResultABCIQuery, error) {
	response, err := n.app.Query(ctx, &abci.RequestQuery{Path: path, Data: data, Height: opts.Height, Prove: opts.Prove})
	if err != nil {
		return nil, err
	}
	return &coretypes.ResultABCIQuery{Response: *response}, nil
}

func bridgeSignerKey(t *testing.T, index byte) *ecdsa.PrivateKey {
	t.Helper()
	seed := sha256.Sum256([]byte{'x', 'b', index})
	key, err := crypto.ToECDSA(seed[:])
	require.NoError(t, err)
	return key
}

func abciQuery[T interface{ Unmarshal([]byte) error }](t *testing.T, app *baseapp.BaseApp, path string, request interface{ Marshal() ([]byte, error) }, response T) T {
	t.Helper()
	data, err := request.Marshal()
	require.NoError(t, err)
	result, err := app.Query(context.Background(), &abci.RequestQuery{Path: path, Data: data})
	require.NoError(t, err)
	require.Zero(t, result.Code, result.Log)
	require.NoError(t, response.Unmarshal(result.Value))
	return response
}

// TestBridgeQueriesReadBackProcessedTransfers processes an inbound
// attestation and an outbound burn in real blocks and reads them back through
// the gRPC queries (ABCI), the REST gateway of the application and a store
// proof against the committed app hash.
func TestBridgeQueriesReadBackProcessedTransfers(t *testing.T) {
	keys := []*ecdsa.PrivateKey{bridgeSignerKey(t, 1), bridgeSignerKey(t, 2), bridgeSignerKey(t, 3)}
	signers := make([]string, len(keys))
	for i, key := range keys {
		signers[i] = crypto.PubkeyToAddress(key.PublicKey).Hex()
	}
	route := bridgetypes.RouteConfig{
		RouteID: liveBridgeRouteID, BridgeSigners: signers,
		Guardian:          common.HexToAddress("0x00000000000000000000000000000000000000B4").Hex(),
		MaxTransferAmount: "1000000000000000000000", DailyLimit: "5000000000000000000000",
		MaxOutstandingAmount: "100000000000000000000000", Enabled: true, SourceChainID: "338",
	}
	keyring := testkeyring.New(1)
	nw := network.NewUnitTestNetwork(
		CreateEvmd,
		network.WithPreFundedAccounts(keyring.GetAllAccAddrs()...),
		network.WithCustomGenesis(network.CustomGenesisState{bridgetypes.ModuleName: bridgetypes.GenesisState{RouteConfig: &route}}),
	)
	txFactory := evmfactory.New(nw, grpc.NewIntegrationHandler(nw))
	app := nw.App.GetBaseApp()
	holder := keyring.GetAccAddr(0)

	// Inbound: a Cronos deposit approved by two of the three signers.
	msg := &bridgetypes.MsgSubmitAttestation{
		Submitter: holder.String(), RouteId: liveBridgeRouteID, Direction: string(bridgetypes.DirectionCronosToXitcoin),
		SourceChainId: "338", SourceRef: "0x" + strings.Repeat("d1", 32), Nonce: 1, Destination: holder.String(),
		Amount: "700000000000000000000", DeadlineUnix: nw.GetContext().BlockTime().Unix() + 3600,
	}
	digest, err := bridgetypes.SigningDigest(msg.Attestation(), nw.GetChainID())
	require.NoError(t, err)
	for _, key := range keys[1:] {
		signature, err := crypto.Sign(digest.Bytes(), key)
		require.NoError(t, err)
		msg.Signatures = append(msg.Signatures, signature)
	}
	id, err := msg.Attestation().ID()
	require.NoError(t, err)
	idText := bridgetypes.HexID(id)

	absent := abciQuery(t, app, "/cosmos.evm.bridge.v1.Query/Attestation", &bridgetypes.QueryAttestationRequest{AttestationId: idText}, &bridgetypes.QueryAttestationResponse{})
	require.Equal(t, bridgetypes.AttestationState_ATTESTATION_STATE_ABSENT, absent.Status)
	require.False(t, absent.Processed)

	tx, err := txFactory.BuildCosmosTx(keyring.GetPrivKey(0), factory.CosmosTxArgs{Msgs: []sdk.Msg{msg}})
	require.NoError(t, err)
	inboundTx, err := txFactory.EncodeTx(tx)
	require.NoError(t, err)
	block, err := nw.NextBlockWithTxs(inboundTx)
	require.NoError(t, err)
	require.Len(t, block.TxResults, 1)
	require.Zero(t, block.TxResults[0].Code, block.TxResults[0].Log)
	inboundHeight := nw.GetContext().BlockHeight()
	inboundHash := sha256.Sum256(inboundTx)

	processed := abciQuery(t, app, "/cosmos.evm.bridge.v1.Query/Attestation", &bridgetypes.QueryAttestationRequest{AttestationId: idText}, &bridgetypes.QueryAttestationResponse{})
	require.Equal(t, bridgetypes.AttestationState_ATTESTATION_STATE_PROCESSED, processed.Status)
	require.True(t, processed.Processed)
	require.Equal(t, inboundHeight, processed.Height)
	record := processed.Record
	require.NotNil(t, record)
	payloadHash, err := msg.Attestation().PayloadHash(nw.GetChainID())
	require.NoError(t, err)
	require.Equal(t, bridgetypes.AttestationRecord{
		AttestationId: idText, RouteId: liveBridgeRouteID, Direction: "cronos_to_xitcoin", SourceChainId: "338",
		SourceRef: strings.Repeat("d1", 32), Nonce: 1, Recipient: holder.String(), Amount: msg.Amount,
		DeadlineUnix: msg.DeadlineUnix, SignerSetVersion: 1, Approvers: signers[1:],
		DestinationChainId: nw.GetChainID(), PayloadHash: bridgetypes.HexID(payloadHash), SigningDigest: digest.Hex(),
		Height: inboundHeight, TxHash: strings.ToUpper(hex.EncodeToString(inboundHash[:])),
	}, *record)

	bySource := abciQuery(t, app, "/cosmos.evm.bridge.v1.Query/SourceDeposit",
		&bridgetypes.QuerySourceDepositRequest{SourceChainId: "338", SourceRef: msg.SourceRef}, &bridgetypes.QueryAttestationResponse{})
	require.Equal(t, processed, bySource)

	// Outbound: the holder burns part of the minted XTC.
	burn := &bridgetypes.MsgInitiateOutboundTransfer{
		Sender: holder.String(), RouteId: liveBridgeRouteID, Destination: "0x00000000000000000000000000000000000000c5", Amount: "250000000000000000000",
	}
	tx, err = txFactory.BuildCosmosTx(keyring.GetPrivKey(0), factory.CosmosTxArgs{Msgs: []sdk.Msg{burn}})
	require.NoError(t, err)
	outboundTx, err := txFactory.EncodeTx(tx)
	require.NoError(t, err)
	block, err = nw.NextBlockWithTxs(outboundTx)
	require.NoError(t, err)
	require.Zero(t, block.TxResults[0].Code, block.TxResults[0].Log)
	outboundHeight := nw.GetContext().BlockHeight()
	outboundHash := sha256.Sum256(outboundTx)
	var burnResponse bridgetypes.MsgInitiateOutboundTransferResponse
	var txData sdk.TxMsgData
	require.NoError(t, txData.Unmarshal(block.TxResults[0].Data))
	require.Len(t, txData.MsgResponses, 1)
	require.NoError(t, burnResponse.Unmarshal(txData.MsgResponses[0].Value))

	transfer := abciQuery(t, app, "/cosmos.evm.bridge.v1.Query/OutboundTransfer",
		&bridgetypes.QueryOutboundTransferRequest{RequestId: burnResponse.RequestId}, &bridgetypes.QueryOutboundTransferResponse{})
	require.Equal(t, bridgetypes.OutboundTransferRecord{
		RequestId: "0x" + burnResponse.RequestId, RouteId: liveBridgeRouteID, Sender: holder.String(),
		Destination: common.HexToAddress(burn.Destination).Hex(), Amount: burn.Amount, Nonce: 1,
		Height: outboundHeight, TxHash: strings.ToUpper(hex.EncodeToString(outboundHash[:])),
	}, *transfer.Transfer)

	outstanding := abciQuery(t, app, "/cosmos.evm.bridge.v1.Query/Outstanding", &bridgetypes.QueryOutstandingRequest{}, &bridgetypes.QueryOutstandingResponse{})
	require.Equal(t, "450000000000000000000", outstanding.OutstandingAmount)
	usage := abciQuery(t, app, "/cosmos.evm.bridge.v1.Query/DailyUsage", &bridgetypes.QueryDailyUsageRequest{}, &bridgetypes.QueryDailyUsageResponse{})
	require.Equal(t, "950000000000000000000", usage.UsedAmount)
	require.Equal(t, nw.GetContext().BlockTime().UTC().Format("2006-01-02"), usage.UtcDate)
	require.Equal(t, bridgekeeper.UTCDay(nw.GetContext().BlockTime().Unix()), usage.UtcDay)

	// The REST gateway of the application serves the same record.
	appCodec := nw.App.AppCodec()
	clientCtx := client.Context{}.WithClient(appNode{app: app}).WithCodec(appCodec).WithInterfaceRegistry(nw.App.InterfaceRegistry())
	server := api.New(clientCtx, log.NewNopLogger(), nil)
	evmdApp, ok := nw.App.(*evmd.EVMD)
	require.True(t, ok)
	evmdApp.RegisterAPIRoutes(server, serverconfig.APIConfig{})
	for path, check := range map[string]func(map[string]any){
		"/xitcoin/bridge/v1/attestations/" + idText: func(body map[string]any) {
			require.Equal(t, "ATTESTATION_STATE_PROCESSED", body["status"])
			require.Equal(t, record.TxHash, body["record"].(map[string]any)["tx_hash"])
		},
		"/xitcoin/bridge/v1/routes/" + liveBridgeRouteID: func(body map[string]any) {
			require.Equal(t, liveBridgeRouteID, body["route"].(map[string]any)["route_id"])
		},
		"/xitcoin/bridge/v1/outbound_transfers/0x" + burnResponse.RequestId: func(body map[string]any) {
			require.Equal(t, transfer.Transfer.TxHash, body["transfer"].(map[string]any)["tx_hash"])
		},
		"/xitcoin/bridge/v1/params":      func(body map[string]any) { require.Contains(t, body, "params") },
		"/xitcoin/bridge/v1/outstanding": func(body map[string]any) { require.Equal(t, outstanding.OutstandingAmount, body["outstanding_amount"]) },
		"/xitcoin/bridge/v1/daily_usage": func(body map[string]any) { require.Equal(t, usage.UsedAmount, body["used_amount"]) },
		"/xitcoin/bridge/v1/source_deposits/338/" + msg.SourceRef: func(body map[string]any) {
			require.Equal(t, idText, body["attestation_id"])
		},
	} {
		recorder := httptest.NewRecorder()
		server.GRPCGatewayRouter.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusOK, recorder.Code, "GET %s: %s", path, recorder.Body.String())
		var body map[string]any
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
		check(body)
	}

	// Independent verification: the record is committed state. Its store
	// proof at the processing height verifies against that height's app hash.
	recordKey := bridgetypes.AttestationRecordKey(id)
	proof, err := app.Query(context.Background(), &abci.RequestQuery{Path: "/store/bridge/key", Data: recordKey, Height: outboundHeight, Prove: true})
	require.NoError(t, err)
	require.Zero(t, proof.Code, proof.Log)
	encoded, err := record.Marshal()
	require.NoError(t, err)
	require.Equal(t, encoded, proof.Value)
	keyPath := merkle.KeyPath{}.AppendKey([]byte(bridgetypes.StoreKey), merkle.KeyEncodingURL).AppendKey(recordKey, merkle.KeyEncodingURL)
	require.NoError(t, rootmulti.DefaultProofRuntime().VerifyValue(proof.ProofOps, app.LastCommitID().Hash, keyPath.String(), proof.Value))
}
