package cli

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/tx"

	"github.com/xitcoin-xtc/pos-chain/x/bridge/types"
)

// NewTxCmd returns the canonical bridge transaction commands.
func NewTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Submit canonical bridge transactions",
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmd.AddCommand(
		NewSubmitAttestationCmd(),
		NewInitiateOutboundTransferCmd(),
		NewInitializeRouteConfigCmd(),
		NewEmergencyPauseRouteCmd(),
		NewResumeRouteCmd(),
		NewUpdateRouteConfigCmd(),
	)
	return cmd
}

// NewEmergencyPauseRouteCmd submits a guardian-signed pause action.
func NewEmergencyPauseRouteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pause-route ROUTE_ID NONCE EXPIRES_UNIX GUARDIAN_SIGNATURE_HEX",
		Short: "Pause the bridge route using the dedicated guardian approval",
		Args:  cobra.ExactArgs(4),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			nonce, err := strconv.ParseUint(args[1], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid nonce: %w", err)
			}
			expires, err := strconv.ParseInt(args[2], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid expiry: %w", err)
			}
			signature, err := decodeSignature(args[3])
			if err != nil {
				return err
			}
			msg := &types.MsgEmergencyPauseRoute{Submitter: clientCtx.GetFromAddress().String(), RouteId: args[0], Nonce: nonce, ExpiresUnix: expires, GuardianSignature: signature}
			if err := msg.ValidateBasic(); err != nil {
				return err
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	addBridgeTxFlags(cmd)
	return cmd
}

// NewResumeRouteCmd submits a threshold-approved route resume.
func NewResumeRouteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resume-route ROUTE_ID NONCE NOT_BEFORE_UNIX EXPIRES_UNIX SIGNATURE_HEX SIGNATURE_HEX [SIGNATURE_HEX]",
		Short: "Resume a paused bridge route using current signer approvals",
		Args:  cobra.RangeArgs(6, 7),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			nonce, notBefore, expires, err := parseControlWindow(args[1], args[2], args[3])
			if err != nil {
				return err
			}
			signatures, err := decodeSignatures(args[4:])
			if err != nil {
				return err
			}
			msg := &types.MsgResumeRoute{Submitter: clientCtx.GetFromAddress().String(), RouteId: args[0], Nonce: nonce, NotBeforeUnix: notBefore, ExpiresUnix: expires, Signatures: signatures}
			if err := msg.ValidateBasic(); err != nil {
				return err
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	addBridgeTxFlags(cmd)
	return cmd
}

// NewUpdateRouteConfigCmd submits a threshold-approved exact configuration.
func NewUpdateRouteConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-route ROUTE_ID SIGNER_1 SIGNER_2 SIGNER_3 GUARDIAN MAX_TRANSFER_ATOMIC DAILY_LIMIT_ATOMIC MAX_OUTSTANDING_ATOMIC ENABLED NONCE NOT_BEFORE_UNIX EXPIRES_UNIX SIGNATURE_HEX SIGNATURE_HEX [SIGNATURE_HEX]",
		Short: "Update bridge authorities, limits, or enabled state using current signer approvals",
		Args:  cobra.RangeArgs(14, 15),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			enabled, err := strconv.ParseBool(args[8])
			if err != nil {
				return fmt.Errorf("invalid enabled value: %w", err)
			}
			nonce, notBefore, expires, err := parseControlWindow(args[9], args[10], args[11])
			if err != nil {
				return err
			}
			signatures, err := decodeSignatures(args[12:])
			if err != nil {
				return err
			}
			msg := &types.MsgUpdateRouteConfig{Submitter: clientCtx.GetFromAddress().String(), RouteId: args[0], BridgeSigners: []string{args[1], args[2], args[3]}, Guardian: args[4], MaxTransferAmount: args[5], DailyLimit: args[6], MaxOutstandingAmount: args[7], Enabled: enabled, Nonce: nonce, NotBeforeUnix: notBefore, ExpiresUnix: expires, Signatures: signatures}
			if err := msg.ValidateBasic(); err != nil {
				return err
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	addBridgeTxFlags(cmd)
	return cmd
}

// NewInitializeRouteConfigCmd creates the first route disabled and paused.
func NewInitializeRouteConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "initialize-route ROUTE_ID SIGNER_1 SIGNER_2 SIGNER_3 GUARDIAN MAX_TRANSFER_ATOMIC DAILY_LIMIT_ATOMIC MAX_OUTSTANDING_ATOMIC",
		Short: "Initialize the first bridge route in a disabled and paused state",
		Args:  cobra.ExactArgs(8),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			msg := &types.MsgInitializeRouteConfig{
				Authority: clientCtx.GetFromAddress().String(), RouteId: args[0],
				BridgeSigners: []string{args[1], args[2], args[3]}, Guardian: args[4],
				MaxTransferAmount: args[5], DailyLimit: args[6], MaxOutstandingAmount: args[7],
			}
			if err := msg.ValidateBasic(); err != nil {
				return err
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	addBridgeTxFlags(cmd)
	return cmd
}

// NewSubmitAttestationCmd submits a threshold-approved Cronos lock attestation.
func NewSubmitAttestationCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "submit-attestation ROUTE_ID SOURCE_CHAIN_ID SOURCE_REF NONCE XITCOIN_DESTINATION AMOUNT_ATOMIC DEADLINE_UNIX SIGNATURE_HEX SIGNATURE_HEX [SIGNATURE_HEX]",
		Short: "Submit a finalized Cronos-to-Xitcoin bridge attestation",
		Args:  cobra.RangeArgs(9, 10),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			nonce, err := strconv.ParseUint(args[3], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid nonce: %w", err)
			}
			deadline, err := strconv.ParseInt(args[6], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid deadline: %w", err)
			}
			signatures, err := decodeSignatures(args[7:])
			if err != nil {
				return err
			}

			msg := &types.MsgSubmitAttestation{
				Submitter:     clientCtx.GetFromAddress().String(),
				RouteId:       args[0],
				Direction:     string(types.DirectionCronosToXitcoin),
				SourceChainId: args[1],
				SourceRef:     args[2],
				Nonce:         nonce,
				Destination:   args[4],
				Amount:        args[5],
				DeadlineUnix:  deadline,
				Signatures:    signatures,
			}
			if err := msg.ValidateBasic(); err != nil {
				return err
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}

	addBridgeTxFlags(cmd)
	return cmd
}

// NewInitiateOutboundTransferCmd burns native XTC and records a Cronos release request.
func NewInitiateOutboundTransferCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "initiate-outbound ROUTE_ID CRONOS_DESTINATION AMOUNT_ATOMIC",
		Short: "Burn native XTC and initiate a Cronos release request",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			msg := &types.MsgInitiateOutboundTransfer{
				Sender:      clientCtx.GetFromAddress().String(),
				RouteId:     args[0],
				Destination: args[1],
				Amount:      args[2],
			}
			if err := msg.ValidateBasic(); err != nil {
				return err
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}

	addBridgeTxFlags(cmd)
	return cmd
}

func decodeSignatures(values []string) ([][]byte, error) {
	if len(values) < types.RequiredApprovals || len(values) > types.MaxBridgeSigners {
		return nil, fmt.Errorf("expected %d to %d signatures", types.RequiredApprovals, types.MaxBridgeSigners)
	}
	decoded := make([][]byte, len(values))
	for i, value := range values {
		signature, err := hex.DecodeString(strings.TrimPrefix(strings.TrimSpace(value), "0x"))
		if err != nil {
			return nil, fmt.Errorf("invalid signature %d: %w", i+1, err)
		}
		if len(signature) != 65 {
			return nil, fmt.Errorf("invalid signature %d: expected 65 bytes", i+1)
		}
		decoded[i] = signature
	}
	return decoded, nil
}

func decodeSignature(value string) ([]byte, error) {
	signature, err := hex.DecodeString(strings.TrimPrefix(strings.TrimSpace(value), "0x"))
	if err != nil {
		return nil, fmt.Errorf("invalid signature: %w", err)
	}
	if len(signature) != 65 {
		return nil, errors.New("invalid signature: expected 65 bytes")
	}
	return signature, nil
}

func parseControlWindow(nonceValue, notBeforeValue, expiresValue string) (uint64, int64, int64, error) {
	nonce, err := strconv.ParseUint(nonceValue, 10, 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid nonce: %w", err)
	}
	notBefore, err := strconv.ParseInt(notBeforeValue, 10, 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid not-before time: %w", err)
	}
	expires, err := strconv.ParseInt(expiresValue, 10, 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid expiry: %w", err)
	}
	return nonce, notBefore, expires, nil
}

func addBridgeTxFlags(cmd *cobra.Command) {
	flags.AddTxFlagsToCmd(cmd)
	if flag := cmd.Flags().Lookup(flags.FlagFees); flag != nil {
		flag.Usage = "Fees in axtc, for example 1000000000000000axtc"
	}
	if flag := cmd.Flags().Lookup(flags.FlagGasPrices); flag != nil {
		flag.Usage = "Gas prices in axtc, for example 1000000000axtc"
	}
}
