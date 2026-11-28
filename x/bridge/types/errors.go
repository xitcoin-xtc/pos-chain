package types

import (
	errorsmod "cosmossdk.io/errors"
)

// Registered x/bridge errors. A failed bridge transaction reports the code of
// the error below in its result (codespace "bridge"), so clients can classify
// a failure without parsing the log. The codes are part of the consensus
// results of a block and are stable: never renumber or reuse one, only add
// new codes after the last. Code 1 is reserved for internal errors, which
// keep codespace "undefined". docs/bridge-architecture.md lists the codes.
var (
	// Request validation (stateless).
	ErrEmptyRequest              = errorsmod.Register(ModuleName, 2, "empty bridge message")
	ErrInvalidRouteID            = errorsmod.Register(ModuleName, 3, "invalid route ID")
	ErrNonCanonicalAmount        = errorsmod.Register(ModuleName, 4, "amount must be a canonical positive decimal integer in atomic units")
	ErrAmountOutOfRange          = errorsmod.Register(ModuleName, 5, "amount exceeds the 256-bit range")
	ErrInvalidAttestation        = errorsmod.Register(ModuleName, 6, "invalid bridge attestation")
	ErrInvalidCronosDestination  = errorsmod.Register(ModuleName, 7, "invalid Cronos destination: the vault cannot release to the zero or dead address")
	ErrInvalidXitcoinDestination = errorsmod.Register(ModuleName, 8, "invalid Xitcoin destination")
	ErrInvalidChainID            = errorsmod.Register(ModuleName, 9, "invalid bridge chain ID")
	ErrInvalidRouteConfig        = errorsmod.Register(ModuleName, 10, "invalid bridge route configuration")
	ErrInvalidControlAction      = errorsmod.Register(ModuleName, 11, "invalid bridge control action")
	ErrInvalidGuardianPause      = errorsmod.Register(ModuleName, 12, "invalid guardian pause action")
	ErrInvalidSignature          = errorsmod.Register(ModuleName, 13, "invalid bridge signature")
	ErrInvalidApprovals          = errorsmod.Register(ModuleName, 14, "invalid bridge approvals")

	// Route state.
	ErrRouteDisabled           = errorsmod.Register(ModuleName, 15, "bridge route is disabled")
	ErrRoutePaused             = errorsmod.Register(ModuleName, 16, "bridge route is paused")
	ErrRouteAlreadyPaused      = errorsmod.Register(ModuleName, 17, "bridge route is already paused")
	ErrRouteNotPaused          = errorsmod.Register(ModuleName, 18, "bridge route is not paused")
	ErrRouteAlreadyInitialized = errorsmod.Register(ModuleName, 19, "bridge initial route configuration already exists")

	// Attestations and settlement.
	ErrAttestationRouteMismatch       = errorsmod.Register(ModuleName, 20, "bridge attestation route does not match configuration")
	ErrAttestationSourceChainMismatch = errorsmod.Register(ModuleName, 21, "bridge attestation source chain does not match the route configuration")
	ErrAttestationAlreadyProcessed    = errorsmod.Register(ModuleName, 22, "bridge attestation already processed")
	ErrAttestationExpired             = errorsmod.Register(ModuleName, 23, "bridge attestation expired")
	ErrSettlementUnavailable          = errorsmod.Register(ModuleName, 24, "bridge settlement is unavailable")
	ErrSettlementDirection            = errorsmod.Register(ModuleName, 25, "bridge attestation is not an inbound Cronos transfer")
	ErrBlockedDestination             = errorsmod.Register(ModuleName, 26, "bridge destination is a blocked module account")

	// Limits.
	ErrTransferLimitExceeded    = errorsmod.Register(ModuleName, 27, "bridge transfer limit exceeded")
	ErrDailyLimitExceeded       = errorsmod.Register(ModuleName, 28, "bridge daily limit exceeded")
	ErrOutstandingLimitExceeded = errorsmod.Register(ModuleName, 29, "bridge outstanding limit exceeded")
	ErrInsufficientOutstanding  = errorsmod.Register(ModuleName, 30, "bridge outstanding amount is insufficient")
	ErrMaximumSupplyExceeded    = errorsmod.Register(ModuleName, 31, "XTC maximum supply exceeded")
	ErrCapBelowOutstanding      = errorsmod.Register(ModuleName, 32, "bridge max outstanding amount cannot be set below the current outstanding amount")

	// Control actions and guardian pauses.
	ErrControlActionNotActive     = errorsmod.Register(ModuleName, 33, "bridge control action is not active")
	ErrControlActionProcessed     = errorsmod.Register(ModuleName, 34, "bridge control action already processed")
	ErrControlPayloadMismatch     = errorsmod.Register(ModuleName, 35, "bridge control payload does not match route configuration")
	ErrControlNonceGap            = errorsmod.Register(ModuleName, 36, "bridge control nonce is not the next sequential nonce")
	ErrGuardianPauseRouteMismatch = errorsmod.Register(ModuleName, 37, "guardian pause route does not match configuration")
	ErrGuardianPauseExpired       = errorsmod.Register(ModuleName, 38, "guardian pause action has expired")
	// ErrSignerSetVersionExhausted rejects a signer rotation once the signer
	// set version has reached MaxSignerSetVersion.
	ErrSignerSetVersionExhausted = errorsmod.Register(ModuleName, 39, "bridge signer set version cannot be increased")
)
