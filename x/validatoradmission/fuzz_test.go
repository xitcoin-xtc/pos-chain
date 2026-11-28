package validatoradmission

// Attack simulation for the validator admission ante handler, as a Go native
// fuzz target. `go test` runs it on its seed corpus; `go test -fuzz` explores
// further (see .github/workflows/fuzz.yml and
// docs/validator-admission-attack-simulations.md). The fuzz input is decoded
// into an environment (approvals, on-chain minimum, governance switch, block
// height) and a tree of messages: authz MsgExec up to ten levels deep,
// governance proposals carrying messages, the restricted messages, grants for
// them and unknown Any types. A small reference walker, written from the
// policy and not from the handler, decides which transactions must be
// rejected. A transaction the reference rejects must never reach the next ante
// handler; a transaction it accepts must.

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/xitcoin-xtc/pos-chain/x/validatoradmission/keeper"
	"github.com/xitcoin-xtc/pos-chain/x/validatoradmission/types"

	sdkmath "cosmossdk.io/math"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	sdktestutil "github.com/cosmos/cosmos-sdk/testutil"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/authz"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	govv1beta1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1beta1"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

// ---------------------------------------------------------------------------
// Reference policy, written from docs/validator-admission.md.

// referenceMaxNesting is the documented bound on authz and proposal nesting
// (the authz limiter of the EVM ante chain allows fewer than 7 levels).
const referenceMaxNesting = 7

const unknownMsgTypeURL = "/xitcoin.unknown.v1.MsgUnknown"

type referenceEnv struct {
	allowGovernance bool
	genesis         bool
	// minimum is nil when the on-chain minimum is unusable.
	minimum  *sdk.Coin
	approved map[string]bool
}

// restricted lists the actions that must be signed directly by the operator
// or the proposer: validator creation and unjailing, mint parameter changes
// and, while executable governance is disabled, proposal submission.
func (e referenceEnv) restricted(typeURL string) bool {
	switch typeURL {
	case "/cosmos.staking.v1beta1.MsgCreateValidator",
		"/cosmos.slashing.v1beta1.MsgUnjail",
		"/cosmos.mint.v1beta1.MsgUpdateParams":
		return true
	case "/cosmos.gov.v1.MsgSubmitProposal", "/cosmos.gov.v1beta1.MsgSubmitProposal":
		return !e.allowGovernance
	}
	return false
}

// operatorApproved reports whether address names an approved operator, in
// any spelling the staking module would decode to that operator.
func (e referenceEnv) operatorApproved(address string) bool {
	operator, err := sdk.ValAddressFromBech32(address)
	if err != nil {
		return false
	}
	return e.approved[operator.String()]
}

func resolveAnys(anys []*codectypes.Any) ([]sdk.Msg, bool) {
	msgs := make([]sdk.Msg, 0, len(anys))
	for _, value := range anys {
		if value == nil {
			return nil, false
		}
		msg, ok := value.GetCachedValue().(sdk.Msg)
		if !ok {
			return nil, false
		}
		msgs = append(msgs, msg)
	}
	return msgs, true
}

// accepts reports whether the policy lets msgs, found at the given nesting
// depth (0 = signed directly), reach the rest of the ante chain. A message
// inside an authz MsgExec or a governance proposal is executed later without
// the ante handler, so it counts as nested.
func (e referenceEnv) accepts(msgs []sdk.Msg, depth int) bool {
	if depth >= referenceMaxNesting {
		return false
	}
	for _, msg := range msgs {
		if depth > 0 && e.restricted(sdk.MsgTypeURL(msg)) {
			return false
		}
		switch msg := msg.(type) {
		case *authz.MsgExec:
			inner, ok := resolveAnys(msg.Msgs)
			if !ok || !e.accepts(inner, depth+1) {
				return false
			}
		case *govtypes.MsgSubmitProposal:
			if !e.allowGovernance {
				return false
			}
			inner, ok := resolveAnys(msg.Messages)
			if !ok || !e.accepts(inner, depth+1) {
				return false
			}
		case *govv1beta1.MsgSubmitProposal:
			if !e.allowGovernance {
				return false
			}
		case *authz.MsgGrant:
			if msg.Grant.Authorization == nil {
				return false
			}
			authorization, ok := msg.Grant.Authorization.GetCachedValue().(authz.Authorization)
			if !ok || e.restricted(authorization.MsgTypeURL()) {
				return false
			}
		case *stakingtypes.MsgCreateValidator:
			if !e.operatorApproved(msg.ValidatorAddress) || e.minimum == nil {
				return false
			}
			if msg.Value.Denom != e.minimum.Denom || msg.Value.Amount.IsNil() || msg.Value.Amount.LT(e.minimum.Amount) {
				return false
			}
			if !e.genesis && (msg.MinSelfDelegation.IsNil() || msg.MinSelfDelegation.LT(e.minimum.Amount)) {
				return false
			}
		case *slashingtypes.MsgUnjail:
			if !e.operatorApproved(msg.ValidatorAddr) {
				return false
			}
		case *minttypes.MsgUpdateParams:
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// Input decoding.

const (
	maxFuzzDepth = 10
	maxFuzzNodes = 48
)

const (
	kindSend byte = iota
	kindDelegate
	kindCreateValidator
	kindUnjail
	kindMintParams
	kindGovProposal
	kindLegacyProposal
	kindGenericGrant
	kindTypedGrant
	kindExec
	kindExecUnknown
	kindGrantUnknown
	kindEditValidator
	kindRevoke
	kindApproveValidator
	kindExecLegacyContent
	kindCount
)

const fuzzOperators = 4

var grantTypeURLs = []string{
	sdk.MsgTypeURL(&stakingtypes.MsgCreateValidator{}),
	sdk.MsgTypeURL(&slashingtypes.MsgUnjail{}),
	sdk.MsgTypeURL(&minttypes.MsgUpdateParams{}),
	sdk.MsgTypeURL(&govtypes.MsgSubmitProposal{}),
	sdk.MsgTypeURL(&govv1beta1.MsgSubmitProposal{}),
	sdk.MsgTypeURL(&authz.MsgExec{}),
	sdk.MsgTypeURL(&stakingtypes.MsgDelegate{}),
	sdk.MsgTypeURL(&stakingtypes.MsgEditValidator{}),
	sdk.MsgTypeURL(&banktypes.MsgSend{}),
	sdk.MsgTypeURL(&types.MsgApproveValidator{}),
	"/cosmos.staking.v1beta1.MsgCreateValidator ",
	"",
}

type fuzzReader struct {
	data []byte
	pos  int
}

func (r *fuzzReader) next() byte {
	if r.pos >= len(r.data) {
		return 0
	}
	b := r.data[r.pos]
	r.pos++
	return b
}

func (r *fuzzReader) pick(n int) int {
	return int(r.next()) % n
}

type fuzzCase struct {
	env        referenceEnv
	minimumRaw string
	approvals  []string
	msgs       []sdk.Msg
}

type fuzzBuilder struct {
	r         *fuzzReader
	nodes     int
	operators []string
	account   sdk.AccAddress
	minimum   sdk.Coin
}

var fuzzOperatorSeeds = [fuzzOperators]byte{51, 52, 53, 54}

func fuzzOperator(index int) string {
	return sdk.ValAddress(bytes.Repeat([]byte{fuzzOperatorSeeds[index]}, 20)).String()
}

func decodeFuzzCase(data []byte) fuzzCase {
	r := &fuzzReader{data: data}
	defaultMinimum, err := sdk.ParseCoinNormalized(types.DefaultMinimumSelfDelegation)
	if err != nil {
		panic(err)
	}

	flags := r.next()
	c := fuzzCase{}
	c.env.allowGovernance = flags&1 == 1
	c.env.genesis = flags&2 == 2
	switch (flags >> 2) & 3 {
	case 1:
		c.minimumRaw = "1axtc"
	case 2:
		c.minimumRaw = "not-a-coin"
	default:
		c.minimumRaw = types.DefaultMinimumSelfDelegation
	}
	if minimum, err := sdk.ParseCoinNormalized(c.minimumRaw); err == nil {
		c.env.minimum = &minimum
	} else {
		c.env.minimum = nil
	}

	b := &fuzzBuilder{r: r, account: sdk.AccAddress(bytes.Repeat([]byte{41}, 20)), minimum: defaultMinimum}
	if c.env.minimum != nil {
		b.minimum = *c.env.minimum
	}
	approvals := r.next()
	c.env.approved = map[string]bool{}
	for i := 0; i < fuzzOperators; i++ {
		b.operators = append(b.operators, fuzzOperator(i))
		if approvals&(1<<i) != 0 {
			c.env.approved[fuzzOperator(i)] = true
			c.approvals = append(c.approvals, fuzzOperator(i))
		}
	}

	count := 1 + r.pick(4)
	for i := 0; i < count; i++ {
		c.msgs = append(c.msgs, b.message(0))
	}
	return c
}

func (b *fuzzBuilder) operator() string {
	switch index := b.r.pick(fuzzOperators + 3); {
	case index < fuzzOperators:
		return b.operators[index]
	case index == fuzzOperators:
		return strings.ToUpper(b.operators[0])
	case index == fuzzOperators+1:
		return strings.ToUpper(b.operators[2])
	default:
		return "xitcoinvaloper1notanaddress"
	}
}

func (b *fuzzBuilder) amount(choice int) sdkmath.Int {
	switch choice {
	case 0:
		return sdkmath.Int{}
	case 1:
		return sdkmath.ZeroInt()
	case 2:
		return sdkmath.OneInt()
	case 3:
		return b.minimum.Amount.SubRaw(1)
	case 4:
		return b.minimum.Amount
	case 5:
		return b.minimum.Amount.AddRaw(1)
	default:
		return sdkmath.OneInt().MulRaw(1 << 62).MulRaw(1 << 62)
	}
}

func (b *fuzzBuilder) children(depth, limit int) []sdk.Msg {
	count := b.r.pick(limit + 1)
	if depth+1 > maxFuzzDepth {
		count = 0
	}
	msgs := make([]sdk.Msg, 0, count)
	for i := 0; i < count; i++ {
		msgs = append(msgs, b.message(depth+1))
	}
	return msgs
}

func mustExec(grantee sdk.AccAddress, msgs []sdk.Msg) *authz.MsgExec {
	msg := authz.NewMsgExec(grantee, msgs)
	return &msg
}

func (b *fuzzBuilder) message(depth int) sdk.Msg {
	b.nodes++
	if b.nodes > maxFuzzNodes {
		return &banktypes.MsgSend{FromAddress: b.account.String(), ToAddress: b.account.String()}
	}
	switch b.r.next() % kindCount {
	case kindSend:
		return &banktypes.MsgSend{FromAddress: b.account.String(), ToAddress: b.account.String()}
	case kindDelegate:
		return &stakingtypes.MsgDelegate{DelegatorAddress: b.account.String(), ValidatorAddress: b.operators[0]}
	case kindCreateValidator:
		operator := b.operator()
		denom := b.minimum.Denom
		if b.r.pick(2) == 1 {
			denom = "aatom"
		}
		value := b.amount(b.r.pick(7))
		msd := b.amount(b.r.pick(7))
		return &stakingtypes.MsgCreateValidator{
			ValidatorAddress:  operator,
			Value:             sdk.Coin{Denom: denom, Amount: value},
			MinSelfDelegation: msd,
		}
	case kindUnjail:
		return &slashingtypes.MsgUnjail{ValidatorAddr: b.operator()}
	case kindMintParams:
		return &minttypes.MsgUpdateParams{Authority: b.account.String(), Params: minttypes.DefaultParams()}
	case kindGovProposal:
		msg, err := govtypes.NewMsgSubmitProposal(b.children(depth, 2), nil, b.account.String(), "", "t", "s", false)
		if err != nil {
			panic(err)
		}
		return msg
	case kindLegacyProposal:
		msg, err := govv1beta1.NewMsgSubmitProposal(govv1beta1.NewTextProposal("t", "d"), nil, b.account)
		if err != nil {
			panic(err)
		}
		return msg
	case kindGenericGrant:
		url := grantTypeURLs[b.r.pick(len(grantTypeURLs))]
		grant, err := authz.NewMsgGrant(b.account, b.account, authz.NewGenericAuthorization(url), nil)
		if err != nil {
			panic(err)
		}
		return grant
	case kindTypedGrant:
		var authorization authz.Authorization
		switch b.r.pick(3) {
		case 0:
			allowed, err := sdk.ValAddressFromBech32(b.operators[0])
			if err != nil {
				panic(err)
			}
			stake, err := stakingtypes.NewStakeAuthorization([]sdk.ValAddress{allowed}, nil, stakingtypes.AuthorizationType_AUTHORIZATION_TYPE_DELEGATE, nil)
			if err != nil {
				panic(err)
			}
			authorization = stake
		case 1:
			authorization = banktypes.NewSendAuthorization(sdk.NewCoins(sdk.NewInt64Coin("axtc", 1)), nil)
		default:
			return &authz.MsgGrant{Granter: b.account.String(), Grantee: b.account.String()}
		}
		grant, err := authz.NewMsgGrant(b.account, b.account, authorization, nil)
		if err != nil {
			panic(err)
		}
		return grant
	case kindExec:
		return mustExec(b.account, b.children(depth, 3))
	case kindExecUnknown:
		msg := mustExec(b.account, b.children(depth, 2))
		msg.Msgs = append(msg.Msgs, &codectypes.Any{TypeUrl: unknownMsgTypeURL, Value: []byte{1, 2, 3}})
		return msg
	case kindGrantUnknown:
		return &authz.MsgGrant{
			Granter: b.account.String(),
			Grantee: b.account.String(),
			Grant:   authz.Grant{Authorization: &codectypes.Any{TypeUrl: "/xitcoin.unknown.v1.Authorization"}},
		}
	case kindEditValidator:
		msd := b.amount(b.r.pick(7))
		return &stakingtypes.MsgEditValidator{ValidatorAddress: b.operators[0], MinSelfDelegation: &msd}
	case kindRevoke:
		return &authz.MsgRevoke{
			Granter:    b.account.String(),
			Grantee:    b.account.String(),
			MsgTypeUrl: grantTypeURLs[b.r.pick(len(grantTypeURLs))],
		}
	case kindApproveValidator:
		return &types.MsgApproveValidator{Authority: b.account.String(), ValidatorAddress: b.operator()}
	default:
		content, err := codectypes.NewAnyWithValue(&govv1beta1.TextProposal{Title: "t", Description: "d"})
		if err != nil {
			panic(err)
		}
		return govtypes.NewMsgExecLegacyContent(content, b.account.String())
	}
}

// ---------------------------------------------------------------------------
// Seed encoding: the same choices, written as bytes.

type seedNode struct {
	bytes []byte
}

// count encodes a number of seed nodes as one input byte.
func count(n int) byte {
	if n < 0 || n > 255 {
		panic("seed node count out of range")
	}
	return byte(n)
}

func seed(flags, approvals byte, top ...seedNode) []byte {
	out := []byte{flags, approvals, count(len(top) - 1)}
	for _, node := range top {
		out = append(out, node.bytes...)
	}
	return out
}

func leaf(kind byte, args ...byte) seedNode {
	return seedNode{bytes: append([]byte{kind}, args...)}
}

func container(kind byte, children ...seedNode) seedNode {
	out := []byte{kind, count(len(children))}
	for _, child := range children {
		out = append(out, child.bytes...)
	}
	return seedNode{bytes: out}
}

// Operator choices: 0-3 the pool (0, 1 usually approved), 4 operator 0 in
// upper case, 5 operator 2 in upper case, 6 not an address. Amount choices:
// 0 nil, 1 zero, 2 one, 3 minimum-1, 4 minimum, 5 minimum+1, 6 2^124.
func createValidator(operator, wrongDenom, value, msd byte) seedNode {
	return leaf(kindCreateValidator, operator, wrongDenom, value, msd)
}

func nestedExec(depth int, inner seedNode) seedNode {
	node := inner
	for i := 0; i < depth; i++ {
		node = container(kindExec, node)
	}
	return node
}

const (
	flagGovernance = 1
	flagGenesis    = 2
	flagLowMinimum = 1 << 2
	flagBadMinimum = 2 << 2
)

func admissionSeeds() [][]byte {
	valid := createValidator(0, 0, 4, 4)
	seeds := [][]byte{
		seed(0, 0b0011, valid),
		seed(0, 0b0011, createValidator(3, 0, 4, 4)),
		seed(0, 0b0011, createValidator(0, 0, 3, 4)),
		seed(0, 0b0011, createValidator(0, 0, 4, 3)),
		seed(0, 0b0011, createValidator(0, 0, 4, 0)),
		seed(0, 0b0011, createValidator(0, 0, 0, 4)),
		seed(0, 0b0011, createValidator(0, 1, 6, 6)),
		seed(0, 0b0011, createValidator(4, 0, 4, 4)),
		seed(flagGenesis, 0b0011, createValidator(0, 0, 4, 2)),
		seed(flagBadMinimum, 0b0011, valid),
		seed(flagLowMinimum, 0b0011, createValidator(0, 0, 2, 2)),
		seed(0, 0b0011, leaf(kindUnjail, 0)),
		seed(0, 0b0011, leaf(kindUnjail, 2)),
		seed(0, 0b0111, leaf(kindUnjail, 5)),
		seed(0, 0b0011, leaf(kindMintParams)),
		seed(flagGovernance, 0b0011, leaf(kindMintParams)),
		seed(0, 0, container(kindGovProposal)),
		seed(0, 0, leaf(kindLegacyProposal)),
		seed(flagGovernance, 0, container(kindGovProposal)),
		seed(flagGovernance, 0, leaf(kindLegacyProposal)),
		// Restricted messages carried by a proposal while governance is on.
		seed(flagGovernance, 0b0011, container(kindGovProposal, leaf(kindMintParams))),
		seed(flagGovernance, 0b0011, container(kindGovProposal, valid)),
		seed(flagGovernance, 0b0011, container(kindGovProposal, leaf(kindUnjail, 0))),
		seed(flagGovernance, 0b0011, container(kindGovProposal, container(kindExec, leaf(kindMintParams)))),
		seed(flagGovernance, 0b0011, container(kindGovProposal, leaf(kindGenericGrant, 2))),
		seed(flagGovernance, 0b0011, container(kindGovProposal, container(kindExecUnknown))),
		seed(flagGovernance, 0b0011, container(kindGovProposal, container(kindGovProposal, leaf(kindMintParams)))),
		seed(flagGovernance, 0b0011, container(kindGovProposal, leaf(kindSend), leaf(kindDelegate))),
		// Grants for every listed type URL, and typed and unknown grants.
		seed(0, 0, leaf(kindGenericGrant, 0)),
		seed(0, 0, leaf(kindGenericGrant, 1)),
		seed(0, 0, leaf(kindGenericGrant, 2)),
		seed(0, 0, leaf(kindGenericGrant, 3)),
		seed(0, 0, leaf(kindGenericGrant, 4)),
		seed(flagGovernance, 0, leaf(kindGenericGrant, 3)),
		seed(0, 0, leaf(kindGenericGrant, 5)),
		seed(0, 0, leaf(kindGenericGrant, 6)),
		seed(0, 0, leaf(kindGenericGrant, 10)),
		seed(0, 0, leaf(kindTypedGrant, 0)),
		seed(0, 0, leaf(kindTypedGrant, 1)),
		seed(0, 0, leaf(kindTypedGrant, 2)),
		seed(0, 0, leaf(kindGrantUnknown)),
		// Self-granted and nested MsgExec at every depth from 1 to 10.
		seed(0, 0b0011, container(kindExec, valid)),
		seed(0, 0b0011, container(kindExec, leaf(kindUnjail, 0))),
		seed(0, 0b0011, container(kindExec, leaf(kindMintParams))),
		seed(0, 0b0011, container(kindExec, container(kindGovProposal))),
		seed(flagGovernance, 0b0011, container(kindExec, container(kindGovProposal, leaf(kindMintParams)))),
		seed(flagGovernance, 0b0011, container(kindExec, container(kindGovProposal, leaf(kindSend)))),
		seed(0, 0b0011, container(kindExec, leaf(kindGenericGrant, 0))),
		seed(0, 0b0011, container(kindExecUnknown, leaf(kindSend))),
		seed(0, 0b0011, container(kindExec, leaf(kindSend), leaf(kindDelegate), valid)),
		seed(0, 0b0011, leaf(kindEditValidator, 1)),
		seed(0, 0b0011, container(kindExec, leaf(kindEditValidator, 3))),
		seed(0, 0b0011, leaf(kindRevoke, 0)),
		seed(0, 0b0011, leaf(kindApproveValidator, 3)),
		seed(0, 0b0011, leaf(kindExecLegacyContent)),
		seed(0, 0b0011, leaf(kindSend), valid, leaf(kindMintParams)),
	}
	for depth := 1; depth <= maxFuzzDepth; depth++ {
		seeds = append(seeds,
			seed(0, 0b0011, nestedExec(depth, leaf(kindSend))),
			seed(0, 0b0011, nestedExec(depth, valid)),
			seed(flagGovernance, 0b0011, nestedExec(depth, container(kindGovProposal, leaf(kindMintParams)))),
		)
	}
	return seeds
}

// ---------------------------------------------------------------------------
// Target.

// admissionCodec decodes messages as the application's transaction decoder
// does, for the decoded round trip below.
func admissionCodec() codec.Codec {
	registry := codectypes.NewInterfaceRegistry()
	sdk.RegisterInterfaces(registry)
	authz.RegisterInterfaces(registry)
	banktypes.RegisterInterfaces(registry)
	govtypes.RegisterInterfaces(registry)
	govv1beta1.RegisterInterfaces(registry)
	minttypes.RegisterInterfaces(registry)
	slashingtypes.RegisterInterfaces(registry)
	stakingtypes.RegisterInterfaces(registry)
	types.RegisterInterfaces(registry)
	return codec.NewProtoCodec(registry)
}

// roundTrip encodes every message into an Any and decodes it back with the
// interface registry, as a transaction decoder would. ok is false when the
// decoder rejects a message (for example an unknown nested Any type).
func roundTrip(cdc codec.Codec, msgs []sdk.Msg) ([]sdk.Msg, bool) {
	decoded := make([]sdk.Msg, 0, len(msgs))
	for _, msg := range msgs {
		packed, err := codectypes.NewAnyWithValue(msg)
		if err != nil {
			return nil, false
		}
		bz, err := cdc.Marshal(packed)
		if err != nil {
			return nil, false
		}
		var back codectypes.Any
		if err := cdc.Unmarshal(bz, &back); err != nil {
			return nil, false
		}
		var out sdk.Msg
		if err := cdc.UnpackAny(&back, &out); err != nil {
			return nil, false
		}
		decoded = append(decoded, out)
	}
	return decoded, true
}

func runAdmission(t *testing.T, c fuzzCase, msgs []sdk.Msg) (reached bool, err error) {
	t.Helper()
	key := storetypes.NewKVStoreKey(types.StoreKey)
	ctx := sdktestutil.DefaultContext(key, storetypes.NewTransientStoreKey("validator_admission_fuzz"))
	height := int64(100)
	if c.env.genesis {
		height = 0
	}
	ctx = ctx.WithBlockHeight(height)
	admission := keeper.NewKeeper(key)
	admission.SetMinimumSelfDelegation(ctx, c.minimumRaw)
	for _, operator := range c.approvals {
		admission.SetApprovedValidator(ctx, operator, true)
	}
	handler := NewAdmissionAnteHandler(admission, func(ctx sdk.Context, _ sdk.Tx, _ bool) (sdk.Context, error) {
		reached = true
		return ctx, nil
	}, c.env.allowGovernance)
	_, err = handler(ctx, testTx{messages: msgs}, false)
	return reached, err
}

func describeMsgs(msgs []sdk.Msg) string {
	parts := make([]string, 0, len(msgs))
	for _, msg := range msgs {
		parts = append(parts, fmt.Sprintf("%s{%v}", sdk.MsgTypeURL(msg), msg))
	}
	return strings.Join(parts, ", ")
}

// FuzzAdmissionAnteHandler: no restricted message, at any depth of authz or
// proposal nesting, ever gets past the admission ante handler unless the
// policy allows it, and neither does a grant for one; the handler never
// panics; an acceptable transaction is accepted, whatever the spelling of
// its operator addresses; and a
// transaction decoded from its encoding gets the same decision.
func FuzzAdmissionAnteHandler(f *testing.F) {
	for _, s := range admissionSeeds() {
		f.Add(s)
	}
	cdc := admissionCodec()
	f.Fuzz(func(t *testing.T, data []byte) {
		c := decodeFuzzCase(data)
		want := c.env.accepts(c.msgs, 0)

		reached, err := runAdmission(t, c, c.msgs)
		if (err == nil) != reached {
			t.Fatalf("handler returned err=%v but reached the next handler=%v", err, reached)
		}
		if reached && !want {
			t.Fatalf("policy violation: accepted a transaction the policy rejects (governance=%v genesis=%v minimum=%q approved=%v): %s",
				c.env.allowGovernance, c.env.genesis, c.minimumRaw, c.approvals, describeMsgs(c.msgs))
		}
		if !reached && want {
			t.Fatalf("rejected an acceptable transaction: %v (governance=%v genesis=%v minimum=%q approved=%v): %s",
				err, c.env.allowGovernance, c.env.genesis, c.minimumRaw, c.approvals, describeMsgs(c.msgs))
		}

		decoded, ok := roundTrip(cdc, c.msgs)
		if !ok {
			if want {
				t.Fatalf("the decoder rejected a transaction the policy accepts: %s", describeMsgs(c.msgs))
			}
			return
		}
		decodedReached, decodedErr := runAdmission(t, c, decoded)
		if decodedReached && !c.env.accepts(decoded, 0) {
			t.Fatalf("policy violation after decoding: %s", describeMsgs(decoded))
		}
		if !decodedReached && c.env.accepts(decoded, 0) {
			t.Fatalf("decoded acceptable transaction rejected: %v: %s", decodedErr, describeMsgs(decoded))
		}
	})
}
