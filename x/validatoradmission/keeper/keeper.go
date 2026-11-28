package keeper

import (
	"bytes"
	"encoding/binary"
	"sort"

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

// canonicalOperator returns the canonical spelling (lower-case bech32) of a
// validator operator address. The bech32 decoder also accepts the upper-case
// spelling of the same address, so approvals are stored and looked up under
// the canonical spelling only.
func canonicalOperator(validatorAddress string) (string, bool) {
	operator, err := sdk.ValAddressFromBech32(validatorAddress)
	if err != nil {
		return "", false
	}
	return operator.String(), true
}

func approvalKey(validatorAddress string) []byte {
	return append(append([]byte{}, types.KeyApprovedValidatorPrefix...), []byte(validatorAddress)...)
}

// SetApprovedValidator approves or revokes the operator that
// validatorAddress names, whatever its spelling. An address that is not a
// valid operator address cannot be approved. A revocation also removes an
// entry stored under the exact spelling given, so that an entry written
// before approvals were canonical can be removed.
func (k Keeper) SetApprovedValidator(ctx sdk.Context, validatorAddress string, approved bool) {
	store := ctx.KVStore(k.storeKey)
	canonical, valid := canonicalOperator(validatorAddress)

	if approved {
		if valid {
			store.Set(approvalKey(canonical), []byte{1})
		}
		return
	}

	if valid {
		store.Delete(approvalKey(canonical))
	}
	store.Delete(approvalKey(validatorAddress))
}

// IsApprovedValidator reports whether the operator that validatorAddress
// names, in any spelling the bech32 decoder accepts, is approved.
func (k Keeper) IsApprovedValidator(ctx sdk.Context, validatorAddress string) bool {
	canonical, valid := canonicalOperator(validatorAddress)
	return valid && ctx.KVStore(k.storeKey).Has(approvalKey(canonical))
}

// RemoveNonCanonicalApprovals deletes every approval entry that is not
// stored under the canonical spelling of a valid operator address, and
// returns the removed spellings. Such entries could be written before
// approvals were canonical (an approval spelled in upper case); they are
// never matched since, so removing them changes no admission decision.
func (k Keeper) RemoveNonCanonicalApprovals(ctx sdk.Context) []string {
	store := ctx.KVStore(k.storeKey)
	iterator := store.Iterator(types.KeyApprovedValidatorPrefix, storetypes.PrefixEndBytes(types.KeyApprovedValidatorPrefix))
	var removed []string
	for ; iterator.Valid(); iterator.Next() {
		spelling := string(iterator.Key()[len(types.KeyApprovedValidatorPrefix):])
		if canonical, valid := canonicalOperator(spelling); !valid || canonical != spelling {
			removed = append(removed, spelling)
		}
	}
	iterator.Close()
	for _, spelling := range removed {
		store.Delete(approvalKey(spelling))
	}
	return removed
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
