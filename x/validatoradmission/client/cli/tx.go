package cli

import (
	"strconv"

	"github.com/spf13/cobra"

	"github.com/xitcoin-xtc/pos-chain/x/validatoradmission/types"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/tx"
)

// NewTxCmd returns Xitcoin Validator Admission transaction commands.
func NewTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "validator-admission",
		Short:                      "Xitcoin validator admission commands",
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmd.AddCommand(
		NewApproveValidatorCmd(),
		NewRevokeValidatorCmd(),
		NewUpdateParamsCmd(),
	)
	return cmd
}

func addXitcoinTxFlags(cmd *cobra.Command) {
	flags.AddTxFlagsToCmd(cmd)

	if flag := cmd.Flags().Lookup(flags.FlagFees); flag != nil {
		flag.Usage = "Fees in axtc, for example 1000000000000000axtc"
	}
	if flag := cmd.Flags().Lookup(flags.FlagGasPrices); flag != nil {
		flag.Usage = "Gas prices in axtc, for example 1000000000axtc"
	}
}

// NewApproveValidatorCmd approves a validator address on Xitcoin.
func NewApproveValidatorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "approve-validator VALIDATOR_ADDRESS",
		Short: "Approve a validator address on Xitcoin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			msg := &types.MsgApproveValidator{
				Authority:        clientCtx.GetFromAddress().String(),
				ValidatorAddress: args[0],
			}
			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}

	addXitcoinTxFlags(cmd)
	return cmd
}

// NewRevokeValidatorCmd revokes and suspends a validator address on Xitcoin.
func NewRevokeValidatorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "revoke-validator VALIDATOR_ADDRESS",
		Short: "Revoke and suspend a validator address on Xitcoin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			msg := &types.MsgRevokeValidator{
				Authority:        clientCtx.GetFromAddress().String(),
				ValidatorAddress: args[0],
			}
			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}

	addXitcoinTxFlags(cmd)
	return cmd
}

// NewUpdateParamsCmd updates Validator Admission policy parameters.
func NewUpdateParamsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-params MAX_APPROVED_VALIDATORS MINIMUM_SELF_DELEGATION",
		Short: "Update Validator Admission policy parameters",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			maxApprovedValidators, err := strconv.ParseUint(args[0], 10, 32)
			if err != nil {
				return err
			}

			msg := &types.MsgUpdateParams{
				Authority:             clientCtx.GetFromAddress().String(),
				MaxApprovedValidators: uint32(maxApprovedValidators),
				MinimumSelfDelegation: args[1],
			}
			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}

	addXitcoinTxFlags(cmd)
	return cmd
}
