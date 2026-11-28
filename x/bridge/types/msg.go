package types

import (
	errorsmod "cosmossdk.io/errors"

	"github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

var (
	_ types.Msg = &MsgSubmitAttestation{}
	_ types.Msg = &MsgInitiateOutboundTransfer{}
	_ types.Msg = &MsgInitializeRouteConfig{}
	_ types.Msg = &MsgEmergencyPauseRoute{}
	_ types.Msg = &MsgResumeRoute{}
	_ types.Msg = &MsgUpdateRouteConfig{}
)

// Attestation returns the internal attestation represented by this message.
func (m MsgSubmitAttestation) Attestation() Attestation {
	return Attestation{
		RouteID:       m.RouteId,
		Direction:     Direction(m.Direction),
		SourceChainID: m.SourceChainId,
		SourceRef:     m.SourceRef,
		Nonce:         m.Nonce,
		Destination:   m.Destination,
		Amount:        m.Amount,
		DeadlineUnix:  m.DeadlineUnix,
	}
}

// ValidateBasic validates the submitting account and unsigned message fields.
func (m *MsgSubmitAttestation) ValidateBasic() error {
	if m == nil {
		return errorsmod.Wrap(ErrEmptyRequest, "attestation submission")
	}
	if _, err := types.AccAddressFromBech32(m.Submitter); err != nil {
		return errorsmod.Wrapf(sdkerrors.ErrInvalidAddress, "invalid submitter address: %s", err)
	}
	return m.Attestation().Validate()
}

// GetSignBytes returns legacy sign bytes for compatibility with the SDK.
func (m MsgSubmitAttestation) GetSignBytes() []byte {
	return AminoCdc.MustMarshalJSON(&m)
}

func (m *MsgInitiateOutboundTransfer) ValidateBasic() error {
	if m == nil {
		return errorsmod.Wrap(ErrEmptyRequest, "outbound transfer")
	}
	if _, err := types.AccAddressFromBech32(m.Sender); err != nil {
		return errorsmod.Wrapf(sdkerrors.ErrInvalidAddress, "invalid sender address: %s", err)
	}
	if !validRouteID(m.RouteId) {
		return ErrInvalidRouteID
	}
	if _, err := ValidateCronosDestination(m.Destination); err != nil {
		return err
	}
	if _, err := ParseAmount(m.Amount); err != nil {
		return err
	}
	return nil
}

func (m MsgInitiateOutboundTransfer) GetSignBytes() []byte {
	return AminoCdc.MustMarshalJSON(&m)
}

func (m *MsgInitializeRouteConfig) RouteConfig() RouteConfig {
	return RouteConfig{
		RouteID: m.RouteId, BridgeSigners: m.BridgeSigners, Guardian: m.Guardian,
		MaxTransferAmount: m.MaxTransferAmount, DailyLimit: m.DailyLimit,
		MaxOutstandingAmount: m.MaxOutstandingAmount, Enabled: false,
	}
}

func (m *MsgInitializeRouteConfig) ValidateBasic() error {
	if m == nil {
		return errorsmod.Wrap(ErrEmptyRequest, "initial route configuration")
	}
	if _, err := types.AccAddressFromBech32(m.Authority); err != nil {
		return errorsmod.Wrapf(sdkerrors.ErrInvalidAddress, "invalid authority address: %s", err)
	}
	return m.RouteConfig().Validate()
}

func (m MsgInitializeRouteConfig) GetSignBytes() []byte {
	return AminoCdc.MustMarshalJSON(&m)
}

func (m *MsgEmergencyPauseRoute) Action() GuardianPauseAction {
	return GuardianPauseAction{RouteID: m.RouteId, Nonce: m.Nonce, ExpiresUnix: m.ExpiresUnix}
}

func (m *MsgEmergencyPauseRoute) ValidateBasic() error {
	if m == nil {
		return errorsmod.Wrap(ErrEmptyRequest, "emergency pause")
	}
	if _, err := types.AccAddressFromBech32(m.Submitter); err != nil {
		return errorsmod.Wrapf(sdkerrors.ErrInvalidAddress, "invalid submitter address: %s", err)
	}
	if err := m.Action().Validate(); err != nil {
		return err
	}
	if len(m.GuardianSignature) != 65 {
		return errorsmod.Wrap(ErrInvalidSignature, "guardian signature must be 65 bytes")
	}
	return nil
}

func (m MsgEmergencyPauseRoute) GetSignBytes() []byte { return AminoCdc.MustMarshalJSON(&m) }

func (m *MsgResumeRoute) Action(payloadHash string) ControlAction {
	return ControlAction{RouteID: m.RouteId, Action: ActionResumeRoute, PayloadHash: payloadHash, Nonce: m.Nonce, NotBeforeUnix: m.NotBeforeUnix, ExpiresUnix: m.ExpiresUnix}
}

func (m *MsgResumeRoute) ValidateBasic() error {
	if m == nil {
		return errorsmod.Wrap(ErrEmptyRequest, "route resume")
	}
	if _, err := types.AccAddressFromBech32(m.Submitter); err != nil {
		return errorsmod.Wrapf(sdkerrors.ErrInvalidAddress, "invalid submitter address: %s", err)
	}
	if !validRouteID(m.RouteId) || m.Nonce == 0 || m.NotBeforeUnix <= 0 || m.ExpiresUnix <= m.NotBeforeUnix {
		return errorsmod.Wrap(ErrInvalidControlAction, "invalid route resume action")
	}
	return validateControlSignatures(m.Signatures)
}

func (m MsgResumeRoute) GetSignBytes() []byte { return AminoCdc.MustMarshalJSON(&m) }

func (m *MsgUpdateRouteConfig) RouteConfig() RouteConfig {
	return RouteConfig{RouteID: m.RouteId, BridgeSigners: m.BridgeSigners, Guardian: m.Guardian, MaxTransferAmount: m.MaxTransferAmount, DailyLimit: m.DailyLimit, MaxOutstandingAmount: m.MaxOutstandingAmount, Enabled: m.Enabled}
}

func (m *MsgUpdateRouteConfig) Action(payloadHash string) ControlAction {
	return ControlAction{RouteID: m.RouteId, Action: ActionUpdateRouteConfig, PayloadHash: payloadHash, Nonce: m.Nonce, NotBeforeUnix: m.NotBeforeUnix, ExpiresUnix: m.ExpiresUnix}
}

func (m *MsgUpdateRouteConfig) ValidateBasic() error {
	if m == nil {
		return errorsmod.Wrap(ErrEmptyRequest, "route configuration update")
	}
	if _, err := types.AccAddressFromBech32(m.Submitter); err != nil {
		return errorsmod.Wrapf(sdkerrors.ErrInvalidAddress, "invalid submitter address: %s", err)
	}
	if err := m.RouteConfig().Validate(); err != nil {
		return err
	}
	if m.Nonce == 0 || m.NotBeforeUnix <= 0 || m.ExpiresUnix <= m.NotBeforeUnix {
		return errorsmod.Wrap(ErrInvalidControlAction, "invalid route configuration update action")
	}
	return validateControlSignatures(m.Signatures)
}

func (m MsgUpdateRouteConfig) GetSignBytes() []byte { return AminoCdc.MustMarshalJSON(&m) }

func validateControlSignatures(signatures [][]byte) error {
	if len(signatures) < RequiredApprovals || len(signatures) > MaxBridgeSigners {
		return errorsmod.Wrap(ErrInvalidApprovals, "invalid bridge control signature count")
	}
	for _, signature := range signatures {
		if len(signature) != 65 {
			return errorsmod.Wrap(ErrInvalidSignature, "bridge control signatures must be 65 bytes")
		}
	}
	return nil
}
