package types

import (
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func uint256Max() *big.Int {
	return new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
}

func TestParseAmountAcceptsOnlyCanonicalDecimals(t *testing.T) {
	for _, accepted := range []string{"1", "10", "1000000000000000000", uint256Max().String()} {
		value, err := ParseAmount(accepted)
		if err != nil || value.String() != accepted {
			t.Fatalf("ParseAmount(%q) = %v, %v", accepted, value, err)
		}
	}
	tooLarge := new(big.Int).Add(uint256Max(), big.NewInt(1)).String()
	for _, rejected := range []string{
		"", "0", "00", "010", "0010", "08", "+10", "-10", "+0", " 10", "10 ", "\t10", "10\n", "\x0010",
		"1_000", "0x10", "0X10", "0o10", "0b10", "1e3", "10.0", "\uff11", "\u0661", tooLarge, "0" + uint256Max().String(),
	} {
		if _, err := ParseAmount(rejected); err == nil {
			t.Fatalf("ParseAmount accepted %q", rejected)
		}
	}
	if zero, err := ParseNonNegativeAmount("0"); err != nil || !zero.IsZero() {
		t.Fatalf("ParseNonNegativeAmount(\"0\") = %v, %v", zero, err)
	}
}

func TestRouteIDsAreASCII(t *testing.T) {
	for _, route := range []string{"cronos-é", "route٣", "ｒoute", "Route", "route id", "route\x00"} {
		attestation := validAttestation()
		attestation.RouteID = route
		if err := attestation.Validate(); err == nil {
			t.Fatalf("route ID %q accepted", route)
		}
	}
}

// Before this release the destination was trimmed for validation and for
// the digest, so " x" and "x" shared one signed digest, and a NUL was
// accepted inside the only free-form field of the 0x00-separated payload.
func TestAttestationDestinationAndSourceRefHaveNoWhitespaceVariants(t *testing.T) {
	base := validAttestation()
	for _, mutate := range []func(*Attestation){
		func(a *Attestation) { a.Destination = " " + a.Destination },
		func(a *Attestation) { a.Destination += "\n" },
		func(a *Attestation) { a.Destination = "xitcoin1test\x00destination" },
		func(a *Attestation) { a.Destination = "xitcoin1test destination" },
		func(a *Attestation) { a.SourceRef = " " + a.SourceRef },
		func(a *Attestation) { a.SourceRef += "\t" },
	} {
		variant := base
		mutate(&variant)
		if err := variant.Validate(); err == nil {
			t.Fatalf("attestation %+q accepted", []string{variant.Destination, variant.SourceRef})
		}
	}
	// Case and 0x spellings remain the same deposit, with the same key and digest.
	upper := base
	upper.SourceRef = "0X" + strings.ToUpper(strings.TrimPrefix(base.SourceRef, "0x"))
	baseID, _ := base.ID()
	upperID, err := upper.ID()
	if err != nil || upperID != baseID {
		t.Fatalf("source reference spelling changed the replay key (err %v)", err)
	}
}

// common.HexToAddress does not trim: " 0x..." passed the old validation and
// then resolved to the zero address, silently disabling a signer or the
// guardian.
func TestRouteConfigAddressesMustBeExact(t *testing.T) {
	for _, mutate := range []func(*RouteConfig){
		func(c *RouteConfig) { c.BridgeSigners[0] = " " + c.BridgeSigners[0] },
		func(c *RouteConfig) { c.BridgeSigners[2] += "\n" },
		func(c *RouteConfig) { c.Guardian = "\t" + c.Guardian },
		func(c *RouteConfig) { c.Guardian += " " },
	} {
		config := validRouteConfig()
		config.BridgeSigners = append([]string(nil), config.BridgeSigners...)
		mutate(&config)
		if err := config.Validate(); err == nil {
			t.Fatalf("configuration with padded address accepted: %q %q", config.BridgeSigners, config.Guardian)
		}
	}
	if got := common.HexToAddress(" 0x0000000000000000000000000000000000000001"); got != (common.Address{}) {
		t.Fatalf("premise: padded address resolves to %s", got.Hex())
	}
}

func TestRouteConfigLimitsParseWithTheAmountParser(t *testing.T) {
	for _, limit := range []string{"010", "08", "+1", " 1", "1" + uint256Max().String()} {
		config := validRouteConfig()
		config.MaxOutstandingAmount = limit
		if err := config.Validate(); err == nil {
			t.Fatalf("limit %q accepted", limit)
		}
		if _, err := config.Limits(); err == nil {
			t.Fatalf("Limits accepted %q", limit)
		}
	}
	limits, err := validRouteConfig().Limits()
	if err != nil || limits.MaxOutstandingAmount.String() != validRouteConfig().MaxOutstandingAmount {
		t.Fatalf("Limits = %+v, %v", limits, err)
	}
}

func TestGenesisOutstandingAmountIsCanonical(t *testing.T) {
	config := validRouteConfig()
	for value, valid := range map[string]bool{"": true, "0": true, "5": true, "05": false, "+5": false, "00": false} {
		genesis := GenesisState{RouteConfig: &config, OutstandingAmount: value}
		if err := genesis.Validate(); (err == nil) != valid {
			t.Fatalf("genesis outstanding %q: err %v, want valid=%v", value, err, valid)
		}
	}
}
