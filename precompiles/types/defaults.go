package types

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"

	transferkeeper "github.com/cosmos/ibc-go/v11/modules/apps/transfer/keeper"
	channelkeeper "github.com/cosmos/ibc-go/v11/modules/core/04-channel/keeper"
	evmaddress "github.com/xitcoin-xtc/pos-chain/encoding/address"
	ibcutils "github.com/xitcoin-xtc/pos-chain/ibc"
	cmn "github.com/xitcoin-xtc/pos-chain/precompiles/common"
	erc20Keeper "github.com/xitcoin-xtc/pos-chain/x/erc20/keeper"

	"cosmossdk.io/core/address"

	"github.com/cosmos/cosmos-sdk/codec"
	sdktypes "github.com/cosmos/cosmos-sdk/types"
	distributionkeeper "github.com/cosmos/cosmos-sdk/x/distribution/keeper"
	govkeeper "github.com/cosmos/cosmos-sdk/x/gov/keeper"
	slashingkeeper "github.com/cosmos/cosmos-sdk/x/slashing/keeper"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

// Optionals define some optional params that can be applied to _some_ precompiles.
// Extend this struct, add a sane default to defaultOptionals, and an Option function to provide users with a non-breaking
// way to provide custom args to certain precompiles.
type Optionals struct {
	AddressCodec       address.Codec // used by gov/staking
	ValidatorAddrCodec address.Codec // used by slashing
	ConsensusAddrCodec address.Codec // used by slashing
	AllowGovSubmit     bool          // used by governance

	// StakingMsgServerDecorator and SlashingMsgServerDecorator wrap the
	// message servers that the staking and slashing precompiles call
	// directly, so that application policies (for example validator
	// admission) also apply to EVM callers. Nil leaves the server unchanged.
	StakingMsgServerDecorator  func(stakingtypes.MsgServer) stakingtypes.MsgServer
	SlashingMsgServerDecorator func(slashingtypes.MsgServer) slashingtypes.MsgServer
}

func defaultOptionals() Optionals {
	return Optionals{
		AddressCodec:       evmaddress.NewEvmCodec(sdktypes.GetConfig().GetBech32AccountAddrPrefix()),
		ValidatorAddrCodec: evmaddress.NewEvmCodec(sdktypes.GetConfig().GetBech32ValidatorAddrPrefix()),
		ConsensusAddrCodec: evmaddress.NewEvmCodec(sdktypes.GetConfig().GetBech32ConsensusAddrPrefix()),
		AllowGovSubmit:     true,
	}
}

type Option func(opts *Optionals)

func WithAddressCodec(codec address.Codec) Option {
	return func(opts *Optionals) {
		opts.AddressCodec = codec
	}
}

func WithValidatorAddrCodec(codec address.Codec) Option {
	return func(opts *Optionals) {
		opts.ValidatorAddrCodec = codec
	}
}

func WithConsensusAddrCodec(codec address.Codec) Option {
	return func(opts *Optionals) {
		opts.ConsensusAddrCodec = codec
	}
}

// WithStakingMsgServerDecorator wraps the message server used by the staking precompile.
func WithStakingMsgServerDecorator(decorator func(stakingtypes.MsgServer) stakingtypes.MsgServer) Option {
	return func(opts *Optionals) {
		opts.StakingMsgServerDecorator = decorator
	}
}

// WithSlashingMsgServerDecorator wraps the message server used by the slashing precompile.
func WithSlashingMsgServerDecorator(decorator func(slashingtypes.MsgServer) slashingtypes.MsgServer) Option {
	return func(opts *Optionals) {
		opts.SlashingMsgServerDecorator = decorator
	}
}

// WithGovernanceProposalSubmission configures the EVM governance precompile.
func WithGovernanceProposalSubmission(enabled bool) Option {
	return func(opts *Optionals) {
		opts.AllowGovSubmit = enabled
	}
}

const bech32PrecompileBaseGas = 6_000

// DefaultStaticPrecompiles returns the list of all available static precompiled contracts from Cosmos EVM.
//
// NOTE: this should only be used during initialization of the Keeper.
func DefaultStaticPrecompiles(
	stakingKeeper stakingkeeper.Keeper,
	distributionKeeper distributionkeeper.Keeper,
	bankKeeper cmn.BankKeeper,
	erc20Keeper *erc20Keeper.Keeper,
	transferKeeper *transferkeeper.Keeper,
	channelKeeper *channelkeeper.Keeper,
	clientKeeper ibcutils.ClientKeeper,
	govKeeper govkeeper.Keeper,
	slashingKeeper slashingkeeper.Keeper,
	codec codec.Codec,
	opts ...Option,
) map[common.Address]vm.PrecompiledContract {
	precompiles := NewStaticPrecompiles().
		WithPraguePrecompiles().
		WithP256Precompile().
		WithBech32Precompile().
		WithStakingPrecompile(stakingKeeper, bankKeeper, opts...).
		WithDistributionPrecompile(distributionKeeper, stakingKeeper, bankKeeper, opts...).
		WithICS02Precompile(codec, clientKeeper).
		WithICS20Precompile(bankKeeper, stakingKeeper, transferKeeper, channelKeeper, erc20Keeper).
		WithBankPrecompile(bankKeeper, erc20Keeper).
		WithGovPrecompile(govKeeper, bankKeeper, codec, opts...).
		WithSlashingPrecompile(slashingKeeper, bankKeeper, opts...)

	return map[common.Address]vm.PrecompiledContract(precompiles)
}
