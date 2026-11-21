package types

const (
	ModuleName = "bridge"
	StoreKey   = ModuleName
)

// Store layout. The values are part of the consensus state and are read
// directly by the raw-store CLI queries and by off-chain reconciliation.
var (
	// KeyProcessedAttestationPrefix || SourceDepositKey marks a consumed
	// source deposit (see Attestation.ID).
	KeyProcessedAttestationPrefix = []byte{0x01}
	// KeyRouteConfig holds the JSON-encoded RouteConfig.
	KeyRouteConfig = []byte{0x02}
	// 0x03 is the per-route daily usage prefix (keeper).
	// 0x04 held v1 processed control-action IDs; v2 uses KeyControlNonce.
	// KeyRouteState holds the JSON-encoded route state ({"paused":bool}).
	KeyRouteState = []byte{0x05}
	// 0x06 is the processed guardian-pause prefix (keeper).
	// KeyOutstandingAmount holds the big-endian unsigned outstanding amount.
	KeyOutstandingAmount = []byte{0x07}
	KeyOutboundNonce     = []byte{0x08}
	// KeyControlNonce holds the last applied control nonce (8 bytes, big-endian).
	KeyControlNonce = []byte{0x09}
)
