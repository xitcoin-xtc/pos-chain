package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/xitcoin-xtc/pos-chain/x/validatorincentives/types"
)

// HandleUpdateParamsCommand validates the domain command before delegating to
// the authority-protected keeper operation.
func (k Keeper) HandleUpdateParamsCommand(
	ctx sdk.Context,
	command types.UpdateParamsCommand,
) error {
	if err := command.ValidateBasic(); err != nil {
		return err
	}
	return k.UpdateParamsAuthorized(
		ctx,
		command.Authority,
		command.Params,
	)
}
