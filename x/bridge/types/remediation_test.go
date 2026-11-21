package types

import (
	"encoding/hex"
	"strings"
	"testing"
)

// crossCheckVectors are shared with the relayer (bridge-relayer
// test/fixtures/attestation-cross-check.json and
// scripts/fixtures/bridge-process.mjs). Both implementations must produce
// these exact values; a change on one side only fails both test suites.
var crossCheckVectors = []struct {
	chainID       string
	sourceChainID string
	id            string
	payloadHash   string
	digest        string
}{
	{
		chainID:       "xitcoin-testnet-v2-1",
		sourceChainID: "338",
		id:            "0x60e1081b4354c658cfe771da1042e6122798577367f118f0cc589b081a535566",
		payloadHash:   "0x9eeac63a8ad1fe3b14f939b15cd13dd703bc4634b735defe5fb2725e64098bef",
		digest:        "0x0fc85111099dd06f0456377e4800671e35d9cd994f22f43f151c69fbe163269a",
	},
	{
		chainID:       "xitcoin",
		sourceChainID: "25",
		id:            "0xb179741ff999179919b2a553db338af2281def1f3a70f990cd0440d91200c2c0",
		payloadHash:   "0x658de8cb0f7210e77fb416e513c5c1b91a409a8f1e27ee3f709f4d1901a244a6",
		digest:        "0x95438a631fe0d6dd3440055d89c0e8abdf042ed80999d6c2ce48bd2ec518f30f",
	},
}

func crossCheckAttestation(sourceChainID string) Attestation {
	return Attestation{
		RouteID:       "cronos-testnet-xitcoin-testnet",
		Direction:     DirectionCronosToXitcoin,
		SourceChainID: sourceChainID,
		SourceRef:     "0x" + strings.Repeat("aa", 32),
		Nonce:         7,
		Destination:   "xtc1zyg3zyg3zyg3zyg3zyg3zyg3zyg3zyg32rdvg9",
		Amount:        "1000000000000000000",
		DeadlineUnix:  1800000000,
	}
}

func TestAttestationCrossCheckVectorsMatchRelayer(t *testing.T) {
	for _, vector := range crossCheckVectors {
		attestation := crossCheckAttestation(vector.sourceChainID)
		id, err := attestation.ID()
		if err != nil {
			t.Fatal(err)
		}
		if got := "0x" + hex.EncodeToString(id[:]); got != vector.id {
			t.Fatalf("%s replay key = %s, want %s", vector.chainID, got, vector.id)
		}
		payloadHash, err := attestation.PayloadHash(vector.chainID)
		if err != nil {
			t.Fatal(err)
		}
		if got := "0x" + hex.EncodeToString(payloadHash[:]); got != vector.payloadHash {
			t.Fatalf("%s payload hash = %s, want %s", vector.chainID, got, vector.payloadHash)
		}
		digest, err := SigningDigest(attestation, vector.chainID)
		if err != nil {
			t.Fatal(err)
		}
		if got := digest.Hex(); got != vector.digest {
			t.Fatalf("%s digest = %s, want %s", vector.chainID, got, vector.digest)
		}
	}
}

func TestReplayKeyIdentifiesOnlyTheSourceDeposit(t *testing.T) {
	base := validAttestation()
	baseID, err := base.ID()
	if err != nil {
		t.Fatal(err)
	}
	mutations := []func(*Attestation){
		func(a *Attestation) { a.DeadlineUnix++ },
		func(a *Attestation) { a.Destination = "xitcoin1otherdestination" },
		func(a *Attestation) { a.Amount = "2" },
		func(a *Attestation) { a.Nonce = 99 },
		func(a *Attestation) { a.RouteID = "other-route" },
		func(a *Attestation) { a.SourceRef = strings.ToUpper(a.SourceRef[2:]) },
	}
	for i, mutate := range mutations {
		candidate := base
		mutate(&candidate)
		id, err := candidate.ID()
		if err != nil {
			t.Fatal(err)
		}
		if id != baseID {
			t.Fatalf("mutation %d changed the source-deposit replay key", i)
		}
	}
	otherDeposit := base
	otherDeposit.SourceRef = "0x" + strings.Repeat("b", 64)
	if id, _ := otherDeposit.ID(); id == baseID {
		t.Fatal("different source deposits share a replay key")
	}
	otherChain := base
	otherChain.SourceChainID = "25"
	if id, _ := otherChain.ID(); id == baseID {
		t.Fatal("same reference on another source chain shares a replay key")
	}
}

func TestSigningDigestBindsNetworkAndEveryField(t *testing.T) {
	base := validAttestation()
	digest, err := SigningDigest(base, testChainID)
	if err != nil {
		t.Fatal(err)
	}
	if other, _ := SigningDigest(base, MainnetChainID); other == digest {
		t.Fatal("digest does not bind the destination chain ID")
	}
	changed := base
	changed.DeadlineUnix++
	if other, _ := SigningDigest(changed, testChainID); other == digest {
		t.Fatal("digest does not bind the deadline")
	}
	for _, chainID := range []string{"", " xitcoin", "xitcoin/x", "a\x00b", strings.Repeat("x", 65)} {
		if _, err := SigningDigest(base, chainID); err == nil {
			t.Fatalf("chain ID %q was accepted", chainID)
		}
	}
}

func TestSourceChainIDMustBeCanonicalEVMChainID(t *testing.T) {
	for _, value := range []string{"", "0", "025", "cronos-testnet", " 338", "338 ", "-1", strings.Repeat("9", 21)} {
		a := validAttestation()
		a.SourceChainID = value
		if err := a.Validate(); err == nil {
			t.Fatalf("source chain ID %q was accepted", value)
		}
	}
	c := validRouteConfig()
	c.SourceChainID = "cronos"
	if err := c.Validate(); err == nil {
		t.Fatal("non-numeric route source chain ID was accepted")
	}
	c.SourceChainID = "25"
	if err := c.Validate(); err != nil {
		t.Fatalf("valid route source chain ID rejected: %v", err)
	}
	if got := (RouteConfig{}).EffectiveSourceChainID(MainnetChainID); got != CronosMainnetChainID {
		t.Fatalf("mainnet default source chain = %s", got)
	}
	if got := (RouteConfig{}).EffectiveSourceChainID("xitcoin-testnet-v2-1"); got != CronosTestnetChainID {
		t.Fatalf("testnet default source chain = %s", got)
	}
	if got := (RouteConfig{SourceChainID: "31337"}).EffectiveSourceChainID(MainnetChainID); got != "31337" {
		t.Fatalf("explicit source chain ignored: %s", got)
	}
}

func TestDomainsAreNetworkSpecific(t *testing.T) {
	testnet, err := AttestationDomain("xitcoin-testnet-v2-1")
	if err != nil {
		t.Fatal(err)
	}
	mainnet, err := AttestationDomain(MainnetChainID)
	if err != nil {
		t.Fatal(err)
	}
	if testnet == mainnet || testnet != "xitcoin-bridge-attestation-v2/xitcoin-testnet-v2-1" {
		t.Fatalf("unexpected domains %q %q", testnet, mainnet)
	}
	action := validControlAction()
	first, _ := ControlDigest(action, "xitcoin-testnet-v2-1")
	second, _ := ControlDigest(action, MainnetChainID)
	if first == second {
		t.Fatal("control digest does not bind the chain ID")
	}
	pauseFirst, _ := GuardianPauseDigest(validGuardianPause(), "xitcoin-testnet-v2-1")
	pauseSecond, _ := GuardianPauseDigest(validGuardianPause(), MainnetChainID)
	if pauseFirst == pauseSecond {
		t.Fatal("guardian pause digest does not bind the chain ID")
	}
}

func TestControlActionAcceptsOnlyHandledActions(t *testing.T) {
	for _, name := range []string{"close_route", "pause_route", ""} {
		action := validControlAction()
		action.Action = name
		if err := action.Validate(); err == nil {
			t.Fatalf("control action %q without a handler was accepted", name)
		}
	}
	action := validControlAction()
	action.Action = ActionResumeRoute
	if err := action.Validate(); err != nil {
		t.Fatalf("resume action rejected: %v", err)
	}
}

func TestCronosDestinationMatchesVaultRecipientRule(t *testing.T) {
	for _, value := range []string{
		"0x0000000000000000000000000000000000000000",
		"0x000000000000000000000000000000000000dEaD",
		"0x000000000000000000000000000000000000dead",
		"not-an-address",
	} {
		if _, err := ValidateCronosDestination(value); err == nil {
			t.Fatalf("destination %s was accepted", value)
		}
		msg := MsgInitiateOutboundTransfer{Sender: "xtc1zyg3zyg3zyg3zyg3zyg3zyg3zyg3zyg32rdvg9", RouteId: "cronos-testnet-xitcoin-testnet", Destination: value, Amount: "1"}
		if err := msg.ValidateBasic(); err == nil {
			t.Fatalf("outbound message to %s passed basic validation", value)
		}
	}
	if _, err := ValidateCronosDestination("0x0000000000000000000000000000000000000001"); err != nil {
		t.Fatalf("valid destination rejected: %v", err)
	}
}

func TestGenesisValidatesProcessedSourceDeposits(t *testing.T) {
	config := validRouteConfig()
	key := "0x" + strings.Repeat("ab", 32)
	valid := GenesisState{RouteConfig: &config, ProcessedSourceDeposits: []string{key}, ControlNonce: 3}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid genesis rejected: %v", err)
	}
	for _, keys := range [][]string{{key, key}, {strings.ToUpper(key)}, {key[2:]}, {"0x1234"}} {
		state := GenesisState{RouteConfig: &config, ProcessedSourceDeposits: keys}
		if err := state.Validate(); err == nil {
			t.Fatalf("invalid processed keys accepted: %v", keys)
		}
	}
	if err := (GenesisState{ControlNonce: 1}).Validate(); err == nil {
		t.Fatal("control nonce without a route was accepted")
	}
}
