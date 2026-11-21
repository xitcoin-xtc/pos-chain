// Package keeper stores bridge admission and native settlement state.
package keeper

import (
	"context"
	"errors"

	sdkmath "cosmossdk.io/math"
	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/xitcoin-xtc/pos-chain/x/bridge/types"
)

var ErrAttestationAlreadyProcessed = errors.New("bridge attestation already processed")

type Keeper struct {
	storeKey    storetypes.StoreKey
	bankKeeper  BankKeeper
	nativeDenom string
	maxSupply   sdkmath.Int
	authority   string
}

type BankKeeper interface {
	MintCoins(ctx context.Context, moduleName string, amt sdk.Coins) error
	BurnCoins(ctx context.Context, moduleName string, amt sdk.Coins) error
	SendCoinsFromModuleToAccount(ctx context.Context, senderModule string, recipientAddr sdk.AccAddress, amt sdk.Coins) error
	SendCoinsFromAccountToModule(ctx context.Context, senderAddr sdk.AccAddress, recipientModule string, amt sdk.Coins) error
	GetSupply(ctx context.Context, denom string) sdk.Coin
}

// Authority returns the account allowed to initialize the first route.
func (k Keeper) Authority() string { return k.authority }

func NewKeeper(storeKey storetypes.StoreKey) Keeper {
	return Keeper{storeKey: storeKey}
}

func NewSettlementKeeper(storeKey storetypes.StoreKey, bankKeeper BankKeeper, nativeDenom, maxSupply, authority string) (Keeper, error) {
	if bankKeeper == nil {
		return Keeper{}, errors.New("bridge bank keeper is required")
	}
	if err := sdk.ValidateDenom(nativeDenom); err != nil {
		return Keeper{}, err
	}
	limit, ok := sdkmath.NewIntFromString(maxSupply)
	if !ok || !limit.IsPositive() {
		return Keeper{}, errors.New("bridge maximum supply must be positive")
	}
	if _, err := sdk.AccAddressFromBech32(authority); err != nil {
		return Keeper{}, errors.New("bridge authority must be a valid account address")
	}
	return Keeper{storeKey: storeKey, bankKeeper: bankKeeper, nativeDenom: nativeDenom, maxSupply: limit, authority: authority}, nil
}

func MustNewSettlementKeeper(storeKey storetypes.StoreKey, bankKeeper BankKeeper, nativeDenom, maxSupply, authority string) Keeper {
	keeper, err := NewSettlementKeeper(storeKey, bankKeeper, nativeDenom, maxSupply, authority)
	if err != nil {
		panic(err)
	}
	return keeper
}

func (k Keeper) IsProcessed(ctx sdk.Context, id [32]byte) bool {
	return ctx.KVStore(k.storeKey).Has(processedKey(id))
}

func (k Keeper) MarkProcessed(ctx sdk.Context, id [32]byte) {
	ctx.KVStore(k.storeKey).Set(processedKey(id), []byte{1})
}

// ConsumeAttestation atomically reserves a valid attestation identifier in the
// transaction state. A second call for the same ID fails before settlement.
func (k Keeper) ConsumeAttestation(ctx sdk.Context, attestation types.Attestation) ([32]byte, error) {
	id, err := attestation.ID()
	if err != nil {
		return [32]byte{}, err
	}
	if k.IsProcessed(ctx, id) {
		return [32]byte{}, ErrAttestationAlreadyProcessed
	}
	k.MarkProcessed(ctx, id)
	return id, nil
}

func processedKey(id [32]byte) []byte {
	key := make([]byte, len(types.KeyProcessedAttestationPrefix)+len(id))
	copy(key, types.KeyProcessedAttestationPrefix)
	copy(key[len(types.KeyProcessedAttestationPrefix):], id[:])
	return key
}
