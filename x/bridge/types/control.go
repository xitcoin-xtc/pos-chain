package types

import (
	"crypto/sha256"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	errorsmod "cosmossdk.io/errors"
)

// Control actions with an on-chain handler. Pausing is a guardian action
// (GuardianPauseAction), not a ControlAction, and there is no close-route
// action: disabling a route is an update_route_config with enabled=false.
const (
	ActionUpdateRouteConfig = "update_route_config"
	ActionResumeRoute       = "resume_route"
)

// ControlAction describes a route-governance action approved by two current
// bridge signers. PayloadHash binds an action to its exact, separately
// canonicalised payload. Nonce must be exactly the last applied control nonce
// plus one (see keeper.ControlNonce), so control actions apply in order and an
// older signed action can never be applied after a newer one. This primitive
// has no token, bank, reserve, minting, relayer, or transaction capability.
type ControlAction struct {
	RouteID       string `json:"route_id"`
	Action        string `json:"action"`
	PayloadHash   string `json:"payload_hash"`
	Nonce         uint64 `json:"nonce"`
	NotBeforeUnix int64  `json:"not_before_unix"`
	ExpiresUnix   int64  `json:"expires_unix"`
}

func (a ControlAction) Validate() error {
	if !validRouteID(a.RouteID) {
		return ErrInvalidRouteID
	}
	switch a.Action {
	case ActionUpdateRouteConfig, ActionResumeRoute:
	default:
		return ErrInvalidControlAction
	}
	if !common.IsHexHash(strings.TrimSpace(a.PayloadHash)) {
		return errorsmod.Wrap(ErrInvalidControlAction, "invalid bridge control payload hash")
	}
	if a.Nonce == 0 {
		return errorsmod.Wrap(ErrInvalidControlAction, "bridge control nonce must be positive")
	}
	if a.NotBeforeUnix <= 0 || a.ExpiresUnix <= a.NotBeforeUnix {
		return errorsmod.Wrap(ErrInvalidControlAction, "invalid bridge control validity window")
	}
	return nil
}

// ID is deterministic and contains every signed governance field and the
// network-specific control domain of chainID.
func (a ControlAction) ID(chainID string) ([32]byte, error) {
	if err := a.Validate(); err != nil {
		return [32]byte{}, err
	}
	domain, err := ControlDomain(chainID)
	if err != nil {
		return [32]byte{}, err
	}
	payload := strings.Join([]string{
		domain,
		a.RouteID,
		a.Action,
		strings.ToLower(strings.TrimSpace(a.PayloadHash)),
		strconv.FormatUint(a.Nonce, 10),
		strconv.FormatInt(a.NotBeforeUnix, 10),
		strconv.FormatInt(a.ExpiresUnix, 10),
	}, "\x00")
	return sha256.Sum256([]byte(payload)), nil
}

// ControlDigest is keccak256(ControlDomain(chainID) || ID(chainID)).
func ControlDigest(action ControlAction, chainID string) (common.Hash, error) {
	id, err := action.ID(chainID)
	if err != nil {
		return common.Hash{}, err
	}
	domain, err := ControlDomain(chainID)
	if err != nil {
		return common.Hash{}, err
	}
	return crypto.Keccak256Hash([]byte(domain), id[:]), nil
}

// VerifyControlApprovals requires two distinct current bridge signers. It does
// not apply the action; the keeper applies it after nonce and payload checks.
func VerifyControlApprovals(action ControlAction, chainID string, bridgeSigners []string, signatures [][]byte) ([]common.Address, error) {
	allowed, err := configuredSigners(bridgeSigners)
	if err != nil {
		return nil, err
	}
	if len(signatures) < RequiredApprovals || len(signatures) > MaxBridgeSigners {
		return nil, errorsmod.Wrapf(ErrInvalidApprovals, "signature count must be between %d and %d", RequiredApprovals, MaxBridgeSigners)
	}
	digest, err := ControlDigest(action, chainID)
	if err != nil {
		return nil, err
	}

	recovered := make([]common.Address, 0, len(signatures))
	seen := make(map[common.Address]struct{}, len(signatures))
	for _, signature := range signatures {
		address, err := recoverSigner(digest, signature)
		if err != nil {
			return nil, err
		}
		if _, ok := allowed[address]; !ok {
			return nil, errorsmod.Wrap(ErrInvalidApprovals, "approval is not from a configured bridge signer")
		}
		if _, exists := seen[address]; exists {
			return nil, errorsmod.Wrap(ErrInvalidApprovals, "duplicate signer approval")
		}
		seen[address] = struct{}{}
		recovered = append(recovered, address)
	}
	return recovered, nil
}
