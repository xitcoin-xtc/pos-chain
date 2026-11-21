package evmd

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	protov2 "google.golang.org/protobuf/proto"

	"cosmossdk.io/log/v2"
	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	dbm "github.com/cosmos/cosmos-db"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client/flags"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	sdk "github.com/cosmos/cosmos-sdk/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types/v1"

	"github.com/xitcoin-xtc/pos-chain/evmd/config"
	srvflags "github.com/xitcoin-xtc/pos-chain/server/flags"
)

func TestTestnetAdministrativeAuthority(t *testing.T) {
	authority := mustTestnetAdministrativeAuthority()
	require.Equal(t, "xtc1vza8zsgvrfwmve084ytd8xqdkkm7u9e5csctc2", authority)
	require.Equal(t, TestnetAdministrativeMultisigAddress, authority)
	require.Equal(t, authority, mustAdministrativeAuthority(config.TestnetCosmosChainID))

	address, err := sdk.AccAddressFromBech32(authority)
	require.NoError(t, err)
	require.Len(t, address, 20)
	require.NotEqual(t, mustGovernanceAuthority(), authority)
}

func newNetworkApp(chainID string, evmChainID uint64, extra map[string]interface{}) *EVMD {
	appOptions := make(simtestutil.AppOptionsMap)
	appOptions[flags.FlagHome] = defaultNodeHome
	appOptions[srvflags.EVMChainID] = evmChainID
	for key, value := range extra {
		appOptions[key] = value
	}
	return NewExampleApp(log.NewNopLogger(), dbm.NewMemDB(), true, appOptions, baseapp.SetChainID(chainID))
}

func TestTestnetAppUsesTestnetAuthorities(t *testing.T) {
	app := newNetworkApp(config.TestnetCosmosChainID, config.TestnetEVMChainID, nil)
	require.Equal(t, TestnetAdministrativeMultisigAddress, app.BridgeKeeper.Authority())
	upgradeAuthority, err := app.UpgradeKeeper.Authority(nil, nil)
	require.NoError(t, err)
	require.Equal(t, TestnetAdministrativeMultisigAddress, upgradeAuthority.Address)
}

func TestPublicNetworksRefuseWrongIdentity(t *testing.T) {
	// A testnet node configured with another EVM chain ID would disagree
	// with the other nodes on transaction validity.
	require.PanicsWithError(t, "Xitcoin Testnet (xitcoin-testnet-v2-1) requires evm.evm-chain-id = 101089 in app.toml, got 101088", func() {
		newNetworkApp(config.TestnetCosmosChainID, config.MainnetEVMChainID, nil)
	})
	// Mainnet authorities are not decided in this release.
	require.Panics(t, func() {
		newNetworkApp(config.MainnetCosmosChainID, config.MainnetEVMChainID, nil)
	})
}

func TestMainnetGenesisRefusesTestnetMultisig(t *testing.T) {
	genesis := GenesisState{
		"validator_incentives": json.RawMessage(`{"authority":"` + TestnetAdministrativeMultisigAddress + `"}`),
	}
	require.NoError(t, validateNetworkAuthorities(config.TestnetCosmosChainID, genesis))
	require.ErrorContains(t, validateNetworkAuthorities(config.MainnetCosmosChainID, genesis), "must not use the testnet administrative multisig")

	genesis["validator_incentives"] = json.RawMessage(`{"authority":"xtc1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqnrql8a"}`)
	require.NoError(t, validateNetworkAuthorities(config.MainnetCosmosChainID, genesis))
}

type proposalTx struct{ msgs []sdk.Msg }

func (tx proposalTx) GetMsgs() []sdk.Msg { return tx.msgs }

func (tx proposalTx) GetMsgsV2() ([]protov2.Message, error) { return nil, nil }

// The former node-local option no longer changes consensus behaviour: a node
// that still sets it in app.toml rejects proposals like every other node.
func TestGovernanceProposalSubmissionIsNotANodeOption(t *testing.T) {
	require.False(t, governanceProposalSubmissionEnabled())
	app := newNetworkApp("xitcoin-governance-option-test", config.TestnetEVMChainID, map[string]interface{}{
		"unsafe-enable-governance-proposal-submission": true,
	})
	ctx := app.NewUncachedContext(false, cmtproto.Header{ChainID: "xitcoin-governance-option-test"})
	proposal := &govtypes.MsgSubmitProposal{
		Proposer: sdk.AccAddress(make([]byte, 20)).String(),
		Title:    "Network proposal",
		Summary:  "Must not reach the governance message server.",
	}
	_, err := app.AnteHandler()(ctx, proposalTx{msgs: []sdk.Msg{proposal}}, false)
	require.ErrorContains(t, err, "executable on-chain proposals are disabled")
}
