package types

// Attack simulations for the bridge primitives, as Go native fuzz targets.
// `go test` runs every target on its seed corpus; `go test -fuzz=FuzzName`
// explores further (see .github/workflows/fuzz.yml). Every property is an
// invariant the chain relies on; a failing input is a security finding.

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	sdkmath "cosmossdk.io/math"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// ---------------------------------------------------------------------------
// Oracles, written independently of the code under test.

var (
	canonicalAmountPattern = regexp.MustCompile(`^[1-9][0-9]*$`)
	maxUint256             = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	routeIDPattern         = regexp.MustCompile(`^[a-z0-9_-]{1,96}$`)
	lowerHex64Pattern      = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// canonicalAmount reports whether s is the decimal form of an integer in
// [1, 2^256-1] that the relayer (^[1-9][0-9]*$) and the Cronos vault
// (uint256) both accept, and returns its value.
func canonicalAmount(s string) (*big.Int, bool) {
	if !canonicalAmountPattern.MatchString(s) {
		return nil, false
	}
	value, ok := new(big.Int).SetString(s, 10)
	if !ok || value.Cmp(maxUint256) > 0 {
		return nil, false
	}
	return value, true
}

// sourceRefSpelling returns the 64-hex lowercase reference that ref spells
// with an optional 0x/0X prefix and any letter case, and nothing else.
func sourceRefSpelling(ref string) (string, bool) {
	normalized := strings.ToLower(ref)
	normalized = strings.TrimPrefix(normalized, "0x")
	return normalized, lowerHex64Pattern.MatchString(normalized) && len(ref) <= 66
}

// documentedSourceDepositKey is the replay key formula of the bridge
// architecture document and of the relayer (protocol.js sourceDepositKey).
func documentedSourceDepositKey(sourceChainID, lowerHex string) [32]byte {
	return sha256.Sum256([]byte(SourceDepositDomain + "\x00" + sourceChainID + "\x00" + lowerHex))
}

func fuzzAttestation() Attestation {
	return Attestation{
		RouteID:       "cronos-testnet-xitcoin-testnet",
		Direction:     DirectionCronosToXitcoin,
		SourceChainID: "338",
		SourceRef:     "0x" + strings.Repeat("ab", 32),
		Nonce:         7,
		Destination:   sdk.AccAddress(bytes.Repeat([]byte{1}, 20)).String(),
		Amount:        "1000",
		DeadlineUnix:  1800003600,
	}
}

func fuzzRouteConfig() RouteConfig {
	return RouteConfig{
		RouteID: "cronos-testnet-xitcoin-testnet",
		BridgeSigners: []string{
			"0x00000000000000000000000000000000000000A1",
			"0x00000000000000000000000000000000000000a2",
			"0x00000000000000000000000000000000000000A3",
		},
		Guardian:             "0x00000000000000000000000000000000000000B4",
		MaxTransferAmount:    "100",
		DailyLimit:           "1000",
		MaxOutstandingAmount: "10000",
		Enabled:              true,
	}
}

var amountSeeds = []string{
	"1", "10", "1000000000000000000", "010", "0010", "+10", "-10", "0", "00", "+0", "08", "09",
	"0x10", "0X10", "0o10", "0b10", "1_000", "1e3", "10.0", " 1", "1 ", "\t1", "1\n", "\x001",
	"\uff11", "\u0661", "", "+", "-", "0x", maxUint256.String(), new(big.Int).Add(maxUint256, big.NewInt(1)).String(),
	"0" + maxUint256.String(), strings.Repeat("9", 79),
}

// ---------------------------------------------------------------------------
// Amount parsing: validation, limits, genesis and settlement read the same
// number, and only canonical decimals are accepted anywhere.

func FuzzParseAmount(f *testing.F) {
	for _, seed := range amountSeeds {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, s string) {
		want, canonical := canonicalAmount(s)
		value, err := ParseAmount(s)
		if (err == nil) != canonical {
			t.Fatalf("ParseAmount(%q) error = %v, canonical = %v", s, err, canonical)
		}
		if err != nil {
			if _, err := ParseNonNegativeAmount(s); (err == nil) != (s == "0") {
				t.Fatalf("ParseNonNegativeAmount(%q) error = %v", s, err)
			}
			return
		}
		if value.String() != s || value.BigInt().Cmp(want) != 0 {
			t.Fatalf("ParseAmount(%q) = %s", s, value)
		}
		// Both historical parsers agree on every accepted input.
		base10, ok10 := new(big.Int).SetString(s, 10)
		base0, ok0 := sdkmath.NewIntFromString(s)
		if !ok10 || !ok0 || base10.Cmp(want) != 0 || base0.BigInt().Cmp(want) != 0 {
			t.Fatalf("parsers disagree on %q: base10=%v base0=%v", s, base10, base0)
		}
		if nonNegative, err := ParseNonNegativeAmount(s); err != nil || !nonNegative.Equal(value) {
			t.Fatalf("ParseNonNegativeAmount(%q) = %v, %v", s, nonNegative, err)
		}
	})
}

func FuzzAmountValidationAgrees(f *testing.F) {
	for _, seed := range amountSeeds {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, s string) {
		_, canonical := canonicalAmount(s)

		outbound := MsgInitiateOutboundTransfer{
			Sender: sdk.AccAddress(bytes.Repeat([]byte{2}, 20)).String(), RouteId: "cronos-testnet-xitcoin-testnet",
			Destination: "0x0000000000000000000000000000000000000001", Amount: s,
		}
		attestation := fuzzAttestation()
		attestation.Amount = s
		withLimit := func(set func(*RouteConfig)) error {
			config := fuzzRouteConfig()
			set(&config)
			return config.Validate()
		}
		bigLimitConfig := fuzzRouteConfig()
		bigLimitConfig.MaxOutstandingAmount = maxUint256.String()
		genesis := GenesisState{RouteConfig: &bigLimitConfig, OutstandingAmount: s}

		checks := []struct {
			name string
			err  error
		}{
			{"MsgInitiateOutboundTransfer.ValidateBasic", outbound.ValidateBasic()},
			{"Attestation.Validate", attestation.Validate()},
			{"RouteConfig.MaxTransferAmount", withLimit(func(c *RouteConfig) { c.MaxTransferAmount = s })},
			{"RouteConfig.DailyLimit", withLimit(func(c *RouteConfig) { c.DailyLimit = s })},
			{"RouteConfig.MaxOutstandingAmount", withLimit(func(c *RouteConfig) { c.MaxOutstandingAmount = s })},
		}
		for _, check := range checks {
			if (check.err == nil) != canonical {
				t.Fatalf("%s accepted=%v for %q, canonical=%v (%v)", check.name, check.err == nil, s, canonical, check.err)
			}
		}
		// An empty outstanding amount means "none" in genesis.
		if err := genesis.Validate(); (err == nil) != (canonical || s == "0" || s == "") {
			t.Fatalf("genesis outstanding %q accepted=%v", s, err == nil)
		}
	})
}

// ---------------------------------------------------------------------------
// Attestation validation and digests.

func FuzzAttestationDigest(f *testing.F) {
	base := fuzzAttestation()
	add := func(route, direction, sourceChain, ref string, nonce uint64, destination, amount string, deadline int64, mutation uint8, position uint16, insert string) {
		f.Add(route, direction, sourceChain, ref, nonce, destination, amount, deadline, mutation, position, insert)
	}
	add(base.RouteID, string(base.Direction), base.SourceChainID, base.SourceRef, base.Nonce, base.Destination, base.Amount, base.DeadlineUnix, 0, 0, " ")
	add(base.RouteID, string(base.Direction), base.SourceChainID, base.SourceRef, base.Nonce, base.Destination, base.Amount, base.DeadlineUnix, 1, 0, "\t")
	add(base.RouteID, string(base.Direction), base.SourceChainID, base.SourceRef, base.Nonce, base.Destination, base.Amount, base.DeadlineUnix, 2, 5, "\x00")
	add(base.RouteID, string(base.Direction), base.SourceChainID, base.SourceRef, base.Nonce, base.Destination, base.Amount, base.DeadlineUnix, 3, 0, "\n")
	add(base.RouteID, string(base.Direction), base.SourceChainID, base.SourceRef, base.Nonce, base.Destination, base.Amount, base.DeadlineUnix, 4, 0, "\u00a0")
	add(base.RouteID, string(base.Direction), base.SourceChainID, base.SourceRef, base.Nonce, base.Destination, base.Amount, base.DeadlineUnix, 5, 0, "")
	add(base.RouteID, string(base.Direction), base.SourceChainID, base.SourceRef, base.Nonce, base.Destination, base.Amount, base.DeadlineUnix, 6, 0, "0")
	add("cronos-\u00e9", string(base.Direction), base.SourceChainID, base.SourceRef, base.Nonce, base.Destination, base.Amount, base.DeadlineUnix, 0, 0, "")
	add("route\u0663", string(base.Direction), base.SourceChainID, base.SourceRef, base.Nonce, base.Destination, base.Amount, base.DeadlineUnix, 0, 0, "")
	add("\uff52\uff4f\uff55\uff54\uff45", string(base.Direction), base.SourceChainID, base.SourceRef, base.Nonce, base.Destination, base.Amount, base.DeadlineUnix, 0, 0, "")
	add(base.RouteID, string(base.Direction), base.SourceChainID, " "+base.SourceRef, base.Nonce, "dest\x00100", "5", 1, 0, 0, "")
	add(base.RouteID, string(base.Direction), base.SourceChainID, base.SourceRef, base.Nonce, base.Destination+"\x00100", "5", 1, 0, 0, "")
	add(base.RouteID, string(DirectionXitcoinToCronos), "25", strings.ToUpper(base.SourceRef), 1, "0x0000000000000000000000000000000000000001", "010", 1, 0, 0, "")

	f.Fuzz(func(t *testing.T, route, direction, sourceChain, ref string, nonce uint64, destination, amount string, deadline int64, mutation uint8, position uint16, insert string) {
		a := Attestation{
			RouteID: route, Direction: Direction(direction), SourceChainID: sourceChain, SourceRef: ref,
			Nonce: nonce, Destination: destination, Amount: amount, DeadlineUnix: deadline,
		}
		validErr := a.Validate()
		digest, digestErr := SigningDigest(a, testChainID)
		again, _ := SigningDigest(a, testChainID)
		if digest != again {
			t.Fatal("SigningDigest is not deterministic")
		}
		if (validErr == nil) != (digestErr == nil) {
			t.Fatalf("Validate error %v but SigningDigest error %v", validErr, digestErr)
		}
		if _, err := a.ID(); (err == nil) != (validErr == nil) {
			t.Fatalf("Validate error %v but ID error %v", validErr, err)
		}
		if validErr != nil {
			return
		}

		// Only ASCII route IDs from [a-z0-9_-] (the relayer's rule).
		if !routeIDPattern.MatchString(a.RouteID) {
			t.Fatalf("route ID %q accepted", a.RouteID)
		}
		// The documented payload is a sequence of 9 fields separated by 0x00
		// and no field contains 0x00, so the payload decodes back into
		// exactly the canonical fields: the encoding is injective.
		ref64, ok := sourceRefSpelling(a.SourceRef)
		if !ok {
			t.Fatalf("source reference %q accepted", a.SourceRef)
		}
		fields := []string{
			testChainID, a.RouteID, string(a.Direction), a.SourceChainID, ref64,
			fmt.Sprintf("%d", a.Nonce), a.Destination, a.Amount, fmt.Sprintf("%d", a.DeadlineUnix),
		}
		preimage := strings.Join(fields, "\x00")
		payloadHash, err := a.PayloadHash(testChainID)
		if err != nil || payloadHash != sha256.Sum256([]byte(preimage)) {
			t.Fatalf("payload hash does not follow the documented encoding (err %v)", err)
		}
		if decoded := strings.Split(preimage, "\x00"); len(decoded) != len(fields) {
			t.Fatalf("payload of %+v has %d fields, want %d", a, len(decoded), len(fields))
		}
		if domain, _ := AttestationDomain(testChainID); digest != crypto.Keccak256Hash([]byte(domain), payloadHash[:]) {
			t.Fatal("signing digest does not follow the documented domain separation")
		}

		// A whitespace, NUL or spelling variant of a field is either rejected
		// or yields a different digest, unless it is the same attestation
		// (source reference case and 0x prefix are one deposit).
		b := mutateAttestation(a, mutation, int(position), insert)
		otherDigest, err := SigningDigest(b, testChainID)
		if err != nil || otherDigest != digest {
			return
		}
		if canonicalAttestation(t, a) != canonicalAttestation(t, b) {
			t.Fatalf("different attestations share a digest:\n%#v\n%#v", a, b)
		}
	})
}

func canonicalAttestation(t *testing.T, a Attestation) Attestation {
	t.Helper()
	ref, ok := sourceRefSpelling(a.SourceRef)
	if !ok {
		t.Fatalf("source reference %q accepted", a.SourceRef)
	}
	a.SourceRef = ref
	return a
}

func insertAt(s string, position int, insert string) string {
	if len(s) == 0 {
		return insert
	}
	position %= len(s) + 1
	return s[:position] + insert + s[position:]
}

func mutateAttestation(a Attestation, mutation uint8, position int, insert string) Attestation {
	switch mutation % 8 {
	case 0:
		a.Destination += insert
	case 1:
		a.Destination = insert + a.Destination
	case 2:
		a.Destination = insertAt(a.Destination, position, insert)
	case 3:
		a.SourceRef += insert
	case 4:
		a.SourceRef = insert + a.SourceRef
	case 5:
		a.SourceRef = strings.ToUpper(a.SourceRef)
	case 6:
		a.Amount = insert + a.Amount
	case 7:
		a.RouteID = insertAt(a.RouteID, position, insert)
	}
	return a
}

// ---------------------------------------------------------------------------
// Source deposit key: case, 0x and whitespace spellings of one deposit map to
// one replay key or are rejected.

var whitespaceSpellings = []string{"", " ", "\t", "\n", "\r", "\v", "\f", "\u0085", "\u00a0", "\u2003", "\u3000", "\x00", "\ufeff"}

func FuzzSourceDepositKey(f *testing.F) {
	f.Add("338", bytes.Repeat([]byte{0xab}, 32), uint64(0), uint8(0), uint8(0))
	f.Add("338", bytes.Repeat([]byte{0xcd}, 32), ^uint64(0), uint8(1), uint8(1))
	f.Add("25", []byte("deposit"), uint64(0x5555), uint8(2), uint8(14))
	f.Add("338", bytes.Repeat([]byte{0xef}, 32), uint64(1), uint8(1), uint8(8))
	f.Add("0338", bytes.Repeat([]byte{0x01}, 32), uint64(0), uint8(0), uint8(0))
	f.Fuzz(func(t *testing.T, sourceChainID string, raw []byte, caseMask uint64, prefix, padding uint8) {
		var deposit [32]byte
		copy(deposit[:], raw)
		lower := hex.EncodeToString(deposit[:])
		want := documentedSourceDepositKey(sourceChainID, lower)

		canonicalKey, err := SourceDepositKey(sourceChainID, lower)
		if (err == nil) != (ValidateEVMChainID(sourceChainID) == nil) {
			t.Fatalf("canonical reference with chain %q: %v", sourceChainID, err)
		}
		if err != nil {
			return
		}
		if canonicalKey != want {
			t.Fatal("replay key does not follow the documented formula")
		}

		spelled := []byte(lower)
		for i := range spelled {
			if caseMask&(1<<(uint(i)%64)) != 0 && spelled[i] >= 'a' {
				spelled[i] -= 'a' - 'A'
			}
		}
		variant := []string{"", "0x", "0X"}[prefix%3] + string(spelled)
		pad := whitespaceSpellings[int(padding)%len(whitespaceSpellings)]
		switch padding / 16 % 3 {
		case 0:
			variant = pad + variant
		case 1:
			variant += pad
		default:
			variant = pad + variant + pad
		}
		key, err := SourceDepositKey(sourceChainID, variant)
		if err == nil && key != want {
			t.Fatalf("spelling %q of one deposit maps to another replay key", variant)
		}
		attestation := fuzzAttestation()
		attestation.SourceChainID = sourceChainID
		attestation.SourceRef = variant
		if id, err := attestation.ID(); err == nil && id != want {
			t.Fatalf("attestation with source reference %q has another replay key", variant)
		}
	})
}

func FuzzSourceDepositKeyArbitrary(f *testing.F) {
	f.Add("338", "0x"+strings.Repeat("ab", 32))
	f.Add("338", " 0X"+strings.Repeat("AB", 32)+"\n")
	f.Add("338", "0x0x"+strings.Repeat("ab", 31))
	f.Add("338", strings.Repeat("\u212a", 64))
	f.Fuzz(func(t *testing.T, sourceChainID, ref string) {
		key, err := SourceDepositKey(sourceChainID, ref)
		if err != nil {
			return
		}
		// An accepted reference is a spelling of exactly one 32-byte value
		// (whitespace is tolerated here only if the implementation chose to
		// map it to the same key), and the key commits to that value.
		trimmed := strings.TrimSpace(ref)
		lower, ok := sourceRefSpelling(trimmed)
		if !ok {
			t.Fatalf("reference %q accepted", ref)
		}
		if key != documentedSourceDepositKey(sourceChainID, lower) {
			t.Fatalf("reference %q maps to an undocumented key", ref)
		}
	})
}

// ---------------------------------------------------------------------------
// Approvals: only distinct configured signers, with a low-s signature over
// this network's digest of this domain, count toward 2-of-3.

type signatureFixture struct {
	keys          []*ecdsa.PrivateKey // 0-2 bridge signers, 3 outsider, 4 guardian
	signers       []string
	guardian      string
	attestation   Attestation
	control       ControlAction
	pause         GuardianPauseAction
	digests       []common.Hash
	signatures    [][][]byte // [key][digest]
	attestationAt int
	controlAt     int
	pauseAt       int
}

var secp256k1N = crypto.S256().Params().N

func newSignatureFixture(tb testing.TB) *signatureFixture {
	tb.Helper()
	fx := &signatureFixture{
		attestation: fuzzAttestation(),
		control: ControlAction{
			RouteID: "cronos-testnet-xitcoin-testnet", Action: ActionResumeRoute,
			PayloadHash: "0x" + strings.Repeat("11", 32), Nonce: 1, NotBeforeUnix: 1800000000, ExpiresUnix: 1800003600,
		},
		pause: GuardianPauseAction{RouteID: "cronos-testnet-xitcoin-testnet", Nonce: 1, ExpiresUnix: 1800003600},
	}
	for i := 0; i < 5; i++ {
		seed := sha256.Sum256([]byte(fmt.Sprintf("bridge-fuzz-key-%d", i)))
		key, err := crypto.ToECDSA(seed[:])
		if err != nil {
			tb.Fatal(err)
		}
		fx.keys = append(fx.keys, key)
	}
	for _, key := range fx.keys[:3] {
		fx.signers = append(fx.signers, crypto.PubkeyToAddress(key.PublicKey).Hex())
	}
	fx.guardian = crypto.PubkeyToAddress(fx.keys[4].PublicKey).Hex()

	must := func(hash common.Hash, err error) common.Hash {
		if err != nil {
			tb.Fatal(err)
		}
		return hash
	}
	payloadHash, err := fx.attestation.PayloadHash(testChainID)
	if err != nil {
		tb.Fatal(err)
	}
	controlID, err := fx.control.ID(testChainID)
	if err != nil {
		tb.Fatal(err)
	}
	fx.digests = []common.Hash{
		must(SigningDigest(fx.attestation, testChainID)),                                           // 0
		must(SigningDigest(fx.attestation, MainnetChainID)),                                        // 1 other network
		must(ControlDigest(fx.control, testChainID)),                                               // 2
		must(ControlDigest(fx.control, MainnetChainID)),                                            // 3 other network
		must(GuardianPauseDigest(fx.pause, testChainID)),                                           // 4
		must(GuardianPauseDigest(fx.pause, MainnetChainID)),                                        // 5 other network
		crypto.Keccak256Hash([]byte("xitcoin-bridge-testnet-attestation-v1"), payloadHash[:]),      // 6 retired v1 domain
		crypto.Keccak256Hash([]byte(ControlDomainPrefix+"/"+testChainID), controlID[:], []byte{0}), // 7 unrelated
	}
	fx.attestationAt, fx.controlAt, fx.pauseAt = 0, 2, 4
	for _, key := range fx.keys {
		row := make([][]byte, len(fx.digests))
		for j, digest := range fx.digests {
			signature, err := crypto.Sign(digest.Bytes(), key)
			if err != nil {
				tb.Fatal(err)
			}
			row[j] = signature
		}
		fx.signatures = append(fx.signatures, row)
	}
	return fx
}

// encodeSignature derives one presented signature and whether it must count.
// Validity is decided from first principles: right digest, low s, recovery
// byte equal to the recovery ID or recovery ID + 27, intact r and s.
func (fx *signatureFixture) encodeSignature(keyIndex, digestIndex int, mode, arg byte) ([]byte, bool) {
	signature := append([]byte(nil), fx.signatures[keyIndex][digestIndex]...)
	recovery := signature[64]
	switch mode % 7 {
	case 0: // recovery ID 0/1
		return signature, true
	case 1: // Ethereum 27/28
		signature[64] += 27
		return signature, true
	case 2: // arbitrary recovery byte
		signature[64] = arg
		return signature, arg == recovery || arg == recovery+27
	case 3: // wrong recovery ID: recovers another key
		signature[64] = (recovery ^ 1) + []byte{0, 27}[arg%2]
		return signature, false
	case 4: // high-s twin: recovers the same key, must not count
		s := new(big.Int).SetBytes(signature[32:64])
		s.Sub(secp256k1N, s)
		s.FillBytes(signature[32:64])
		signature[64] = (recovery ^ 1) + []byte{0, 27}[arg%2]
		return signature, false
	case 5: // corrupted r or s
		signature[int(arg)%64] ^= 0x01 | arg
		return signature, false
	default: // wrong length
		if arg%2 == 0 {
			return signature[:64], false
		}
		return append(signature, 0), false
	}
}

func FuzzApprovals(f *testing.F) {
	fx := newSignatureFixture(f)
	f.Add([]byte{0, 2, 0, 0, 0, 0, 1, 0, 0, 0})              // two valid attestation approvals
	f.Add([]byte{0, 2, 0, 0, 0, 0, 0, 0, 1, 0})              // same signer, v and v+27
	f.Add([]byte{0, 2, 0, 0, 4, 0, 1, 0, 0, 0})              // high-s twin
	f.Add([]byte{0, 2, 0, 0, 0, 0, 3, 0, 0, 0})              // outsider
	f.Add([]byte{0, 2, 0, 1, 0, 0, 1, 1, 0, 0})              // other network
	f.Add([]byte{0, 2, 0, 2, 0, 0, 1, 2, 0, 0})              // control-domain signatures
	f.Add([]byte{1, 2, 0, 2, 0, 0, 1, 2, 1, 0})              // two valid control approvals
	f.Add([]byte{1, 3, 0, 2, 0, 0, 1, 2, 1, 0, 2, 2, 2, 28}) // three, one with v=28
	f.Add([]byte{1, 2, 0, 0, 0, 0, 1, 0, 0, 0})              // attestation signatures for a control action
	f.Add([]byte{2, 1, 4, 4, 0, 0})                          // guardian pause
	f.Add([]byte{2, 1, 4, 5, 0, 0})                          // guardian pause, other network
	f.Add([]byte{2, 1, 0, 4, 0, 0})                          // bridge signer as guardian
	f.Add([]byte{2, 1, 4, 4, 4, 1})                          // guardian high-s
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 2 {
			return
		}
		verifier, count := data[0]%3, int(data[1]%5)
		data = data[2:]
		signatures := make([][]byte, 0, count)
		keys := make([]int, 0, count)
		valid := make([]bool, 0, count)
		for i := 0; i < count; i++ {
			var b [4]byte
			copy(b[:], data)
			if len(data) >= 4 {
				data = data[4:]
			} else {
				data = nil
			}
			keyIndex, digestIndex := int(b[0])%len(fx.keys), int(b[1])%len(fx.digests)
			signature, encodingValid := fx.encodeSignature(keyIndex, digestIndex, b[2], b[3])
			target, allowed := fx.attestationAt, keyIndex < 3
			switch verifier {
			case 1:
				target = fx.controlAt
			case 2:
				target, allowed = fx.pauseAt, keyIndex == 4
			}
			signatures = append(signatures, signature)
			keys = append(keys, keyIndex)
			valid = append(valid, encodingValid && digestIndex == target && allowed)
		}
		allValid, distinct := true, map[int]bool{}
		for i, key := range keys {
			allValid = allValid && valid[i]
			distinct[key] = true
		}

		switch verifier {
		case 0, 1:
			want := count >= RequiredApprovals && count <= MaxBridgeSigners && allValid && len(distinct) == count
			var recovered []common.Address
			var err error
			if verifier == 0 {
				recovered, err = VerifyApprovals(fx.attestation, testChainID, fx.signers, signatures)
			} else {
				recovered, err = VerifyControlApprovals(fx.control, testChainID, fx.signers, signatures)
			}
			if (err == nil) != want {
				t.Fatalf("verifier %d accepted=%v want %v (keys %v, valid %v, err %v)", verifier, err == nil, want, keys, valid, err)
			}
			if err == nil {
				for i, address := range recovered {
					if address != crypto.PubkeyToAddress(fx.keys[keys[i]].PublicKey) {
						t.Fatalf("approval %d attributed to %s", i, address.Hex())
					}
				}
			}
		case 2:
			// A guardian pause carries exactly one signature.
			signature, want := []byte{}, false
			if count > 0 {
				signature, want = signatures[0], valid[0]
			}
			address, err := VerifyGuardianPause(fx.pause, testChainID, fx.guardian, signature)
			if (err == nil) != want {
				t.Fatalf("guardian check accepted=%v want %v (err %v)", err == nil, want, err)
			}
			if err == nil && address != common.HexToAddress(fx.guardian) {
				t.Fatal("pause attributed to another address")
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Route configuration: every accepted configuration parses with the amount
// parser and names exactly the addresses it spells.

func FuzzRouteConfigValidate(f *testing.F) {
	c := fuzzRouteConfig()
	f.Add(c.RouteID, c.BridgeSigners[0], c.BridgeSigners[1], c.BridgeSigners[2], c.Guardian, c.MaxTransferAmount, c.DailyLimit, c.MaxOutstandingAmount, "")
	f.Add(c.RouteID, " "+c.BridgeSigners[0], c.BridgeSigners[1], c.BridgeSigners[2], c.Guardian, c.MaxTransferAmount, c.DailyLimit, c.MaxOutstandingAmount, "338")
	f.Add(c.RouteID, c.BridgeSigners[0], c.BridgeSigners[1], c.BridgeSigners[2], c.Guardian+"\n", c.MaxTransferAmount, c.DailyLimit, c.MaxOutstandingAmount, "25")
	f.Add(c.RouteID, c.BridgeSigners[0], c.BridgeSigners[1], c.BridgeSigners[2], c.Guardian, "010", "08", "+1", "")
	f.Add(c.RouteID, c.BridgeSigners[0], c.BridgeSigners[1], c.BridgeSigners[2], c.Guardian, maxUint256.String(), "1"+maxUint256.String(), "1", "")
	f.Add("route-\u00e9", strings.ToLower(c.BridgeSigners[0]), "00000000000000000000000000000000000000a2", c.BridgeSigners[2], "0X00000000000000000000000000000000000000B4", "1", "1", "1", "")
	f.Fuzz(func(t *testing.T, route, signer0, signer1, signer2, guardian, maxTransfer, daily, maxOutstanding, sourceChain string) {
		config := RouteConfig{
			RouteID: route, BridgeSigners: []string{signer0, signer1, signer2}, Guardian: guardian,
			MaxTransferAmount: maxTransfer, DailyLimit: daily, MaxOutstandingAmount: maxOutstanding,
			Enabled: true, SourceChainID: sourceChain,
		}
		err := config.Validate()
		if _, hashErr := RouteConfigPayloadHash(config); (hashErr == nil) != (err == nil) {
			t.Fatalf("Validate error %v but payload hash error %v", err, hashErr)
		}
		if err != nil {
			return
		}
		if !routeIDPattern.MatchString(route) {
			t.Fatalf("route ID %q accepted", route)
		}
		if _, err := config.Limits(); err != nil {
			t.Fatalf("accepted configuration has an unparsable limit: %v", err)
		}
		for _, value := range []string{maxTransfer, daily, maxOutstanding} {
			if _, ok := canonicalAmount(value); !ok {
				t.Fatalf("limit %q accepted", value)
			}
		}
		for _, address := range []string{signer0, signer1, signer2, guardian} {
			spelled := strings.ToLower(strings.TrimPrefix(strings.TrimPrefix(address, "0x"), "0X"))
			if !strings.EqualFold(hex.EncodeToString(common.HexToAddress(address).Bytes()), spelled) {
				t.Fatalf("configured address %q resolves to %s", address, common.HexToAddress(address).Hex())
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Message decoding: arbitrary bytes never make decoding, validation, sign
// bytes or digest computation panic.

func FuzzMessageDecoding(f *testing.F) {
	registry := codectypes.NewInterfaceRegistry()
	RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)

	submit := MsgSubmitAttestation{
		Submitter: sdk.AccAddress(make([]byte, 20)).String(), RouteId: "cronos-testnet-xitcoin-testnet",
		Direction: string(DirectionCronosToXitcoin), SourceChainId: "338", SourceRef: "0x" + strings.Repeat("a", 64),
		Nonce: 1, Destination: sdk.AccAddress(bytes.Repeat([]byte{1}, 20)).String(), Amount: "10", DeadlineUnix: 1800003600,
		Signatures: [][]byte{make([]byte, 65), make([]byte, 65)},
	}
	c := fuzzRouteConfig()
	seeds := []sdk.Msg{
		&submit,
		&MsgInitiateOutboundTransfer{Sender: submit.Submitter, RouteId: submit.RouteId, Destination: "0x0000000000000000000000000000000000000001", Amount: "010"},
		&MsgInitializeRouteConfig{Authority: submit.Submitter, RouteId: c.RouteID, BridgeSigners: c.BridgeSigners, Guardian: c.Guardian, MaxTransferAmount: "1", DailyLimit: "2", MaxOutstandingAmount: "3"},
		&MsgEmergencyPauseRoute{Submitter: submit.Submitter, RouteId: c.RouteID, Nonce: 1, ExpiresUnix: 2, GuardianSignature: make([]byte, 65)},
		&MsgResumeRoute{Submitter: submit.Submitter, RouteId: c.RouteID, Nonce: 1, NotBeforeUnix: 1, ExpiresUnix: 2, Signatures: [][]byte{make([]byte, 65), make([]byte, 65)}},
		&MsgUpdateRouteConfig{Submitter: submit.Submitter, RouteId: c.RouteID, BridgeSigners: c.BridgeSigners, Guardian: c.Guardian, MaxTransferAmount: "1", DailyLimit: "2", MaxOutstandingAmount: "3", Nonce: 1, NotBeforeUnix: 1, ExpiresUnix: 2, Signatures: [][]byte{make([]byte, 65), make([]byte, 65)}},
	}
	for _, msg := range seeds {
		raw, err := cdc.Marshal(msg)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(raw)
		if anyRaw, err := cdc.MarshalInterface(msg); err == nil {
			f.Add(anyRaw)
		}
	}
	genesis, _ := json.Marshal(GenesisState{RouteConfig: &c, OutstandingAmount: "010", ProcessedSourceDeposits: []string{"0x" + strings.Repeat("0", 64)}})
	f.Add(genesis)
	f.Add([]byte(`{"route_config":{"route_id":"r","max_transfer_amount":"08"},"outstanding_amount":"+1"}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		for _, msg := range []interface {
			sdk.Msg
			Unmarshal([]byte) error
			ValidateBasic() error
		}{
			&MsgSubmitAttestation{}, &MsgInitiateOutboundTransfer{}, &MsgInitializeRouteConfig{},
			&MsgEmergencyPauseRoute{}, &MsgResumeRoute{}, &MsgUpdateRouteConfig{},
		} {
			if err := msg.Unmarshal(data); err != nil {
				continue
			}
			_ = msg.ValidateBasic()
			switch m := msg.(type) {
			case *MsgSubmitAttestation:
				_ = m.GetSignBytes()
				_, _ = m.Attestation().ID()
				_, _ = SigningDigest(m.Attestation(), testChainID)
			case *MsgInitiateOutboundTransfer:
				_ = m.GetSignBytes()
				_, _ = ParseAmount(m.Amount)
				_, _ = ValidateCronosDestination(m.Destination)
			case *MsgInitializeRouteConfig:
				_ = m.GetSignBytes()
				_, _ = m.RouteConfig().Limits()
			case *MsgEmergencyPauseRoute:
				_ = m.GetSignBytes()
				_, _ = GuardianPauseDigest(m.Action(), testChainID)
			case *MsgResumeRoute:
				_ = m.GetSignBytes()
				_, _ = ControlDigest(m.Action("0x"+strings.Repeat("22", 32)), testChainID)
			case *MsgUpdateRouteConfig:
				_ = m.GetSignBytes()
				_, _ = RouteConfigPayloadHash(m.RouteConfig())
				_, _ = ControlDigest(m.Action("0x"+strings.Repeat("22", 32)), testChainID)
			}
		}
		// The Any route of transaction decoding. (ProtoCodec.UnmarshalInterface
		// adds unknown-field rejection, whose descriptor caches make coverage
		// non-deterministic and stall the fuzzing engine; it is not bridge code.)
		var packed codectypes.Any
		if err := packed.Unmarshal(data); err == nil {
			var msg sdk.Msg
			if err := registry.UnpackAny(&packed, &msg); err == nil {
				if validator, ok := msg.(sdk.HasValidateBasic); ok {
					_ = validator.ValidateBasic()
				}
			}
		}
		var genesis GenesisState
		if err := json.Unmarshal(data, &genesis); err == nil {
			_ = genesis.Validate()
		}
		var config RouteConfig
		if err := json.Unmarshal(data, &config); err == nil {
			_ = config.Validate()
			_, _ = config.Limits()
		}
	})
}
