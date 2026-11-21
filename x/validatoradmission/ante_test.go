package validatoradmission

import (
	"bytes"
	"testing"

	protov2 "google.golang.org/protobuf/proto"

	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	sdktestutil "github.com/cosmos/cosmos-sdk/testutil"
	sdk "github.com/cosmos/cosmos-sdk/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/xitcoin-xtc/pos-chain/x/validatoradmission/keeper"
	"github.com/xitcoin-xtc/pos-chain/x/validatoradmission/types"
)

type testTx struct {
	messages []sdk.Msg
}

func (tx testTx) GetMsgs() []sdk.Msg {
	return tx.messages
}

func (tx testTx) GetMsgsV2() ([]protov2.Message, error) {
	return nil, nil
}

func TestAdmissionAnteHandlerBlocksAndAllowsValidatorActions(t *testing.T) {
	key := storetypes.NewKVStoreKey(types.StoreKey)
	ctx := sdktestutil.DefaultContext(
		key,
		storetypes.NewTransientStoreKey("validator_admission_ante_test"),
	)

	validatorAddress := sdk.ValAddress(bytes.Repeat([]byte{21}, 20)).String()
	admissionKeeper := keeper.NewKeeper(key)

	minimumSelfDelegation, err := sdk.ParseCoinNormalized(
		types.DefaultMinimumSelfDelegation,
	)
	if err != nil {
		t.Fatalf("minimum self delegation parsing failed: %v", err)
	}

	actions := []struct {
		name string
		tx   sdk.Tx
	}{
		{
			name: "create validator",
			tx: testTx{messages: []sdk.Msg{
				&stakingtypes.MsgCreateValidator{
					ValidatorAddress: validatorAddress,
					Value:            minimumSelfDelegation,
				},
			}},
		},
		{
			name: "unjail validator",
			tx: testTx{messages: []sdk.Msg{
				&slashingtypes.MsgUnjail{ValidatorAddr: validatorAddress},
			}},
		},
	}

	for _, action := range actions {
		t.Run(action.name, func(t *testing.T) {
			nextCalled := false
			handler := NewAdmissionAnteHandler(
				admissionKeeper,
				func(ctx sdk.Context, _ sdk.Tx, _ bool) (sdk.Context, error) {
					nextCalled = true
					return ctx, nil
				},
			)

			if _, err := handler(ctx, action.tx, false); err == nil {
				t.Fatal("unapproved validator action was accepted")
			}
			if nextCalled {
				t.Fatal("unapproved validator action reached normal handler")
			}

			admissionKeeper.SetApprovedValidator(ctx, validatorAddress, true)
			nextCalled = false

			if _, err := handler(ctx, action.tx, false); err != nil {
				t.Fatalf("approved validator action was rejected: %v", err)
			}
			if !nextCalled {
				t.Fatal("approved validator action did not reach normal handler")
			}

			admissionKeeper.SetApprovedValidator(ctx, validatorAddress, false)
		})
	}
}

func TestAdmissionAnteHandlerRejectsOnChainGovernanceProposals(t *testing.T) {
	key := storetypes.NewKVStoreKey(types.StoreKey)
	ctx := sdktestutil.DefaultContext(
		key,
		storetypes.NewTransientStoreKey("fixed_policy_test"),
	)

	admissionKeeper := keeper.NewKeeper(key)
	nextCalled := false
	handler := NewAdmissionAnteHandler(
		admissionKeeper,
		func(ctx sdk.Context, _ sdk.Tx, _ bool) (sdk.Context, error) {
			nextCalled = true
			return ctx, nil
		},
	)

	proposal := &govtypes.MsgSubmitProposal{
		Proposer: sdk.AccAddress(bytes.Repeat([]byte{31}, 20)).String(),
		Metadata: "ipfs://xitcoin-governance-proposal",
		Title:    "Network proposal",
		Summary:  "This proposal must not reach the governance message server.",
	}

	if _, err := handler(ctx, testTx{messages: []sdk.Msg{proposal}}, false); err == nil {
		t.Fatal("on-chain governance proposal was accepted")
	}
	if nextCalled {
		t.Fatal("rejected governance proposal reached the next ante handler")
	}
}
