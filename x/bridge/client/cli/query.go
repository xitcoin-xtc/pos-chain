package cli

import (
	"github.com/spf13/cobra"

	"github.com/xitcoin-xtc/pos-chain/x/bridge/types"

	"github.com/cosmos/gogoproto/proto"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
)

func GetQueryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "bridge",
		Short:                      "Query canonical bridge state",
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}
	cmd.AddCommand(
		GetAttestationStatusCmd(), GetRouteCmd(), GetSourceDepositKeyCmd(),
		GetParamsCmd(), GetRouteConfigCmd(), GetOutstandingCmd(), GetDailyUsageCmd(),
		GetAttestationCmd(), GetSourceDepositCmd(), GetOutboundTransferCmd(),
	)
	return cmd
}

// typedQueryCmd builds a command that calls one typed bridge query and
// prints its response.
func typedQueryCmd(use, short string, args cobra.PositionalArgs, call func(cmd *cobra.Command, client types.QueryClient, args []string) (proto.Message, error)) *cobra.Command {
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  args,
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}
			response, err := call(cmd, types.NewQueryClient(clientCtx), args)
			if err != nil {
				return err
			}
			return clientCtx.PrintProto(response)
		},
	}
	flags.AddQueryFlagsToCmd(cmd)
	return cmd
}

func GetParamsCmd() *cobra.Command {
	return typedQueryCmd("params", "Query the bridge settlement parameters and signing domains", cobra.NoArgs,
		func(cmd *cobra.Command, client types.QueryClient, _ []string) (proto.Message, error) {
			return client.Params(cmd.Context(), &types.QueryParamsRequest{})
		})
}

func GetRouteConfigCmd() *cobra.Command {
	return typedQueryCmd("route-config ROUTE_ID", "Query the configuration and state of a bridge route by its stored route ID", cobra.ExactArgs(1),
		func(cmd *cobra.Command, client types.QueryClient, args []string) (proto.Message, error) {
			return client.Route(cmd.Context(), &types.QueryRouteRequest{RouteId: args[0]})
		})
}

func GetOutstandingCmd() *cobra.Command {
	return typedQueryCmd("outstanding", "Query the bridge outstanding amount and its cap", cobra.NoArgs,
		func(cmd *cobra.Command, client types.QueryClient, _ []string) (proto.Message, error) {
			return client.Outstanding(cmd.Context(), &types.QueryOutstandingRequest{})
		})
}

func GetDailyUsageCmd() *cobra.Command {
	return typedQueryCmd("daily-usage", "Query the route's daily limit usage for the UTC day of the queried block", cobra.NoArgs,
		func(cmd *cobra.Command, client types.QueryClient, _ []string) (proto.Message, error) {
			return client.DailyUsage(cmd.Context(), &types.QueryDailyUsageRequest{})
		})
}

func GetAttestationCmd() *cobra.Command {
	return typedQueryCmd("attestation ATTESTATION_ID", "Query whether an attestation is absent or processed, with its approved fields and processing transaction", cobra.ExactArgs(1),
		func(cmd *cobra.Command, client types.QueryClient, args []string) (proto.Message, error) {
			return client.Attestation(cmd.Context(), &types.QueryAttestationRequest{AttestationId: args[0]})
		})
}

func GetSourceDepositCmd() *cobra.Command {
	return typedQueryCmd("source-deposit SOURCE_CHAIN_ID SOURCE_REF", "Query the attestation status of a Cronos deposit by source chain ID and depositId", cobra.ExactArgs(2),
		func(cmd *cobra.Command, client types.QueryClient, args []string) (proto.Message, error) {
			return client.SourceDeposit(cmd.Context(), &types.QuerySourceDepositRequest{SourceChainId: args[0], SourceRef: args[1]})
		})
}

func GetOutboundTransferCmd() *cobra.Command {
	return typedQueryCmd("outbound-transfer REQUEST_ID", "Query the burn recorded for an outbound request ID", cobra.ExactArgs(1),
		func(cmd *cobra.Command, client types.QueryClient, args []string) (proto.Message, error) {
			return client.OutboundTransfer(cmd.Context(), &types.QueryOutboundTransferRequest{RequestId: args[0]})
		})
}

func GetAttestationStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "attestation-status ATTESTATION_ID",
		Short: "Query whether a source deposit (replay key, see source-deposit-key) has been minted",
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
