package keeper

import (
	"encoding/json"
	"errors"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/xitcoin-xtc/pos-chain/x/bridge/types"
)

var (
	ErrRoutePaused        = errors.New("bridge route is paused")
	ErrRouteAlreadyPaused = errors.New("bridge route is already paused")
)

var (
	routeStateKey        = []byte{0x05}
	processedPausePrefix = []byte{0x06}
)

// RouteState is deliberately narrow. The guardian can only set Paused=true.
type RouteState struct {
	Paused bool `json:"paused"`
}

func (k Keeper) GetRouteState(ctx sdk.Context) (RouteState, error) {
	value := ctx.KVStore(k.storeKey).Get(routeStateKey)
	if len(value) == 0 {
		return RouteState{}, nil
	}
	var state RouteState
	if err := json.Unmarshal(value, &state); err != nil {
		return RouteState{}, err
	}
	return state, nil
}

func (k Keeper) RequireRouteAvailable(ctx sdk.Context, config types.RouteConfig) error {
	if err := config.Validate(); err != nil {
		return err
	}
	if !config.Enabled {
		return ErrRouteDisabled
	}
	state, err := k.GetRouteState(ctx)
	if err != nil {
		return err
	}
	if state.Paused {
		return ErrRoutePaused
	}
	return nil
}

// PauseRoute records a guardian-authorised suspension and nothing else.
func (k Keeper) PauseRoute(ctx sdk.Context, config types.RouteConfig, action types.GuardianPauseAction, signature []byte) error {
	if err := config.Validate(); err != nil {
		return err
	}
	if action.RouteID != config.RouteID {
		return errors.New("guardian pause route does not match configuration")
	}
	if err := action.Validate(); err != nil {
		return err
	}
	if ctx.BlockTime().Unix() >= action.ExpiresUnix {
		return errors.New("guardian pause action has expired")
	}
	id, err := action.ID()
	if err != nil {
		return err
	}
	store := ctx.KVStore(k.storeKey)
	if store.Has(processedPauseKey(id)) {
		return ErrControlActionProcessed
	}
	if _, err := types.VerifyGuardianPause(action, config.Guardian, signature); err != nil {
		return err
	}
	state, err := k.GetRouteState(ctx)
	if err != nil {
		return err
	}
	if state.Paused {
		return ErrRouteAlreadyPaused
	}
	encoded, err := json.Marshal(RouteState{Paused: true})
	if err != nil {
		return err
	}
	store.Set(routeStateKey, encoded)
	store.Set(processedPauseKey(id), []byte{1})
	return nil
}

func processedPauseKey(id [32]byte) []byte {
	key := make([]byte, len(processedPausePrefix)+len(id))
	copy(key, processedPausePrefix)
	copy(key[len(processedPausePrefix):], id[:])
	return key
}
