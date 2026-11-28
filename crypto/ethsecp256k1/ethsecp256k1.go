package ethsecp256k1

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/subtle"
	"fmt"

	"github.com/ethereum/go-ethereum/crypto"

	"github.com/xitcoin-xtc/pos-chain/ethereum/eip712"

	errorsmod "cosmossdk.io/errors"

	"github.com/cosmos/cosmos-sdk/codec"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	errortypes "github.com/cosmos/cosmos-sdk/types/errors"
)

const (
	// PrivKeySize defines the size of the PrivKey bytes
	PrivKeySize = 32
	// PubKeySize defines the size of the PubKey bytes
	PubKeySize = 33
	// KeyType is the string constant for the Secp256k1 algorithm
	KeyType = "eth_secp256k1"
)

// Amino encoding names
const (
	// PrivKeyName defines the amino encoding name for the EthSecp256k1 private key
	PrivKeyName = "os/PrivKeyEthSecp256k1"
	// PubKeyName defines the amino encoding name for the EthSecp256k1 public key
	PubKeyName = "os/PubKeyEthSecp256k1"
)

// ----------------------------------------------------------------------------
// secp256k1 Private Key

var (
	_ cryptotypes.PrivKey  = &PrivKey{}
	_ codec.AminoMarshaler = &PrivKey{}
)

// GenerateKey generates a new random private key. It returns an error upon
// failure.
func GenerateKey() (*PrivKey, error) {
	priv, err := crypto.GenerateKey()
	if err != nil {
		return nil, err
	}

	return &PrivKey{
		Key: crypto.FromECDSA(priv),
	}, nil
}

// Bytes returns the byte representation of the ECDSA Private Key.
func (privKey PrivKey) Bytes() []byte {
	bz := make([]byte, len(privKey.Key))
	copy(bz, privKey.Key)

	return bz
}

// PubKey returns the ECDSA private key's public key. If the privkey is not valid
// it returns a nil value.
func (privKey PrivKey) PubKey() cryptotypes.PubKey {
	ecdsaPrivKey, err := privKey.ToECDSA()
	if err != nil {
		return nil
	}

	return &PubKey{
		Key: crypto.CompressPubkey(&ecdsaPrivKey.PublicKey),
	}
}

// Equals returns true if two ECDSA private keys are equal and false otherwise.
func (privKey PrivKey) Equals(other cryptotypes.LedgerPrivKey) bool {
	return privKey.Type() == other.Type() && subtle.ConstantTimeCompare(privKey.Bytes(), other.Bytes()) == 1
}

// Type returns eth_secp256k1
func (privKey PrivKey) Type() string {
	return KeyType
}

// MarshalAmino overrides Amino binary marshaling.
func (privKey PrivKey) MarshalAmino() ([]byte, error) {
	return privKey.Key, nil
}

// UnmarshalAmino overrides Amino binary marshaling.
func (privKey *PrivKey) UnmarshalAmino(bz []byte) error {
	if len(bz) != PrivKeySize {
		return fmt.Errorf("invalid privkey size, expected %d got %d", PrivKeySize, len(bz))
	}
	privKey.Key = bz

	return nil
}

// MarshalAminoJSON overrides Amino JSON marshaling.
func (privKey PrivKey) MarshalAminoJSON() ([]byte, error) {
	// When we marshal to Amino JSON, we don't marshal the "key" field itself,
	// just its contents (i.e. the key bytes).
	return privKey.MarshalAmino()
}

// UnmarshalAminoJSON overrides Amino JSON marshaling.
func (privKey *PrivKey) UnmarshalAminoJSON(bz []byte) error {
	return privKey.UnmarshalAmino(bz)
}

// Sign creates a recoverable ECDSA signature on the secp256k1 curve over the
// provided hash of the message. The produced signature is 65 bytes
// where the last byte contains the recovery ID.
func (privKey PrivKey) Sign(digestBz []byte) ([]byte, error) {
	// TODO: remove
	if len(digestBz) != crypto.DigestLength {
		digestBz = crypto.Keccak256Hash(digestBz).Bytes()
	}

	key, err := privKey.ToECDSA()
	if err != nil {
		return nil, err
	}

	return crypto.Sign(digestBz, key)
}

// ToECDSA returns the ECDSA private key as a reference to ecdsa.PrivateKey type.
func (privKey PrivKey) ToECDSA() (*ecdsa.PrivateKey, error) {
	return crypto.ToECDSA(privKey.Bytes())
}

// ----------------------------------------------------------------------------
// secp256k1 Public Key

var (
	_ cryptotypes.PubKey   = &PubKey{}
	_ codec.AminoMarshaler = &PubKey{}
)

// Address returns the address of the ECDSA public key.
// The function will return an empty address if the public key is invalid.
func (pubKey PubKey) Address() cryptotypes.Address {
	pubk, err := crypto.DecompressPubkey(pubKey.Key)
	if err != nil {
		return nil
	}

	return cryptotypes.Address(crypto.PubkeyToAddress(*pubk).Bytes())
}

// Bytes returns the raw bytes of the ECDSA public key.
func (pubKey PubKey) Bytes() []byte {
	bz := make([]byte, len(pubKey.Key))
	copy(bz, pubKey.Key)

	return bz
}

// String implements the fmt.Stringer interface.
func (pubKey PubKey) String() string {
	return fmt.Sprintf("EthPubKeySecp256k1{%X}", pubKey.Key)
}

// Type returns eth_secp256k1
func (pubKey PubKey) Type() string {
	return KeyType
}

// Equals returns true if the pubkey type is the same and their bytes are deeply equal.
func (pubKey PubKey) Equals(other cryptotypes.PubKey) bool {
	return pubKey.Type() == other.Type() && bytes.Equal(pubKey.Bytes(), other.Bytes())
}

// MarshalAmino overrides Amino binary marshaling.
func (pubKey PubKey) MarshalAmino() ([]byte, error) {
	return pubKey.Key, nil
}

// UnmarshalAmino overrides Amino binary marshaling.
func (pubKey *PubKey) UnmarshalAmino(bz []byte) error {
	if len(bz) != PubKeySize {
		return errorsmod.Wrapf(errortypes.ErrInvalidPubKey, "invalid pubkey size, expected %d, got %d", PubKeySize, len(bz))
	}
	pubKey.Key = bz

	return nil
}

// MarshalAminoJSON overrides Amino JSON marshaling.
func (pubKey PubKey) MarshalAminoJSON() ([]byte, error) {
	// When we marshal to Amino JSON, we don't marshal the "key" field itself,
	// just its contents (i.e. the key bytes).
	return pubKey.MarshalAmino()
}

// UnmarshalAminoJSON overrides Amino JSON marshaling.
func (pubKey *PubKey) UnmarshalAminoJSON(bz []byte) error {
	return pubKey.UnmarshalAmino(bz)
}

// Signature encodings accepted by VerifySignature. Every signature is the
// recoverable form R || S || V, where V carries the recovery ID of the signing
// key, so that the same signed content has exactly one accepted encoding:
//
//   - a signature over the sign bytes (SIGN_MODE_DIRECT or
//     SIGN_MODE_LEGACY_AMINO_JSON, as PrivKey.Sign and the keyring produce it)
//     has V in {0, 1};
//   - a signature over the EIP-712 representation of the sign bytes (as
//     eth_signTypedData_v4 and the Ledger Ethereum app produce it) has V in
//     {27, 28}.
//
// S must be in the lower half of the curve order. A 64-byte R || S
// signature, a V of the other path, a V that does not recover this key and
// any other length are rejected: each of them would be a second transaction
// encoding, with a different hash, of a transaction that the signer signed
// once.
const (
	// SignatureLength is the length of an accepted signature: R || S || V.
	SignatureLength = crypto.SignatureLength
	// SignBytesRecoveryOffset is added to the recovery ID in the V byte of a
	// signature over the sign bytes.
	SignBytesRecoveryOffset byte = 0
	// EIP712RecoveryOffset is added to the recovery ID in the V byte of a
	// signature over the EIP-712 representation of the sign bytes.
	EIP712RecoveryOffset byte = 27
)

// ValidateSignatureEncoding reports whether sig has the length and a V byte of
// an encoding VerifySignature can accept. It does not verify the signature:
// VerifySignature also requires V to match the path and the recovery ID.
func ValidateSignatureEncoding(sig []byte) error {
	if len(sig) != SignatureLength {
		return fmt.Errorf(
			"eth_secp256k1 signature must be %d bytes (R || S || V), got %d",
			SignatureLength, len(sig),
		)
	}
	v := sig[crypto.RecoveryIDOffset]
	if !isRecoveryV(v, SignBytesRecoveryOffset) && !isRecoveryV(v, EIP712RecoveryOffset) {
		return fmt.Errorf(
			"eth_secp256k1 signature V must be 0 or 1 (27 or 28 for EIP-712 signatures), got %d", v,
		)
	}
	return nil
}

func isRecoveryV(v, offset byte) bool {
	return v == offset || v == offset+1
}

// VerifySignature verifies that the ECDSA public key created a given signature over
// the provided message. It will calculate the Keccak256 hash of the message
// prior to verification and approve verification if the signature can be verified
// from either the original message or its EIP-712 representation.
//
// CONTRACT: The signature must be 65 bytes [R || S || V], with V in {0, 1}
// over the message and in {27, 28} over its EIP-712 representation, and V
// must be the recovery ID of this key (see SignatureLength).
func (pubKey PubKey) VerifySignature(msg, sig []byte) bool {
	return pubKey.verifySignatureECDSA(msg, sig) || pubKey.verifySignatureAsEIP712(msg, sig)
}

// Verifies the signature as an EIP-712 signature by first converting the message payload
// to EIP-712 object bytes, then performing ECDSA verification on the hash. This is to support
// signing a Cosmos payload using EIP-712.
func (pubKey PubKey) verifySignatureAsEIP712(msg, sig []byte) bool {
	// Rejected before the (costlier) EIP-712 conversion of the message.
	if len(sig) != SignatureLength || !isRecoveryV(sig[crypto.RecoveryIDOffset], EIP712RecoveryOffset) {
		return false
	}

	eip712Bytes, err := eip712.GetEIP712BytesForMsg(msg)
	if err != nil {
		return false
	}

	if pubKey.verifyRecoverable(eip712Bytes, sig, EIP712RecoveryOffset) {
		return true
	}

	// Try verifying the signature using the legacy EIP-712 encoding
	legacyEIP712Bytes, err := eip712.LegacyGetEIP712BytesForMsg(msg)
	if err != nil {
		return false
	}

	return pubKey.verifyRecoverable(legacyEIP712Bytes, sig, EIP712RecoveryOffset)
}

// Perform standard ECDSA signature verification for the given raw bytes and signature.
func (pubKey PubKey) verifySignatureECDSA(msg, sig []byte) bool {
	return pubKey.verifyRecoverable(msg, sig, SignBytesRecoveryOffset)
}

// verifyRecoverable accepts only sig = R || S || V over Keccak256(msg) where
// (R, S) is a low-S signature by this key and V - offset is its recovery ID.
func (pubKey PubKey) verifyRecoverable(msg, sig []byte, offset byte) bool {
	if len(sig) != SignatureLength {
		return false
	}
	v := sig[crypto.RecoveryIDOffset]
	if !isRecoveryV(v, offset) {
		return false
	}

	hash := crypto.Keccak256(msg)
	// (R, S) must be a valid low-S signature by this key, as before.
	if !crypto.VerifySignature(pubKey.Key, hash, sig[:crypto.RecoveryIDOffset]) {
		return false
	}

	// V must be the recovery ID: only one of the two values recovers this key.
	recoverable := make([]byte, SignatureLength)
	copy(recoverable, sig)
	recoverable[crypto.RecoveryIDOffset] = v - offset
	recovered, err := crypto.Ecrecover(hash, recoverable)
	if err != nil {
		return false
	}
	return bytes.Equal(recovered, pubKey.uncompressed())
}

// uncompressed returns the 65-byte uncompressed encoding of the key, which
// crypto.Ecrecover returns, or nil if the key does not parse. Keys are stored
// compressed; crypto.VerifySignature also parses an uncompressed key, so it
// is accepted here too.
func (pubKey PubKey) uncompressed() []byte {
	switch len(pubKey.Key) {
	case PubKeySize:
		key, err := crypto.DecompressPubkey(pubKey.Key)
		if err != nil {
			return nil
		}
		return crypto.FromECDSAPub(key)
	case 65:
		if _, err := crypto.UnmarshalPubkey(pubKey.Key); err != nil {
			return nil
		}
		return pubKey.Key
	default:
		return nil
	}
}
