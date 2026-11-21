package keeper

import (
	"errors"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/xitcoin-xtc/pos-chain/x/validatorincentives/types"
)

const IncentiveDenom = "axtc"

// Treasury provides the narrow, non-minting bank boundary used by validator
// incentives. Its module account is registered without mint or burn rights.
type Treasury struct {
	accountKeeper types.AccountKeeper
	bankKeeper    types.BankKeeper
}

func NewTreasury(
	accountKeeper types.AccountKeeper,
	bankKeeper types.BankKeeper,
) Treasury {
	return Treasury{
		accountKeeper: accountKeeper,
		bankKeeper:    bankKeeper,
	}
}

func (t Treasury) Balance(ctx sdk.Context) (sdk.Coin, error) {
	address := t.accountKeeper.GetModuleAddress(
		types.TreasuryAccountName,
	)
	if len(address) == 0 {
		return sdk.Coin{}, errors.New("incentive treasury module account is missing")
	}

	return t.bankKeeper.GetBalance(
		ctx,
		address,
		IncentiveDenom,
	), nil
}

func (t Treasury) Send(
	ctx sdk.Context,
	recipient sdk.AccAddress,
	amount sdkmath.Int,
) error {
	if len(recipient) == 0 {
		return errors.New("reward recipient is empty")
	}
	if !amount.IsPositive() {
		return errors.New("reward amount must be positive")
	}

	balance, err := t.Balance(ctx)
	if err != nil {
		return err
	}
	if balance.Amount.LT(amount) {
		return errors.New("incentive treasury balance is insufficient")
	}

	return t.bankKeeper.SendCoinsFromModuleToAccount(
		ctx,
		types.TreasuryAccountName,
		recipient,
		sdk.NewCoins(sdk.NewCoin(IncentiveDenom, amount)),
	)
}

func (t Treasury) SendToModule(
	ctx sdk.Context,
	recipientModule string,
	amount sdkmath.Int,
) error {
	if recipientModule == "" {
		return errors.New("reward recipient module is empty")
	}
	if !amount.IsPositive() {
		return errors.New("reward amount must be positive")
	}
	balance, err := t.Balance(ctx)
	if err != nil {
		return err
	}
	if balance.Amount.LT(amount) {
		return errors.New("incentive treasury balance is insufficient")
	}

	return t.bankKeeper.SendCoinsFromModuleToModule(
		ctx,
		types.TreasuryAccountName,
		recipientModule,
		sdk.NewCoins(sdk.NewCoin(IncentiveDenom, amount)),
	)
}
