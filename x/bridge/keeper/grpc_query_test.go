package keeper

import (
	"context"
	"strings"
	"testing"

	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	sdktestutil "github.com/cosmos/cosmos-sdk/testutil"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/xitcoin-xtc/pos-chain/x/bridge/types"
)

func queryContext(t *testing.T) (Keeper, context.Context) {
	t.Helper()
	key := storetypes.NewKVStoreKey(types.StoreKey)
	ctx := sdktestutil.DefaultContext(key, storetypes.NewTransientStoreKey("bridge_query_test"))
	return NewKeeper(key), sdk.WrapSDKContext(ctx)
}

func TestAttestationStatusReportsCanonicalState(t *testing.T) {
	k, ctx := queryContext(t)
	idText := "0x" + strings.Repeat("ab", 32)

	before, err := k.AttestationStatus(ctx, &types.QueryAttestationStatusRequest{AttestationId: idText})
	if err != nil {
		t.Fatalf("query before processing failed: %v", err)
	}
	if before.Processed || before.AttestationId != idText {
		t.Fatalf("unexpected initial response: %+v", before)
	}

	id, _, _ := types.ParseAttestationID(idText)
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	k.MarkProcessed(sdkCtx, id)

	after, err := k.AttestationStatus(ctx, &types.QueryAttestationStatusRequest{AttestationId: strings.ToUpper(idText)})
	if err != nil {
		t.Fatalf("query after processing failed: %v", err)
	}
	if !after.Processed || after.AttestationId != idText {
		t.Fatalf("unexpected processed response: %+v", after)
	}
}

func TestAttestationStatusRejectsInvalidRequests(t *testing.T) {
	k, ctx := queryContext(t)
	if _, err := k.AttestationStatus(ctx, nil); err == nil {
		t.Fatal("nil request accepted")
	}
	if _, err := k.AttestationStatus(ctx, &types.QueryAttestationStatusRequest{AttestationId: "0x01"}); err == nil {
		t.Fatal("short ID accepted")
	}
}
