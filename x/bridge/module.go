package bridge

import (
	"context"
	"encoding/json"
	"fmt"

	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/gorilla/mux"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/spf13/cobra"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"

	"github.com/xitcoin-xtc/pos-chain/x/bridge/client/cli"
	"github.com/xitcoin-xtc/pos-chain/x/bridge/keeper"
	"github.com/xitcoin-xtc/pos-chain/x/bridge/types"
)

const consensusVersion = 1

var (
	_ module.AppModule      = AppModule{}
	_ module.AppModuleBasic = AppModuleBasic{}
	_ module.HasABCIGenesis = AppModule{}
)

type AppModuleBasic struct{}

func (AppModuleBasic) Name() string { return types.ModuleName }
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
func (AppModuleBasic) ValidateGenesis(_ codec.JSONCodec, _ client.TxEncodingConfig, data json.RawMessage) error {
	var state types.GenesisState
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("invalid %s genesis: %w", types.ModuleName, err)
	}
	return state.Validate()
}
func (AppModuleBasic) RegisterRESTRoutes(_ client.Context, _ *mux.Router) {}
func (AppModuleBasic) RegisterGRPCGatewayRoutes(clientCtx client.Context, serveMux *runtime.ServeMux) {
	if err := types.RegisterQueryHandlerClient(context.Background(), serveMux, types.NewQueryClient(clientCtx)); err != nil {
		panic(err)
	}
}
func (AppModuleBasic) GetQueryCmd() *cobra.Command { return cli.GetQueryCmd() }
func (AppModuleBasic) GetTxCmd() *cobra.Command    { return cli.NewTxCmd() }
func (AppModuleBasic) ConsensusVersion() uint64    { return consensusVersion }

type AppModule struct {
	AppModuleBasic
	keeper keeper.Keeper
}

func NewAppModule(k keeper.Keeper) AppModule {
	return AppModule{AppModuleBasic: AppModuleBasic{}, keeper: k}
}
func (AppModule) Name() string { return types.ModuleName }
func (am AppModule) RegisterServices(cfg module.Configurator) {
	types.RegisterQueryServer(cfg.QueryServer(), am.keeper)
	types.RegisterMsgServer(cfg.MsgServer(), keeper.NewMsgServer(am.keeper))
}
func (am AppModule) InitGenesis(ctx sdk.Context, _ codec.JSONCodec, data json.RawMessage) []abci.ValidatorUpdate {
	var state types.GenesisState
	if err := json.Unmarshal(data, &state); err != nil {
		panic(fmt.Sprintf("invalid %s genesis: %s", types.ModuleName, err))
	}
	if err := state.Validate(); err != nil {
		panic(fmt.Sprintf("invalid %s genesis: %s", types.ModuleName, err))
	}
	am.keeper.InitGenesis(ctx, state)
	return []abci.ValidatorUpdate{}
}
func (am AppModule) ExportGenesis(ctx sdk.Context, _ codec.JSONCodec) json.RawMessage {
	state, err := json.Marshal(am.keeper.ExportGenesis(ctx))
	if err != nil {
		panic(err)
	}
	return state
}
func (AppModule) RegisterStoreDecoder(_ simtypes.StoreDecoderRegistry) {}
func (AppModule) GenerateGenesisState(_ *module.SimulationState)       {}
func (AppModule) WeightedOperations(_ module.SimulationState) []simtypes.WeightedOperation {
	return nil
}
func (AppModule) IsAppModule()        {}
func (AppModule) IsOnePerModuleType() {}
