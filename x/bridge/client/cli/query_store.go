package cli

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"

	"github.com/spf13/cobra"
	"github.com/xitcoin-xtc/pos-chain/x/bridge/types"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
)

// The commands below read the bridge store directly through an ABCI store
// query (path /store/bridge/key). They need no gRPC service and return the
// exact consensus values that the keeper reads. The typed queries in query.go
// (params, route-config, outstanding, daily-usage, attestation,
// source-deposit, outbound-transfer) return the same values through gRPC.

// RouteStoreView is the decoded route state returned by `query bridge route`.
type RouteStoreView struct {
	Height            int64              `json:"height"`
	Configured        bool               `json:"configured"`
	RouteConfig       *types.RouteConfig `json:"route_config,omitempty"`
	Paused            bool               `json:"paused"`
	OutstandingAmount string             `json:"outstanding_amount"`
	ControlNonce      uint64             `json:"control_nonce"`
	OutboundNonce     uint64             `json:"outbound_nonce"`
}

// DecodeOutstandingAmount decodes the big-endian unsigned outstanding amount.
// An absent value means zero.
func DecodeOutstandingAmount(value []byte) string {
	return new(big.Int).SetBytes(value).String()
}

// DecodeRouteStore assembles the route view from raw store values.
func DecodeRouteStore(height int64, config, state, outstanding, controlNonce, outboundNonce []byte) (RouteStoreView, error) {
	view := RouteStoreView{Height: height, OutstandingAmount: DecodeOutstandingAmount(outstanding)}
	if len(config) != 0 {
		var decoded types.RouteConfig
		if err := json.Unmarshal(config, &decoded); err != nil {
			return RouteStoreView{}, err
		}
		view.Configured = true
		view.RouteConfig = &decoded
	}
	if len(state) != 0 {
		var decoded struct {
			Paused bool `json:"paused"`
		}
		if err := json.Unmarshal(state, &decoded); err != nil {
			return RouteStoreView{}, err
		}
		view.Paused = decoded.Paused
	}
	for _, field := range []struct {
		raw    []byte
		target *uint64
	}{{controlNonce, &view.ControlNonce}, {outboundNonce, &view.OutboundNonce}} {
		switch len(field.raw) {
		case 0:
		case 8:
			*field.target = binary.BigEndian.Uint64(field.raw)
		default:
			return RouteStoreView{}, errors.New("invalid bridge nonce encoding")
		}
	}
	return view, nil
}

func queryStoreAt(clientCtx client.Context, key []byte) ([]byte, int64, error) {
	return clientCtx.QueryStore(key, types.StoreKey)
}

// GetRouteCmd prints the route configuration, paused flag, outstanding
// amount and nonces at one height.
func GetRouteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "route",
		Short: "Query the bridge route configuration, paused state, outstanding amount and nonces",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			clientCtx, err := client.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}
			values := make([][]byte, 0, 5)
			var height int64
			for _, key := range [][]byte{types.KeyRouteConfig, types.KeyRouteState, types.KeyOutstandingAmount, types.KeyControlNonce, types.KeyOutboundNonce} {
				value, at, err := queryStoreAt(clientCtx, key)
				if err != nil {
					return err
				}
				if height == 0 {
					height = at
					clientCtx = clientCtx.WithHeight(at)
				} else if at != height {
					return errors.New("bridge store values were read at different heights")
				}
				values = append(values, value)
			}
			view, err := DecodeRouteStore(height, values[0], values[1], values[2], values[3], values[4])
			if err != nil {
				return err
			}
			encoded, err := json.Marshal(view)
			if err != nil {
				return err
			}
			return clientCtx.PrintRaw(encoded)
		},
	}
	flags.AddQueryFlagsToCmd(cmd)
	return cmd
}

// GetSourceDepositKeyCmd computes the replay key of a source deposit offline.
// Its output is the ATTESTATION_ID accepted by `attestation-status`.
func GetSourceDepositKeyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "source-deposit-key SOURCE_CHAIN_ID SOURCE_REF",
		Short: "Compute the replay key of a Cronos deposit (offline)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := types.SourceDepositKey(args[0], args[1])
			if err != nil {
				return err
			}
			cmd.Println("0x" + hex.EncodeToString(key[:]))
			return nil
		},
	}
}
