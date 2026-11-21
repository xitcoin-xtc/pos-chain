package keeper

import (
	"encoding/json"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/xitcoin-xtc/pos-chain/x/bridge/types"
)

var routeConfigKey = types.KeyRouteConfig

// SetRouteConfig stores only a validated configuration. Transactions reach it
// only through MsgInitializeRouteConfig (authority, once, disabled and paused)
// and a 2-of-3 update_route_config control action.
func (k Keeper) SetRouteConfig(ctx sdk.Context, config types.RouteConfig) error {
	if err := config.Validate(); err != nil {
		return err
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		return err
	}
	ctx.KVStore(k.storeKey).Set(routeConfigKey, encoded)
	return nil
}

func (k Keeper) GetRouteConfig(ctx sdk.Context) (types.RouteConfig, bool, error) {
	value := ctx.KVStore(k.storeKey).Get(routeConfigKey)
	if len(value) == 0 {
		return types.RouteConfig{}, false, nil
	}
	var config types.RouteConfig
	if err := json.Unmarshal(value, &config); err != nil {
		return types.RouteConfig{}, false, err
	}
	if err := config.Validate(); err != nil {
		return types.RouteConfig{}, false, err
	}
	return config, true, nil
}
