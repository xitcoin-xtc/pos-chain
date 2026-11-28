package types

import (
	"strings"

	"github.com/ethereum/go-ethereum/common"
)

// CronosDeadAddress is refused as a release recipient by CronosBridgeVault
// (_isValidRecipient), together with the zero address.
var CronosDeadAddress = common.HexToAddress("0x000000000000000000000000000000000000dEaD")

// ValidateCronosDestination accepts exactly the recipients that
// CronosBridgeVault.release accepts, so the chain never burns native XTC for
// a release the vault is guaranteed to refuse.
func ValidateCronosDestination(destination string) (common.Address, error) {
	destination = strings.TrimSpace(destination)
	if !common.IsHexAddress(destination) {
		return common.Address{}, ErrInvalidCronosDestination
	}
	address := common.HexToAddress(destination)
	if address == (common.Address{}) || address == CronosDeadAddress {
		return common.Address{}, ErrInvalidCronosDestination
	}
	return address, nil
}
