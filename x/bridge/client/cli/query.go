package cli

import (
	"github.com/spf13/cobra"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"

	"github.com/xitcoin-xtc/pos-chain/x/bridge/types"
)

func GetQueryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "bridge",
		Short:                      "Query canonical bridge state",
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}
	cmd.AddCommand(GetAttestationStatusCmd())
	return cmd
}

func GetAttestationStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "attestation-status ATTESTATION_ID",
		Short: "Query whether a bridge attestation has been processed",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}
			response, err := types.NewQueryClient(clientCtx).AttestationStatus(
				cmd.Context(),
				&types.QueryAttestationStatusRequest{AttestationId: args[0]},
			)
			if err != nil {
				return err
			}
			return clientCtx.PrintProto(response)
		},
	}
	flags.AddQueryFlagsToCmd(cmd)
	return cmd
}
