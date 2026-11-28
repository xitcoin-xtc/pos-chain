package validatoradmission

// The bypass table (docs/validator-admission-attack-simulations.md): every
// restricted action (create a validator without approval or below the
// minimum, unjail or re-create a revoked validator, change mint parameters,
// submit a proposal while governance is disabled, carry a restricted message
// in a proposal while it is enabled) is attempted through every path that
// reaches the message servers: a direct transaction, a self-granted and a
// nested authz MsgExec, a grant then MsgExec, the staking, slashing and
// governance precompiles called from an EOA, from a contract, through
// DELEGATECALL and CALLCODE, from an EIP-7702 delegated account and from the
// ICS20 source and destination callbacks, and a governance proposal. Each
// attempt must leave the protected state unchanged. Where a path can succeed
// for an approved operator, a positive control shows that it really reaches
// the message server, so a rejection is the policy and not a broken harness.

import (
	"fmt"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	callbacktypes "github.com/xitcoin-xtc/pos-chain/x/ibc/callbacks/types"
	validatoradmissiontypes "github.com/xitcoin-xtc/pos-chain/x/validatoradmission/types"

	transfertypes "github.com/cosmos/ibc-go/v11/modules/apps/transfer/types"
	clienttypes "github.com/cosmos/ibc-go/v11/modules/core/02-client/types"
	channeltypes "github.com/cosmos/ibc-go/v11/modules/core/04-channel/types"

	sdkmath "cosmossdk.io/math"

	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/authz"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	govv1beta1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1beta1"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

var oneToken = big.NewInt(1e18)

func exec(grantee testKey, msgs ...sdk.Msg) *authz.MsgExec {
	msg := authz.NewMsgExec(grantee.acc, msgs)
	return &msg
}

func nested(grantee testKey, depth int, msg sdk.Msg) sdk.Msg {
	for i := 0; i < depth; i++ {
		msg = exec(grantee, msg)
	}
	return msg
}

func grant(t *testing.T, granter, grantee testKey, msg sdk.Msg) *authz.MsgGrant {
	t.Helper()
	g, err := authz.NewMsgGrant(granter.acc, grantee.acc, authz.NewGenericAuthorization(sdk.MsgTypeURL(msg)), nil)
	require.NoError(t, err)
	return g
}

// requireRejected asserts that a Cosmos transaction fails, with the
// admission error when want is set.
func requireRejected(t *testing.T, err error, want string) {
	t.Helper()
	require.Error(t, err, "the transaction was accepted")
	if want != "" {
		require.Contains(t, err.Error(), want)
	}
}

// ---------------------------------------------------------------------------
// IBC callbacks.

// sourceCallback runs the ICS20 source callback (acknowledgement or timeout)
// of a transfer sent by contract, as the callbacks middleware does, on a
// cached context. The returned context holds the resulting state.
func (c *chain) sourceCallback(contract common.Address, timeout bool) (sdk.Context, error) {
	c.t.Helper()
	ctx, _ := c.ctx().CacheContext()
	ctx = ctx.WithGasMeter(storetypes.NewGasMeter(10_000_000))
	sender := sdk.AccAddress(contract.Bytes()).String()
	data := transfertypes.NewFungibleTokenPacketData(bondDenom, "1", sender, "cosmos1receiver",
		fmt.Sprintf(`{"src_callback": {"address": "%s"}}`, contract.Hex()))
	packet := channeltypes.NewPacket(data.GetBytes(), 1, transfertypes.PortID, "channel-0", transfertypes.PortID, "channel-1",
		clienttypes.ZeroHeight(), 10_000_000)
	keeper := c.app.GetCallbackKeeper()
	if timeout {
		return ctx, keeper.IBCOnTimeoutPacketCallback(ctx, packet, c.attacker.acc, contract.Hex(), sender, transfertypes.V1)
	}
	ack := channeltypes.NewResultAcknowledgement([]byte{1})
	return ctx, keeper.IBCOnAcknowledgementPacketCallback(ctx, packet, ack.Acknowledgement(), c.attacker.acc, contract.Hex(), sender, transfertypes.V1)
}

// destinationCallback runs the ICS20 destination callback of a packet that
// returns the native token, with calldata forwardArmed() to contract, on a
// cached context.
func (c *chain) destinationCallback(contract common.Address) (sdk.Context, error) {
	c.t.Helper()
	ctx, _ := c.ctx().CacheContext()
	ctx = ctx.WithGasMeter(storetypes.NewGasMeter(10_000_000))
	remoteSender := "remote1sender"
	receiver := callbacktypes.GenerateIsolatedAddress("channel-1", remoteSender)
	calldata := packAttacker(c.t, "forwardArmed")
	data := transfertypes.NewFungibleTokenPacketData("transfer/channel-0/"+bondDenom, "1", remoteSender, receiver.String(),
		fmt.Sprintf(`{"dest_callback": {"address": "%s", "calldata": "%x"}}`, contract.Hex(), calldata))
	packet := channeltypes.NewPacket(data.GetBytes(), 1, transfertypes.PortID, "channel-0", transfertypes.PortID, "channel-1",
		clienttypes.ZeroHeight(), 10_000_000)
	ack := channeltypes.NewResultAcknowledgement([]byte{1})
	return ctx, c.app.GetCallbackKeeper().IBCReceivePacketCallback(ctx, packet, ack, contract.Hex(), transfertypes.V1)
}

func validatorIn(t *testing.T, c *chain, ctx sdk.Context, key testKey) (stakingtypes.Validator, bool) {
	t.Helper()
	validator, err := c.app.StakingKeeper.GetValidator(ctx, sdk.ValAddress(key.acc))
	if err != nil {
		return stakingtypes.Validator{}, false
	}
	return validator, true
}

// ---------------------------------------------------------------------------
// Validator creation without approval.

func TestCreateValidatorWithoutApprovalIsRejectedOnEveryPath(t *testing.T) {
	c := newChain(t, false)
	attacker := c.attacker
	create := c.createValidatorMsg(attacker, "attacker", minimumSelfDelegation, minimumSelfDelegation)

	t.Run("direct transaction", func(t *testing.T) {
		requireRejected(t, c.cosmosTx(attacker, create), "not approved")
	})
	t.Run("self-granted MsgExec", func(t *testing.T) {
		requireRejected(t, c.cosmosTx(attacker, exec(attacker, create)), "cannot be executed through authz")
	})
	for _, depth := range []int{2, 3, 6, 7, 10} {
		t.Run(fmt.Sprintf("MsgExec nested %d deep", depth), func(t *testing.T) {
			requireRejected(t, c.cosmosTx(attacker, nested(attacker, depth, create)), "")
		})
	}
	t.Run("grant then MsgExec", func(t *testing.T) {
		requireRejected(t, c.cosmosTx(attacker, grant(t, attacker, c.candidate, create)), "authorizations for")
		requireRejected(t, c.cosmosTx(c.candidate, exec(c.candidate, create)), "cannot be executed through authz")
	})
	t.Run("staking precompile from the EOA", func(t *testing.T) {
		_, err := c.call(attacker, stakingAddress, createValidatorInput(t, attacker.addr, "attacker", minimumSelfDelegation, minimumSelfDelegation))
		require.Error(t, err)
	})
	contract := c.deployAttacker(attacker, new(big.Int).Mul(big.NewInt(2_000), oneToken))
	contractKey := testKey{acc: sdk.AccAddress(contract.Bytes()), addr: contract}
	for name, mode := range map[string]uint8{"contract CALL": modeCall, "DELEGATECALL": modeDelegateCall, "CALLCODE": modeCallCode} {
		t.Run("staking precompile through "+name, func(t *testing.T) {
			// The contract (CALL) or the calling contract's context
			// (DELEGATECALL, CALLCODE) is the operator.
			input := createValidatorInput(t, contract, "contract", minimumSelfDelegation, minimumSelfDelegation)
			require.False(t, c.forward(attacker, contract, stakingAddress, input, mode))
			c.requireNoValidator(contractKey)
		})
	}

	// EIP-7702: the delegated account runs the attacker code, so the
	// precompile sees the account itself as the caller.
	c.delegateCode(c.delegated, contract)
	delegatedInput := createValidatorInput(t, c.delegated.addr, "delegated", minimumSelfDelegation, minimumSelfDelegation)
	t.Run("staking precompile from an EIP-7702 account", func(t *testing.T) {
		require.False(t, c.forward(attacker, c.delegated.addr, stakingAddress, delegatedInput, modeCall))
		require.False(t, c.forward(c.delegated, c.delegated.addr, stakingAddress, delegatedInput, modeCall))
		for _, mode := range []uint8{modeDelegateCall, modeCallCode} {
			require.False(t, c.forward(attacker, c.delegated.addr, stakingAddress, delegatedInput, mode))
		}
	})
	// The IBC callbacks only call contracts (not EIP-7702 accounts), and a
	// contract cannot operate a validator: the contract forwards a creation
	// of its own validator.
	contractInput := createValidatorInput(t, contract, "contract", minimumSelfDelegation, minimumSelfDelegation)
	for _, mode := range []uint8{modeCall, modeDelegateCall, modeCallCode} {
		c.arm(attacker, contract, stakingAddress, contractInput, mode)
		for _, timeout := range []bool{false, true} {
			t.Run(fmt.Sprintf("ICS20 source callback (mode=%d timeout=%v)", mode, timeout), func(t *testing.T) {
				ctx, _ := c.sourceCallback(contract, timeout)
				_, found := validatorIn(t, c, ctx, contractKey)
				require.False(t, found)
			})
		}
		t.Run(fmt.Sprintf("ICS20 destination callback (mode=%d)", mode), func(t *testing.T) {
			ctx, _ := c.destinationCallback(contract)
			_, found := validatorIn(t, c, ctx, contractKey)
			require.False(t, found)
		})
	}

	for _, key := range []testKey{attacker, c.delegated, contractKey} {
		c.requireNoValidator(key)
	}

	// Positive controls: the callbacks reach the staking precompile (a
	// delegation from the contract goes through), and the EIP-7702 path
	// creates the validator of an operator once it is approved.
	t.Run("control: callbacks reach the staking precompile", func(t *testing.T) {
		validator := c.net.GetValidators()[0].OperatorAddress
		c.arm(attacker, contract, stakingAddress, delegateInput(t, contract, validator, oneToken), modeCall)
		for _, timeout := range []bool{false, true} {
			ctx, err := c.sourceCallback(contract, timeout)
			require.NoError(t, err)
			_, err = c.app.StakingKeeper.GetDelegation(ctx, contractKey.acc, sdk.MustValAddressFromBech32(validator))
			require.NoError(t, err, "the source callback did not delegate")
		}
		ctx, err := c.destinationCallback(contract)
		require.NoError(t, err)
		_, err = c.app.StakingKeeper.GetDelegation(ctx, contractKey.acc, sdk.MustValAddressFromBech32(validator))
		require.NoError(t, err, "the destination callback did not delegate")
	})
	t.Run("control: approved operator through EIP-7702", func(t *testing.T) {
		approve := &validatoradmissiontypes.MsgApproveValidator{Authority: c.authority.acc.String(), ValidatorAddress: c.delegated.valoper()}
		c.mustCosmosTx(c.authority, approve)
		require.True(t, c.forward(attacker, c.delegated.addr, stakingAddress, delegatedInput, modeCall))
		_, found := c.validator(c.delegated)
		require.True(t, found)
	})
}

// ---------------------------------------------------------------------------
// min_self_delegation below the admission minimum.

func TestMinSelfDelegationBelowMinimumIsRejected(t *testing.T) {
	c := newChain(t, false)
	candidate := c.candidate
	below := minimumSelfDelegation.SubRaw(1)

	t.Run("creation: direct transaction", func(t *testing.T) {
		requireRejected(t, c.cosmosTx(candidate, c.createValidatorMsg(candidate, "candidate", minimumSelfDelegation, below)), "min_self_delegation must be at least")
		requireRejected(t, c.cosmosTx(candidate, c.createValidatorMsg(candidate, "candidate", below, below)), "self delegation must be at least")
	})
	t.Run("creation: staking precompile", func(t *testing.T) {
		_, err := c.call(candidate, stakingAddress, createValidatorInput(t, candidate.addr, "candidate", minimumSelfDelegation, below))
		require.Error(t, err)
	})
	contract := c.deployAttacker(c.attacker, nil)
	c.delegateCode(candidate, contract)
	t.Run("creation: EIP-7702 account", func(t *testing.T) {
		input := createValidatorInput(t, candidate.addr, "candidate", minimumSelfDelegation, below)
		require.False(t, c.forward(c.attacker, candidate.addr, stakingAddress, input, modeCall))
	})
	c.requireNoValidator(candidate)

	// Control: the minimum itself is accepted, through the EIP-7702 path.
	input := createValidatorInput(t, candidate.addr, "candidate", minimumSelfDelegation, minimumSelfDelegation)
	require.True(t, c.forward(c.attacker, candidate.addr, stakingAddress, input, modeCall))
	validator, found := c.validator(candidate)
	require.True(t, found)
	require.True(t, validator.MinSelfDelegation.Equal(minimumSelfDelegation))

	// An edit can never lower min_self_delegation below the minimum.
	keep := stakingtypes.DoNotModifyDesc
	edit := func(value sdkmath.Int) *stakingtypes.MsgEditValidator {
		description := stakingtypes.Description{Moniker: keep, Identity: keep, Website: keep, SecurityContact: keep, Details: keep}
		return stakingtypes.NewMsgEditValidator(candidate.valoper(), description, nil, &value)
	}
	t.Run("edit: direct transaction", func(t *testing.T) {
		requireRejected(t, c.cosmosTx(candidate, edit(below)), "")
		requireRejected(t, c.cosmosTx(candidate, edit(sdkmath.OneInt())), "")
	})
	t.Run("edit: MsgExec", func(t *testing.T) {
		requireRejected(t, c.cosmosTx(candidate, exec(candidate, edit(below))), "")
	})
	t.Run("edit: staking precompile from the EIP-7702 account", func(t *testing.T) {
		require.False(t, c.forward(c.attacker, candidate.addr, stakingAddress, editValidatorInput(t, candidate.addr, below.BigInt()), modeCall))
		require.False(t, c.forward(candidate, candidate.addr, stakingAddress, editValidatorInput(t, candidate.addr, big.NewInt(1)), modeCall))
	})
	validator, found = c.validator(candidate)
	require.True(t, found)
	require.True(t, validator.MinSelfDelegation.Equal(minimumSelfDelegation), "min_self_delegation changed to %s", validator.MinSelfDelegation)

	// Control: the same EIP-7702 path raises it once the self-delegation
	// covers the new value.
	c.mustCosmosTx(candidate, stakingtypes.NewMsgDelegate(candidate.acc.String(), candidate.valoper(), sdk.NewCoin(bondDenom, sdkmath.NewInt(1))))
	raised := minimumSelfDelegation.AddRaw(1)
	require.True(t, c.forward(c.attacker, candidate.addr, stakingAddress, editValidatorInput(t, candidate.addr, raised.BigInt()), modeCall))
	validator, _ = c.validator(candidate)
	require.True(t, validator.MinSelfDelegation.Equal(raised))
}

// ---------------------------------------------------------------------------
// Unjailing a revoked validator.

// jailBySelfUndelegation jails the operator's validator the way the staking
// module does: its self-delegation falls below min_self_delegation. It then
// restores the self-delegation, so that only the jail remains.
func (c *chain) jailBySelfUndelegation(key testKey) {
	c.t.Helper()
	one := sdk.NewCoin(bondDenom, sdkmath.NewInt(1))
	c.mustCosmosTx(key, stakingtypes.NewMsgUndelegate(key.acc.String(), key.valoper(), one))
	c.requireJailed(key)
	c.mustCosmosTx(key, stakingtypes.NewMsgDelegate(key.acc.String(), key.valoper(), one))
	c.requireJailed(key)
}

func (c *chain) requireUnjailed(key testKey) {
	c.t.Helper()
	validator, found := c.validator(key)
	require.True(c.t, found)
	require.False(c.t, validator.IsJailed(), "validator %s is still jailed", key.valoper())
}

func TestRevokedValidatorCannotUnjailOnAnyPath(t *testing.T) {
	c := newChain(t, false)
	operator := c.operator
	c.mustCosmosTx(operator, c.createValidatorMsg(operator, "operator", minimumSelfDelegation, minimumSelfDelegation))
	contract := c.deployAttacker(c.attacker, nil)
	unjail := &slashingtypes.MsgUnjail{ValidatorAddr: operator.valoper()}
	input := unjailInput(t, operator.addr)

	// Positive controls while approved: a direct MsgUnjail, the slashing
	// precompile, the EIP-7702 path and the callbacks all unjail.
	c.jailBySelfUndelegation(operator)
	c.mustCosmosTx(operator, unjail)
	c.requireUnjailed(operator)
	c.jailBySelfUndelegation(operator)
	_, err := c.call(operator, slashingAddress, input)
	require.NoError(t, err)
	c.requireUnjailed(operator)
	c.delegateCode(operator, contract)
	c.jailBySelfUndelegation(operator)
	require.True(t, c.forward(c.attacker, operator.addr, slashingAddress, input, modeCall))
	c.requireUnjailed(operator)
	c.jailBySelfUndelegation(operator)

	// Revocation. The validator is already jailed; revocation removes the
	// approval.
	c.mustCosmosTx(c.authority, &validatoradmissiontypes.MsgRevokeValidator{Authority: c.authority.acc.String(), ValidatorAddress: operator.valoper()})
	require.False(t, c.app.ValidatorAdmissionKeeper.IsApprovedValidator(c.ctx(), operator.valoper()))
	c.requireJailed(operator)

	t.Run("direct transaction", func(t *testing.T) {
		requireRejected(t, c.cosmosTx(operator, unjail), "revoked validator cannot unjail")
	})
	t.Run("self-granted MsgExec", func(t *testing.T) {
		requireRejected(t, c.cosmosTx(operator, exec(operator, unjail)), "cannot be executed through authz")
	})
	t.Run("nested MsgExec", func(t *testing.T) {
		requireRejected(t, c.cosmosTx(operator, nested(operator, 4, unjail)), "")
	})
	t.Run("grant then MsgExec", func(t *testing.T) {
		requireRejected(t, c.cosmosTx(operator, grant(t, operator, c.attacker, unjail)), "authorizations for")
		requireRejected(t, c.cosmosTx(c.attacker, exec(c.attacker, unjail)), "cannot be executed through authz")
	})
	t.Run("slashing precompile from the operator", func(t *testing.T) {
		_, err := c.call(operator, slashingAddress, input)
		require.Error(t, err)
	})
	t.Run("slashing precompile from the operator's EIP-7702 code", func(t *testing.T) {
		for _, mode := range []uint8{modeCall, modeDelegateCall, modeCallCode} {
			require.False(t, c.forward(c.attacker, operator.addr, slashingAddress, input, mode))
			require.False(t, c.forward(operator, operator.addr, slashingAddress, input, mode))
		}
	})
	t.Run("slashing precompile from a contract", func(t *testing.T) {
		for _, mode := range []uint8{modeCall, modeDelegateCall, modeCallCode} {
			require.False(t, c.forward(c.attacker, contract, slashingAddress, input, mode))
		}
	})
	// The IBC callbacks call contracts only, never the operator's EIP-7702
	// account; a contract forwarding the unjail is not the operator.
	for _, mode := range []uint8{modeCall, modeDelegateCall, modeCallCode} {
		c.arm(c.attacker, contract, slashingAddress, input, mode)
		for _, timeout := range []bool{false, true} {
			t.Run(fmt.Sprintf("ICS20 source callback (mode=%d timeout=%v)", mode, timeout), func(t *testing.T) {
				ctx, _ := c.sourceCallback(contract, timeout)
				validator, found := validatorIn(t, c, ctx, operator)
				require.True(t, found)
				require.True(t, validator.IsJailed())
			})
		}
		t.Run(fmt.Sprintf("ICS20 destination callback (mode=%d)", mode), func(t *testing.T) {
			ctx, _ := c.destinationCallback(contract)
			validator, found := validatorIn(t, c, ctx, operator)
			require.True(t, found)
			require.True(t, validator.IsJailed())
		})
	}
	_, err = c.sourceCallback(operator.addr, false)
	require.ErrorContains(t, err, "not a contract", "an EIP-7702 account must not receive IBC callbacks")
	c.requireJailed(operator)
}

// The authority may spell an operator address in any form the bech32 decoder
// accepts. A revocation must remove the approval it names whatever the
// spelling, or the validator it just jailed can unjail at once.
func TestRevocationRemovesTheApprovalWhateverTheSpelling(t *testing.T) {
	c := newChain(t, false)
	operator := c.operator
	c.mustCosmosTx(operator, c.createValidatorMsg(operator, "operator", minimumSelfDelegation, minimumSelfDelegation))

	c.mustCosmosTx(c.authority, &validatoradmissiontypes.MsgRevokeValidator{
		Authority:        c.authority.acc.String(),
		ValidatorAddress: strings.ToUpper(operator.valoper()),
	})
	c.requireJailed(operator)
	require.False(t, c.app.ValidatorAdmissionKeeper.IsApprovedValidator(c.ctx(), operator.valoper()),
		"the revocation jailed the validator but left it approved")
	requireRejected(t, c.cosmosTx(operator, &slashingtypes.MsgUnjail{ValidatorAddr: operator.valoper()}), "")
	requireRejected(t, c.cosmosTx(operator, &slashingtypes.MsgUnjail{ValidatorAddr: strings.ToUpper(operator.valoper())}), "")
	c.requireJailed(operator)

	// An approval spelled in upper case approves the operator it names.
	c.mustCosmosTx(c.authority, &validatoradmissiontypes.MsgApproveValidator{
		Authority:        c.authority.acc.String(),
		ValidatorAddress: strings.ToUpper(c.attacker.valoper()),
	})
	require.True(t, c.app.ValidatorAdmissionKeeper.IsApprovedValidator(c.ctx(), c.attacker.valoper()))
	c.mustCosmosTx(c.attacker, c.createValidatorMsg(c.attacker, "attacker", minimumSelfDelegation, minimumSelfDelegation))
}

// ---------------------------------------------------------------------------
// Re-creating a revoked validator.

func TestRevokedValidatorCannotBeRecreated(t *testing.T) {
	c := newChain(t, false)
	operator := c.operator
	c.mustCosmosTx(operator, c.createValidatorMsg(operator, "operator", minimumSelfDelegation, minimumSelfDelegation))
	c.mustCosmosTx(c.authority, &validatoradmissiontypes.MsgRevokeValidator{Authority: c.authority.acc.String(), ValidatorAddress: operator.valoper()})
	c.requireJailed(operator)

	// Unbond everything and wait for the unbonding period: x/staking
	// removes the validator.
	validator, _ := c.validator(operator)
	c.mustCosmosTx(operator, stakingtypes.NewMsgUndelegate(operator.acc.String(), operator.valoper(), sdk.NewCoin(bondDenom, validator.Tokens)))
	params, err := c.app.StakingKeeper.GetParams(c.ctx())
	require.NoError(t, err)
	require.NoError(t, c.net.NextBlockAfter(params.UnbondingTime))
	c.nextBlock()
	c.nextBlock()
	c.requireNoValidator(operator)

	create := c.createValidatorMsg(operator, "operator-again", minimumSelfDelegation, minimumSelfDelegation)
	requireRejected(t, c.cosmosTx(operator, create), "not approved")
	requireRejected(t, c.cosmosTx(operator, exec(operator, create)), "cannot be executed through authz")
	_, err = c.call(operator, stakingAddress, createValidatorInput(t, operator.addr, "operator-again", minimumSelfDelegation, minimumSelfDelegation))
	require.Error(t, err)
	contract := c.deployAttacker(c.attacker, nil)
	c.delegateCode(operator, contract)
	require.False(t, c.forward(c.attacker, operator.addr, stakingAddress,
		createValidatorInput(t, operator.addr, "operator-again", minimumSelfDelegation, minimumSelfDelegation), modeCall))
	c.requireNoValidator(operator)
}

// ---------------------------------------------------------------------------
// Mint parameters and proposal submission while governance is disabled.

func TestMintParametersAndProposalsAreRejectedWithGovernanceDisabled(t *testing.T) {
	c := newChain(t, false)
	attacker := c.attacker
	params := c.mintParams()
	proposals := c.nextProposalID()

	changed := minttypes.DefaultParams()
	changed.MintDenom = bondDenom
	changed.InflationMax = sdkmath.LegacyNewDecWithPrec(99, 2)
	mint := &minttypes.MsgUpdateParams{Authority: attacker.acc.String(), Params: changed}
	t.Run("mint: direct transaction", func(t *testing.T) {
		requireRejected(t, c.cosmosTx(attacker, mint), "Mint parameters cannot be changed")
	})
	t.Run("mint: MsgExec and grant", func(t *testing.T) {
		requireRejected(t, c.cosmosTx(attacker, exec(attacker, mint)), "cannot be executed through authz")
		requireRejected(t, c.cosmosTx(attacker, grant(t, attacker, c.candidate, mint)), "authorizations for")
	})

	deposit := sdk.NewCoins(sdk.NewCoin(bondDenom, c.minDeposit()))
	proposal, err := govv1.NewMsgSubmitProposal([]sdk.Msg{&banktypes.MsgSend{
		FromAddress: c.govAddress().String(), ToAddress: attacker.acc.String(), Amount: sdk.NewCoins(sdk.NewInt64Coin(bondDenom, 1)),
	}}, deposit, attacker.acc.String(), "", "attack", "attack", false)
	require.NoError(t, err)
	legacy, err := govv1beta1.NewMsgSubmitProposal(govv1beta1.NewTextProposal("attack", "attack"), deposit, attacker.acc)
	require.NoError(t, err)
	t.Run("proposal: direct transaction", func(t *testing.T) {
		requireRejected(t, c.cosmosTx(attacker, proposal), "proposals are disabled")
		requireRejected(t, c.cosmosTx(attacker, legacy), "proposals are disabled")
	})
	t.Run("proposal: MsgExec, nested MsgExec and grant", func(t *testing.T) {
		requireRejected(t, c.cosmosTx(attacker, exec(attacker, proposal)), "")
		requireRejected(t, c.cosmosTx(attacker, nested(attacker, 3, legacy)), "")
		requireRejected(t, c.cosmosTx(attacker, grant(t, attacker, c.candidate, proposal)), "authorizations for")
	})
	t.Run("proposal: governance precompile from the EOA", func(t *testing.T) {
		_, err := c.call(attacker, govAddress, c.submitProposalInput(attacker.addr))
		require.Error(t, err)
	})
	contract := c.deployAttacker(attacker, new(big.Int).Mul(big.NewInt(100), oneToken))
	t.Run("proposal: governance precompile through a contract, DELEGATECALL and CALLCODE", func(t *testing.T) {
		for _, mode := range []uint8{modeCall, modeDelegateCall, modeCallCode} {
			require.False(t, c.forward(attacker, contract, govAddress, c.submitProposalInput(contract), mode))
		}
	})
	c.delegateCode(c.delegated, contract)
	t.Run("proposal: governance precompile from an EIP-7702 account", func(t *testing.T) {
		input := c.submitProposalInput(c.delegated.addr)
		require.False(t, c.forward(attacker, c.delegated.addr, govAddress, input, modeCall))
		require.False(t, c.forward(c.delegated, c.delegated.addr, govAddress, input, modeCall))
	})
	t.Run("proposal: governance precompile from the IBC callbacks", func(t *testing.T) {
		c.arm(attacker, contract, govAddress, c.submitProposalInput(contract), modeCall)
		for _, timeout := range []bool{false, true} {
			ctx, _ := c.sourceCallback(contract, timeout)
			id, err := c.app.GovKeeper.ProposalID.Peek(ctx)
			require.NoError(t, err)
			require.Equal(t, proposals, id)
		}
		ctx, _ := c.destinationCallback(contract)
		id, err := c.app.GovKeeper.ProposalID.Peek(ctx)
		require.NoError(t, err)
		require.Equal(t, proposals, id)
	})

	require.Equal(t, params, c.mintParams())
	require.Equal(t, proposals, c.nextProposalID())
}

// ---------------------------------------------------------------------------
// Governance enabled: a proposal must not carry a restricted message.

func TestProposalsCannotCarryRestrictedMessagesWithGovernanceEnabled(t *testing.T) {
	c := newChain(t, true)
	gov := c.govAddress()
	govOperator := sdk.ValAddress(gov).String()
	deposit := sdk.NewCoins(sdk.NewCoin(bondDenom, c.minDeposit()))
	params := c.mintParams()

	changed := params
	changed.InflationMax = sdkmath.LegacyNewDecWithPrec(99, 2)
	changed.InflationMin = sdkmath.LegacyNewDecWithPrec(98, 2)
	changed.InflationRateChange = sdkmath.LegacyNewDecWithPrec(99, 2)
	mint := &minttypes.MsgUpdateParams{Authority: gov.String(), Params: changed}
	createValidator, err := stakingtypes.NewMsgCreateValidator(govOperator, consensusKey("gov").PubKey(),
		sdk.NewCoin(bondDenom, minimumSelfDelegation), stakingtypes.Description{Moniker: "gov"},
		stakingtypes.NewCommissionRates(sdkmath.LegacyNewDecWithPrec(1, 1), sdkmath.LegacyNewDecWithPrec(2, 1), sdkmath.LegacyNewDecWithPrec(1, 2)),
		minimumSelfDelegation)
	require.NoError(t, err)
	govExec := authz.NewMsgExec(gov, []sdk.Msg{mint})
	grantMint, err := authz.NewMsgGrant(gov, c.attacker.acc, authz.NewGenericAuthorization(sdk.MsgTypeURL(mint)), nil)
	require.NoError(t, err)

	cases := map[string]sdk.Msg{
		"mint parameters":            mint,
		"validator creation":         createValidator,
		"unjail":                     &slashingtypes.MsgUnjail{ValidatorAddr: govOperator},
		"mint parameters in MsgExec": &govExec,
		"grant for mint parameters":  grantMint,
	}
	for name, msg := range cases {
		t.Run(name+" via MsgSubmitProposal", func(t *testing.T) {
			before := c.nextProposalID()
			proposal, err := govv1.NewMsgSubmitProposal([]sdk.Msg{msg}, deposit, c.genesisOperators[0].acc.String(), "", "attack", "attack", false)
			require.NoError(t, err)
			if err := c.cosmosTx(c.genesisOperators[0], proposal); err == nil {
				status := c.passProposal(before)
				require.NotEqual(t, govv1.StatusPassed, status, "a proposal carrying %s passed and executed", sdk.MsgTypeURL(msg))
				t.Fatalf("a proposal carrying %s was accepted (final status %s)", sdk.MsgTypeURL(msg), status)
			}
			require.Equal(t, before, c.nextProposalID())
			require.Equal(t, params, c.mintParams())
		})
	}
	t.Run("mint parameters via the governance precompile", func(t *testing.T) {
		before := c.nextProposalID()
		_, err := c.call(c.genesisOperators[0], govAddress, c.submitProposalInput(c.genesisOperators[0].addr, mint))
		if err == nil {
			status := c.passProposal(before)
			require.NotEqual(t, govv1.StatusPassed, status, "a precompile proposal carrying mint parameters passed")
			t.Fatalf("the governance precompile accepted a proposal carrying mint parameters (final status %s)", status)
		}
		require.Equal(t, before, c.nextProposalID())
		require.Equal(t, params, c.mintParams())
	})
	t.Run("mint parameters via a nested proposal", func(t *testing.T) {
		inner, err := govv1.NewMsgSubmitProposal([]sdk.Msg{mint}, deposit, gov.String(), "", "inner", "inner", false)
		require.NoError(t, err)
		before := c.nextProposalID()
		outer, err := govv1.NewMsgSubmitProposal([]sdk.Msg{inner}, deposit, c.genesisOperators[0].acc.String(), "", "outer", "outer", false)
		require.NoError(t, err)
		if err := c.cosmosTx(c.genesisOperators[0], outer); err == nil {
			c.passProposal(before)
			if c.nextProposalID() > before+1 {
				require.NotEqual(t, govv1.StatusPassed, c.passProposal(before+1))
			}
			t.Fatal("a proposal submitting a proposal that carries mint parameters was accepted")
		}
		require.Equal(t, before, c.nextProposalID())
		require.Equal(t, params, c.mintParams())
	})
	c.requireNoValidator(testKey{acc: gov})

	// Control: governance still executes an unrestricted proposal.
	t.Run("control: unrestricted proposal passes", func(t *testing.T) {
		before := c.nextProposalID()
		distribution, err := c.app.DistrKeeper.Params.Get(c.ctx())
		require.NoError(t, err)
		distribution.CommunityTax = sdkmath.LegacyNewDecWithPrec(3, 2)
		update := &distrtypes.MsgUpdateParams{Authority: gov.String(), Params: distribution}
		proposal, err := govv1.NewMsgSubmitProposal([]sdk.Msg{update}, deposit, c.genesisOperators[0].acc.String(), "", "ok", "ok", false)
		require.NoError(t, err)
		c.mustCosmosTx(c.genesisOperators[0], proposal)
		require.Equal(t, govv1.StatusPassed, c.passProposal(before))
		updated, err := c.app.DistrKeeper.Params.Get(c.ctx())
		require.NoError(t, err)
		require.True(t, updated.CommunityTax.Equal(distribution.CommunityTax))
	})
}
