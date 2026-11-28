package integration

import (
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"

	"github.com/xitcoin-xtc/pos-chain/testutil/integration/base/factory"
	evmfactory "github.com/xitcoin-xtc/pos-chain/testutil/integration/evm/factory"
	"github.com/xitcoin-xtc/pos-chain/testutil/integration/evm/grpc"
	"github.com/xitcoin-xtc/pos-chain/testutil/integration/evm/network"
	testkeyring "github.com/xitcoin-xtc/pos-chain/testutil/keyring"
	bridgetypes "github.com/xitcoin-xtc/pos-chain/x/bridge/types"

	errorsmod "cosmossdk.io/errors"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

// TestBridgeFailuresReportTypedCodes checks the code and codespace that a
// client reads from the result of a failed bridge transaction. Before
// xitcoin-security-hardening-v2 every one of them was code 1, codespace
// "undefined".
func TestBridgeFailuresReportTypedCodes(t *testing.T) {
	keys := []string{}
	for i := byte(1); i <= 3; i++ {
		keys = append(keys, crypto.PubkeyToAddress(bridgeSignerKey(t, i).PublicKey).Hex())
	}
	route := bridgetypes.RouteConfig{
		RouteID: liveBridgeRouteID, BridgeSigners: keys,
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
	holder := keyring.GetAccAddr(0)
	gas := uint64(400_000)

	deliver := func(msg sdk.Msg) (uint32, string, string) {
		t.Helper()
		tx, err := txFactory.BuildCosmosTx(keyring.GetPrivKey(0), factory.CosmosTxArgs{Msgs: []sdk.Msg{msg}, Gas: &gas})
		require.NoError(t, err)
		bz, err := txFactory.EncodeTx(tx)
		require.NoError(t, err)
		block, err := nw.NextBlockWithTxs(bz)
		require.NoError(t, err)
		require.Len(t, block.TxResults, 1)
		return block.TxResults[0].Code, block.TxResults[0].Codespace, block.TxResults[0].Log
	}
	requireCode := func(want *errorsmod.Error, code uint32, codespace, log string) {
		t.Helper()
		require.Equal(t, "bridge", codespace, log)
		require.Equal(t, want.ABCICode(), code, log)
	}

	// Rejected by ValidateBasic, before the message runs.
	invalid := &bridgetypes.MsgInitiateOutboundTransfer{
		Sender: holder.String(), RouteId: "Not-A-Route", Destination: "0x00000000000000000000000000000000000000c5", Amount: "1",
	}
	code, codespace, log := deliver(invalid)
	requireCode(bridgetypes.ErrInvalidRouteID, code, codespace, log)

	// An outbound burn above the outstanding amount.
	burn := &bridgetypes.MsgInitiateOutboundTransfer{
		Sender: holder.String(), RouteId: liveBridgeRouteID, Destination: "0x00000000000000000000000000000000000000c5", Amount: "1",
	}
	code, codespace, log = deliver(burn)
	requireCode(bridgetypes.ErrInsufficientOutstanding, code, codespace, log)

	// An outbound burn above the per-transfer limit.
	burn.Amount = "1000000000000000000001"
	code, codespace, log = deliver(burn)
	requireCode(bridgetypes.ErrTransferLimitExceeded, code, codespace, log)

	// An attestation approved by one signer only.
	msg := &bridgetypes.MsgSubmitAttestation{
		Submitter: holder.String(), RouteId: liveBridgeRouteID, Direction: string(bridgetypes.DirectionCronosToXitcoin),
		SourceChainId: "338", SourceRef: "0x" + strings.Repeat("e2", 32), Nonce: 1, Destination: holder.String(),
		Amount: "1000", DeadlineUnix: nw.GetContext().BlockTime().Unix() + 3600,
	}
	digest, err := bridgetypes.SigningDigest(msg.Attestation(), nw.GetChainID())
	require.NoError(t, err)
	sign := func(i byte) []byte {
		signature, err := crypto.Sign(digest.Bytes(), bridgeSignerKey(t, i))
		require.NoError(t, err)
		return signature
	}
	msg.Signatures = [][]byte{sign(1), sign(1)}
	code, codespace, log = deliver(msg)
	requireCode(bridgetypes.ErrInvalidApprovals, code, codespace, log)

	// Processed once, then replayed.
	msg.Signatures = [][]byte{sign(1), sign(2)}
	code, _, log = deliver(msg)
	require.Zero(t, code, log)
	code, codespace, log = deliver(msg)
	requireCode(bridgetypes.ErrAttestationAlreadyProcessed, code, codespace, log)

	// A source chain other than the route's.
	other := *msg
	other.SourceChainId = "25"
	other.SourceRef = "0x" + strings.Repeat("e3", 32)
	code, codespace, log = deliver(&other)
	requireCode(bridgetypes.ErrAttestationSourceChainMismatch, code, codespace, log)

	// The authority check of the SDK keeps its own code.
	init := &bridgetypes.MsgInitializeRouteConfig{
		Authority: holder.String(), RouteId: "second", BridgeSigners: keys, Guardian: route.Guardian,
		MaxTransferAmount: route.MaxTransferAmount, DailyLimit: route.DailyLimit, MaxOutstandingAmount: route.MaxOutstandingAmount,
	}
	code, codespace, log = deliver(init)
	require.Equal(t, sdkerrors.ErrUnauthorized.Codespace(), codespace, log)
	require.Equal(t, sdkerrors.ErrUnauthorized.ABCICode(), code, log)
}
