package validatoradmission

// Last lines of defence, tested directly on the real application: the
// admission staking hook (x/staking calls it for every validator creation,
// whatever the path), the admission governance hook (x/gov calls it for every
// proposal submission), and the absence of the modules that would let a
// foreign or multi-party account dispatch messages without these checks.

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	sdkmath "cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"
	govkeeper "github.com/cosmos/cosmos-sdk/x/gov/keeper"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

// The staking hook rejects a validator that the ante handler and the
// precompile wrappers never saw: the plain x/staking message server (the
// handler the message router, authz and x/gov dispatch to) and the hook
// itself, called directly.
func TestStakingHookIsTheLastLineOfDefence(t *testing.T) {
	c := newChain(t, false)
	server := stakingkeeper.NewMsgServerImpl(c.app.StakingKeeper)
	router := c.app.MsgServiceRouter()

	cases := []struct {
		name string
		msg  *stakingtypes.MsgCreateValidator
		want string
	}{
		{"unapproved operator", c.createValidatorMsg(c.attacker, "attacker", minimumSelfDelegation, minimumSelfDelegation), "not approved"},
		{"min_self_delegation below the minimum", c.createValidatorMsg(c.candidate, "candidate", minimumSelfDelegation, minimumSelfDelegation.SubRaw(1)), "min_self_delegation must be at least"},
		{"min_self_delegation of one", c.createValidatorMsg(c.candidate, "candidate", minimumSelfDelegation, sdkmath.OneInt()), "min_self_delegation must be at least"},
	}
	for _, tc := range cases {
		// The error makes the caller discard the message's writes: baseapp
		// for a transaction, authz for a MsgExec, x/gov for a proposal.
		t.Run(tc.name+" through the plain message server", func(t *testing.T) {
			ctx, _ := c.ctx().CacheContext()
			_, err := server.CreateValidator(ctx, tc.msg)
			require.ErrorContains(t, err, tc.want)
		})
		t.Run(tc.name+" through the message router", func(t *testing.T) {
			ctx, _ := c.ctx().CacheContext()
			handler := router.Handler(tc.msg)
			require.NotNil(t, handler)
			_, err := handler(ctx, tc.msg)
			require.ErrorContains(t, err, tc.want)
		})
	}

	t.Run("hook called directly", func(t *testing.T) {
		ctx, _ := c.ctx().CacheContext()
		operator := sdk.ValAddress(c.attacker.acc)
		validator, err := stakingtypes.NewValidator(operator.String(), consensusKey("direct").PubKey(), stakingtypes.Description{Moniker: "direct"})
		require.NoError(t, err)
		validator.MinSelfDelegation = minimumSelfDelegation
		require.NoError(t, c.app.StakingKeeper.SetValidator(ctx, validator))
		require.ErrorContains(t, c.app.StakingKeeper.Hooks().AfterValidatorCreated(ctx, operator), "not approved")

		approved := sdk.ValAddress(c.candidate.acc)
		validator, err = stakingtypes.NewValidator(approved.String(), consensusKey("direct-approved").PubKey(), stakingtypes.Description{Moniker: "direct"})
		require.NoError(t, err)
		validator.MinSelfDelegation = minimumSelfDelegation.SubRaw(1)
		require.NoError(t, c.app.StakingKeeper.SetValidator(ctx, validator))
		require.ErrorContains(t, c.app.StakingKeeper.Hooks().AfterValidatorCreated(ctx, approved), "min_self_delegation")
		validator.MinSelfDelegation = minimumSelfDelegation
		require.NoError(t, c.app.StakingKeeper.SetValidator(ctx, validator))
		require.NoError(t, c.app.StakingKeeper.Hooks().AfterValidatorCreated(ctx, approved))
	})

	t.Run("control: approved operator at the minimum", func(t *testing.T) {
		ctx, _ := c.ctx().CacheContext()
		_, err := server.CreateValidator(ctx, c.createValidatorMsg(c.candidate, "candidate", minimumSelfDelegation, minimumSelfDelegation))
		require.NoError(t, err)
	})
}

// The governance hook rejects a proposal that the ante handler never saw:
// the plain x/gov message server, which the governance precompile and x/gov
// itself call.
func TestGovernanceHookIsTheLastLineOfDefence(t *testing.T) {
	for _, governance := range []bool{false, true} {
		c := newChain(t, governance)
		server := govkeeper.NewMsgServerImpl(&c.app.GovKeeper)
		gov := c.govAddress()
		deposit := sdk.NewCoins(sdk.NewCoin(c.app.GetEVMKeeper().GetParams(c.ctx()).EvmDenom, c.minDeposit()))
		proposer := c.genesisOperators[0].acc.String()
		mint := &minttypes.MsgUpdateParams{Authority: gov.String(), Params: c.mintParams()}

		restricted, err := govv1.NewMsgSubmitProposal([]sdk.Msg{mint}, deposit, proposer, "", "attack", "attack", false)
		require.NoError(t, err)
		ctx, _ := c.ctx().CacheContext()
		_, err = server.SubmitProposal(ctx, restricted)
		require.Error(t, err, "governance=%v: the x/gov message server accepted a proposal carrying mint parameters", governance)

		harmless, err := govv1.NewMsgSubmitProposal(nil, deposit, proposer, "text", "text", "text", false)
		require.NoError(t, err)
		ctx, _ = c.ctx().CacheContext()
		_, err = server.SubmitProposal(ctx, harmless)
		if governance {
			require.NoError(t, err, "control: a text proposal is accepted while governance is enabled")
		} else {
			require.ErrorContains(t, err, "proposals are disabled")
		}
	}
}

// No interchain-accounts host or controller and no x/group: either would
// let a remote chain or a group policy dispatch messages through the router
// without the transaction ante handler.
func TestInterchainAccountsAndGroupAreNotWired(t *testing.T) {
	c := newChain(t, false)
	for name := range c.app.ModuleManager.Modules {
		for _, forbidden := range []string{"interchainaccounts", "icahost", "icacontroller", "group"} {
			require.NotContains(t, strings.ToLower(name), forbidden, "module %s is wired", name)
		}
	}
	for _, storeKey := range []string{"icahost", "icacontroller", "group"} {
		require.Nil(t, c.app.GetKey(storeKey), "store %s is mounted", storeKey)
	}
	router := c.app.MsgServiceRouter()
	for _, typeURL := range []string{
		"/cosmos.group.v1.MsgSubmitProposal",
		"/cosmos.group.v1.MsgExec",
		"/ibc.applications.interchain_accounts.host.v1.MsgModuleQuerySafe",
		"/ibc.applications.interchain_accounts.controller.v1.MsgSendTx",
		"/ibc.applications.interchain_accounts.controller.v1.MsgRegisterInterchainAccount",
	} {
		require.Nil(t, router.HandlerByTypeURL(typeURL), "%s has a message handler", typeURL)
		_, err := c.app.InterfaceRegistry().Resolve(typeURL)
		require.Error(t, err, "%s is registered", typeURL)
	}
	ports := c.app.GetIBCKeeper().PortKeeper.Router
	for _, port := range []string{"icahost", "icacontroller"} {
		require.False(t, ports.HasRoute(port), "IBC port %s is routed", port)
	}
	require.True(t, ports.HasRoute("transfer"), "control: the transfer port is routed")
}
