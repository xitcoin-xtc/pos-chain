package keeper

import (
	"encoding/binary"
	"math/big"

	"github.com/xitcoin-xtc/pos-chain/x/bridge/types"

	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// SignerSetVersion returns the version of the configured bridge signer set:
// the stored value, or 1 for a route configured before versions were stored
// (the xitcoin-security-hardening-v2 upgrade stores it), or 0 without a route.
func (k Keeper) SignerSetVersion(ctx sdk.Context) uint64 {
	store := ctx.KVStore(k.storeKey)
	if value := store.Get(types.KeySignerSetVersion); len(value) == 8 {
		return binary.BigEndian.Uint64(value)
	}
	if store.Has(types.KeyRouteConfig) {
		return 1
	}
	return 0
}

func (k Keeper) setSignerSetVersion(ctx sdk.Context, version uint64) {
	encoded := make([]byte, 8)
	binary.BigEndian.PutUint64(encoded, version)
	ctx.KVStore(k.storeKey).Set(types.KeySignerSetVersion, encoded)
}

// StoreSignerSetVersion writes signer set version 1 for a configured route
// that has no stored version yet and reports whether it wrote. It is the
// xitcoin-security-hardening-v2 migration and is idempotent.
func (k Keeper) StoreSignerSetVersion(ctx sdk.Context) bool {
	store := ctx.KVStore(k.storeKey)
	if !store.Has(types.KeyRouteConfig) || store.Has(types.KeySignerSetVersion) {
		return false
	}
	k.setSignerSetVersion(ctx, 1)
	return true
}

// OutboundNonce returns the nonce of the last outbound transfer.
func (k Keeper) OutboundNonce(ctx sdk.Context) uint64 {
	value := ctx.KVStore(k.storeKey).Get(types.KeyOutboundNonce)
	if len(value) != 8 {
		return 0
	}
	return binary.BigEndian.Uint64(value)
}

func (k Keeper) setAttestationRecord(ctx sdk.Context, id [32]byte, record types.AttestationRecord) error {
	encoded, err := record.Marshal()
	if err != nil {
		return err
	}
	ctx.KVStore(k.storeKey).Set(types.AttestationRecordKey(id), encoded)
	return nil
}

// GetAttestationRecord returns the record written when the attestation was
// processed.
func (k Keeper) GetAttestationRecord(ctx sdk.Context, id [32]byte) (types.AttestationRecord, bool, error) {
	value := ctx.KVStore(k.storeKey).Get(types.AttestationRecordKey(id))
	if len(value) == 0 {
		return types.AttestationRecord{}, false, nil
	}
	var record types.AttestationRecord
	if err := record.Unmarshal(value); err != nil {
		return types.AttestationRecord{}, false, err
	}
	return record, true, nil
}

func (k Keeper) setOutboundTransfer(ctx sdk.Context, id [32]byte, record types.OutboundTransferRecord) error {
	encoded, err := record.Marshal()
	if err != nil {
		return err
	}
	ctx.KVStore(k.storeKey).Set(types.OutboundTransferKey(id), encoded)
	return nil
}

// GetOutboundTransfer returns the record written by the burning transaction.
func (k Keeper) GetOutboundTransfer(ctx sdk.Context, id [32]byte) (types.OutboundTransferRecord, bool, error) {
	value := ctx.KVStore(k.storeKey).Get(types.OutboundTransferKey(id))
	if len(value) == 0 {
		return types.OutboundTransferRecord{}, false, nil
	}
	var record types.OutboundTransferRecord
	if err := record.Unmarshal(value); err != nil {
		return types.OutboundTransferRecord{}, false, err
	}
	return record, true, nil
}

// AttestationRecords returns every attestation record in store order.
func (k Keeper) AttestationRecords(ctx sdk.Context) ([]types.AttestationRecord, error) {
	iterator := storetypes.KVStorePrefixIterator(ctx.KVStore(k.storeKey), types.KeyAttestationRecordPrefix)
	defer iterator.Close()
	var records []types.AttestationRecord
	for ; iterator.Valid(); iterator.Next() {
		var record types.AttestationRecord
		if err := record.Unmarshal(iterator.Value()); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

// OutboundTransfers returns every outbound transfer record in store order.
func (k Keeper) OutboundTransfers(ctx sdk.Context) ([]types.OutboundTransferRecord, error) {
	iterator := storetypes.KVStorePrefixIterator(ctx.KVStore(k.storeKey), types.KeyOutboundTransferPrefix)
	defer iterator.Close()
	var records []types.OutboundTransferRecord
	for ; iterator.Valid(); iterator.Next() {
		var record types.OutboundTransferRecord
		if err := record.Unmarshal(iterator.Value()); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

// LatestDailyUsage returns the most recent non-empty daily usage bucket of a
// route. It does not depend on the block time, which an export context does
// not carry.
func (k Keeper) LatestDailyUsage(ctx sdk.Context, routeID string) (uint64, *big.Int, bool) {
	prefix := dailyUsageKey(routeID, 0)[:len(dailyUsagePrefix)+32]
	iterator := storetypes.KVStoreReversePrefixIterator(ctx.KVStore(k.storeKey), prefix)
	defer iterator.Close()
	for ; iterator.Valid(); iterator.Next() {
		key := iterator.Key()
		if len(key) != len(prefix)+8 {
			continue
		}
		used := new(big.Int).SetBytes(iterator.Value())
		if used.Sign() == 0 {
			continue
		}
		return binary.BigEndian.Uint64(key[len(prefix):]), used, true
	}
	return 0, nil, false
}

func (k Keeper) setDailyUsage(ctx sdk.Context, routeID string, day uint64, used *big.Int) {
	ctx.KVStore(k.storeKey).Set(dailyUsageKeyForDay(routeID, day), used.Bytes())
}
