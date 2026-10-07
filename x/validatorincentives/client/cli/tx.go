package cli

import (
	"strconv"

	"github.com/spf13/cobra"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/tx"

	"github.com/xitcoin-xtc/pos-chain/x/validatorincentives/types"
)

// NewTxCmd returns Xitcoin validator incentive transaction commands.
func NewTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "validator-incentives",
		Short:                      "Xitcoin prefunded validator incentive commands",
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmd.AddCommand(
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

// NewUpdateParamsCmd updates funded incentive operating parameters.
func NewUpdateParamsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use: "update-params TREASURY_RELEASE_RATE_BASIS_POINTS " +
			"BLOCKS_PER_YEAR CALCULATION_PERIOD_BLOCKS",
		Short: "Update validator incentive parameters",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			rate, err := strconv.ParseUint(args[0], 10, 32)
			if err != nil {
				return err
			}
			blocksPerYear, err := strconv.ParseUint(args[1], 10, 64)
			if err != nil {
				return err
			}
			calculationPeriodBlocks, err := strconv.ParseUint(
				args[2],
				10,
				64,
			)
			if err != nil {
				return err
			}

			msg := &types.MsgUpdateParams{
				Authority:                      clientCtx.GetFromAddress().String(),
				TreasuryReleaseRateBasisPoints: uint32(rate),
				BlocksPerYear:                  blocksPerYear,
				CalculationPeriodBlocks:        calculationPeriodBlocks,
			}
			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			return tx.GenerateOrBroadcastTxCLI(
				clientCtx,
				cmd.Flags(),
				msg,
			)
		},
	}

	addXitcoinTxFlags(cmd)
	return cmd
}
