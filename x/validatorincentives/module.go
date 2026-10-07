package validatorincentives

import (
	"context"
	"encoding/json"
	"fmt"

	"cosmossdk.io/core/appmodule"

	"github.com/gorilla/mux"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/spf13/cobra"

	abci "github.com/cometbft/cometbft/abci/types"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"

	"github.com/xitcoin-xtc/pos-chain/x/validatorincentives/keeper"
	"github.com/xitcoin-xtc/pos-chain/x/validatorincentives/types"
)

const consensusVersion = 2

var (
	_ module.AppModule      = AppModule{}
	_ module.AppModuleBasic = AppModuleBasic{}
	_ module.HasABCIGenesis = AppModule{}
	_ appmodule.HasBeginBlocker = AppModule{}
)

type AppModuleBasic struct{}

func (AppModuleBasic) Name() string {
	return types.ModuleName
}

func (AppModuleBasic) RegisterLegacyAminoCodec(cdc *codec.LegacyAmino) {
	types.RegisterLegacyAminoCodec(cdc)
}

func (AppModuleBasic) RegisterInterfaces(registry codectypes.InterfaceRegistry) {
	types.RegisterInterfaces(registry)
}

func (AppModuleBasic) DefaultGenesis(_ codec.JSONCodec) json.RawMessage {
	state, err := json.Marshal(types.DefaultGenesisState())
	if err != nil {
		panic(err)
	}
	return state
}

func (AppModuleBasic) ValidateGenesis(
	_ codec.JSONCodec,
	_ client.TxEncodingConfig,
	data json.RawMessage,
) error {
	var state types.GenesisState
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("invalid %s genesis: %w", types.ModuleName, err)
	}
	return state.Validate()
}

func (AppModuleBasic) RegisterRESTRoutes(
	_ client.Context,
	_ *mux.Router,
) {
}

func (AppModuleBasic) RegisterGRPCGatewayRoutes(
	clientCtx client.Context,
	serveMux *runtime.ServeMux,
) {
	if err := types.RegisterQueryHandlerClient(
		context.Background(),
		serveMux,
		types.NewQueryClient(clientCtx),
	); err != nil {
		panic(err)
	}
}

func (AppModuleBasic) GetQueryCmd() *cobra.Command {
	return nil
}

func (AppModuleBasic) ConsensusVersion() uint64 {
	return consensusVersion
}

type AppModule struct {
	AppModuleBasic
	keeper        keeper.Keeper
	stakingKeeper types.StakingKeeper
	treasury      keeper.Treasury
}

func NewAppModule(
	k keeper.Keeper,
	stakingKeeper types.StakingKeeper,
	treasury keeper.Treasury,
) AppModule {
	return AppModule{
		AppModuleBasic: AppModuleBasic{},
		keeper:         k,
		stakingKeeper:  stakingKeeper,
		treasury:       treasury,
	}
}

func (AppModule) Name() string {
	return types.ModuleName
}

func (am AppModule) RegisterServices(cfg module.Configurator) {
	types.RegisterQueryServer(
		cfg.QueryServer(),
		keeper.NewQueryServer(am.keeper, am.treasury),
	)
	types.RegisterMsgServer(
		cfg.MsgServer(),
		keeper.NewMsgServerImpl(am.keeper),
	)
	if err := cfg.RegisterMigration(
		types.ModuleName,
		1,
		func(ctx sdk.Context) error { return am.keeper.Migrate1to2(ctx) },
	); err != nil {
		panic(fmt.Sprintf("failed to register %s migration: %s", types.ModuleName, err))
	}
}

// BeginBlock releases the deterministic share for the current block to the
// canonical fee collector. The distribution module accounts for it on the
// following block because it runs earlier in the begin-block order.
func (am AppModule) BeginBlock(goCtx context.Context) error {
	ctx := sdk.UnwrapSDKContext(goCtx)
	return am.keeper.ProcessBlock(
		ctx,
		am.stakingKeeper,
		am.treasury,
		authtypes.FeeCollectorName,
	)
}

func (am AppModule) InitGenesis(
	ctx sdk.Context,
	_ codec.JSONCodec,
	data json.RawMessage,
) []abci.ValidatorUpdate {
	var state types.GenesisState
	if err := json.Unmarshal(data, &state); err != nil {
		panic(fmt.Sprintf(
			"invalid %s genesis: %s",
			types.ModuleName,
			err,
		))
	}
	if err := state.Validate(); err != nil {
		panic(fmt.Sprintf(
			"invalid %s genesis: %s",
			types.ModuleName,
			err,
		))
	}

	am.keeper.InitGenesis(ctx, state)
	return []abci.ValidatorUpdate{}
}

func (am AppModule) ExportGenesis(
	ctx sdk.Context,
	_ codec.JSONCodec,
) json.RawMessage {
	state, err := json.Marshal(am.keeper.ExportGenesis(ctx))
	if err != nil {
		panic(err)
	}
	return state
}

func (AppModule) RegisterStoreDecoder(
	_ simtypes.StoreDecoderRegistry,
) {
}

func (AppModule) GenerateGenesisState(
	_ *module.SimulationState,
) {
}

func (AppModule) WeightedOperations(
	_ module.SimulationState,
) []simtypes.WeightedOperation {
	return nil
}

func (AppModule) IsAppModule() {}

func (AppModule) IsOnePerModuleType() {}
