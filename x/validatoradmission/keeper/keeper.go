package keeper

import (
	"bytes"
	"encoding/binary"
	"sort"
	"strings"

	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/xitcoin-xtc/pos-chain/x/validatoradmission/types"
)

type Keeper struct {
	storeKey storetypes.StoreKey
}

func NewKeeper(storeKey storetypes.StoreKey) Keeper {
	return Keeper{storeKey: storeKey}
}

func (k Keeper) SetAuthority(ctx sdk.Context, authority string) {
	ctx.KVStore(k.storeKey).Set(types.KeyAuthority, []byte(authority))
}

func (k Keeper) GetAuthority(ctx sdk.Context) string {
	return string(ctx.KVStore(k.storeKey).Get(types.KeyAuthority))
}

func (k Keeper) SetMaxApprovedValidators(ctx sdk.Context, value uint32) {
	buffer := make([]byte, 4)
	binary.BigEndian.PutUint32(buffer, value)
	ctx.KVStore(k.storeKey).Set(types.KeyMaxApprovedValidators, buffer)
}

func (k Keeper) GetMaxApprovedValidators(ctx sdk.Context) uint32 {
	value := ctx.KVStore(k.storeKey).Get(types.KeyMaxApprovedValidators)
	if len(value) != 4 {
		maxApprovedValidators, _ := types.DefaultPolicy()
		return maxApprovedValidators
	}
	return binary.BigEndian.Uint32(value)
}

func (k Keeper) SetMinimumSelfDelegation(ctx sdk.Context, value string) {
	ctx.KVStore(k.storeKey).Set(types.KeyMinimumSelfDelegation, []byte(value))
}

func (k Keeper) GetMinimumSelfDelegation(ctx sdk.Context) string {
	value := string(ctx.KVStore(k.storeKey).Get(types.KeyMinimumSelfDelegation))
	if value == "" {
		_, minimumSelfDelegation := types.DefaultPolicy()
		return minimumSelfDelegation
	}
	return value
}

func (k Keeper) SetApprovedValidator(ctx sdk.Context, validatorAddress string, approved bool) {
	store := ctx.KVStore(k.storeKey)
	key := append(append([]byte{}, types.KeyApprovedValidatorPrefix...), []byte(validatorAddress)...)

	if approved {
		store.Set(key, []byte{1})
		return
	}

	store.Delete(key)
}

func (k Keeper) IsApprovedValidator(ctx sdk.Context, validatorAddress string) bool {
	key := append(append([]byte{}, types.KeyApprovedValidatorPrefix...), []byte(strings.TrimSpace(validatorAddress))...)
	return ctx.KVStore(k.storeKey).Has(key)
}

func (k Keeper) ApprovedValidatorCount(ctx sdk.Context) uint32 {
	store := ctx.KVStore(k.storeKey)
	iterator := store.Iterator(types.KeyApprovedValidatorPrefix, nil)
	defer iterator.Close()

	var count uint32
	for ; iterator.Valid(); iterator.Next() {
		if !bytes.HasPrefix(iterator.Key(), types.KeyApprovedValidatorPrefix) {
			break
		}
		count++
	}
	return count
}

func (k Keeper) InitGenesis(ctx sdk.Context, state types.GenesisState) {
	maxApprovedValidators, minimumSelfDelegation := state.Policy()
	k.SetAuthority(ctx, state.Authority)
	k.SetMaxApprovedValidators(ctx, maxApprovedValidators)
	k.SetMinimumSelfDelegation(ctx, minimumSelfDelegation)
	for _, validatorAddress := range state.ApprovedValidators {
		k.SetApprovedValidator(ctx, validatorAddress, true)
	}
}

func (k Keeper) ExportGenesis(ctx sdk.Context) types.GenesisState {
	store := ctx.KVStore(k.storeKey)
	iterator := store.Iterator(types.KeyApprovedValidatorPrefix, nil)
	defer iterator.Close()

	approvedValidators := []string{}
	for ; iterator.Valid(); iterator.Next() {
		key := iterator.Key()
		if !bytes.HasPrefix(key, types.KeyApprovedValidatorPrefix) {
			break
		}
		approvedValidators = append(approvedValidators, string(key[len(types.KeyApprovedValidatorPrefix):]))
	}
	sort.Strings(approvedValidators)

	return types.GenesisState{
		Authority:             k.GetAuthority(ctx),
		ApprovedValidators:    approvedValidators,
		MaxApprovedValidators: k.GetMaxApprovedValidators(ctx),
		MinimumSelfDelegation: k.GetMinimumSelfDelegation(ctx),
	}
}
