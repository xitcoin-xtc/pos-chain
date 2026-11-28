package attack

// Calldata attack simulations of every Cosmos precompile on the real
// application. `go test` replays the seed corpora; `go test -fuzz` explores
// further (see .github/workflows/fuzz.yml and
// docs/precompile-attack-simulations.md).
//
// Each input is decoded into a call kind (CALL, STATICCALL, DELEGATECALL,
// CALLCODE), optional value, a calldata (ABI-encoded arguments drawn from
// addresses, strings and amounts that exist on the chain, raw argument
// bytes, a truncated encoding, or arbitrary bytes) and a gas limit around
// and above the precompile's base cost. It runs against a fresh cache of the
// same chain state, twice. Properties:
//   - no panic;
//   - the call consumes at least the base cost RequiredGas(input), and a
//     call supplied with less fails with out-of-gas;
//   - a failed call, any read-only call (STATICCALL, DELEGATECALL,
//     CALLCODE: the precompile runs read-only) and any call of a view
//     method leave every persistent KV store byte-identical, Cosmos and EVM
//     state alike;
//   - value sent to a method that is not payable never stays with the
//     precompile: the call fails, or the transaction commit rejects it
//     (static precompile addresses cannot receive funds);
//   - the same input gives the same result, gas and state (determinism).

import (
	"bytes"
	"errors"
	"math/big"
	"reflect"
	"runtime/debug"
	"sort"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/holiman/uint256"

	bankprecompile "github.com/xitcoin-xtc/pos-chain/precompiles/bank"
	"github.com/xitcoin-xtc/pos-chain/precompiles/bech32"
	distprecompile "github.com/xitcoin-xtc/pos-chain/precompiles/distribution"
	erc20precompile "github.com/xitcoin-xtc/pos-chain/precompiles/erc20"
	govprecompile "github.com/xitcoin-xtc/pos-chain/precompiles/gov"
	ics02precompile "github.com/xitcoin-xtc/pos-chain/precompiles/ics02"
	ics20precompile "github.com/xitcoin-xtc/pos-chain/precompiles/ics20"
	slashingprecompile "github.com/xitcoin-xtc/pos-chain/precompiles/slashing"
	stakingprecompile "github.com/xitcoin-xtc/pos-chain/precompiles/staking"
	"github.com/xitcoin-xtc/pos-chain/precompiles/werc20"
	testconstants "github.com/xitcoin-xtc/pos-chain/testutil/constants"
	erc20types "github.com/xitcoin-xtc/pos-chain/x/erc20/types"
	"github.com/xitcoin-xtc/pos-chain/x/vm/statedb"
	evmtypes "github.com/xitcoin-xtc/pos-chain/x/vm/types"

	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

type precompileTarget struct {
	name    string
	address common.Address
	abi     *abi.ABI // nil for precompiles without an ABI (p256)
	methods []abi.Method
}

func newTarget(name, address string, contractABI *abi.ABI) precompileTarget {
	target := precompileTarget{name: name, address: common.HexToAddress(address), abi: contractABI}
	if contractABI != nil {
		names := make([]string, 0, len(contractABI.Methods))
		for name := range contractABI.Methods {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			target.methods = append(target.methods, contractABI.Methods[name])
		}
	}
	return target
}

var (
	stakingTarget      = newTarget("staking", evmtypes.StakingPrecompileAddress, &stakingprecompile.ABI)
	distributionTarget = newTarget("distribution", evmtypes.DistributionPrecompileAddress, &distprecompile.ABI)
	bankTarget         = newTarget("bank", evmtypes.BankPrecompileAddress, &bankprecompile.ABI)
	bech32Target       = newTarget("bech32", evmtypes.Bech32PrecompileAddress, &bech32.ABI)
	p256Target         = newTarget("p256", evmtypes.P256PrecompileAddress, nil)
	ics02Target        = newTarget("ics02", evmtypes.ICS02PrecompileAddress, &ics02precompile.ABI)
	ics20Target        = newTarget("ics20", evmtypes.ICS20PrecompileAddress, &ics20precompile.ABI)
	govTarget          = newTarget("gov", evmtypes.GovPrecompileAddress, &govprecompile.ABI)
	slashingTarget     = newTarget("slashing", evmtypes.SlashingPrecompileAddress, &slashingprecompile.ABI)
	// The native-denomination WERC20 precompile, a dynamic ERC-20 precompile.
	werc20Target = newTarget("werc20", testconstants.WEVMOSContractMainnet, &werc20.ABI)
	// The dynamic ERC-20 precompile of an IBC denomination, registered by the
	// harness.
	erc20Target = newTarget("erc20", mustIBCDenomAddress(ibcVoucherDenom), &erc20precompile.ABI)
)

const ibcVoucherDenom = "ibc/27394FB092D2ECCD56123C74F36E4C1F926001CEADA9CA97EA622B25F41E5EB2"

func mustIBCDenomAddress(denom string) string {
	pair, err := erc20types.NewTokenPairSTRv2(denom)
	if err != nil {
		panic(err)
	}
	return pair.Erc20Address
}

// callKind is the EVM call used to reach the precompile.
type callKind byte

const (
	kindCall callKind = iota
	kindStaticCall
	kindDelegateCall
	kindCallCode
)

// readOnly reports whether the precompile runs read-only: the EVM fork runs
// precompiles read-only under STATICCALL, DELEGATECALL and CALLCODE.
func (k callKind) readOnly() bool { return k != kindCall }

// dataKind is how the calldata is built.
type dataKind byte

const (
	dataStructured dataKind = iota // selector and ABI-encoded arguments
	dataRawArgs                    // selector and arbitrary argument bytes
	dataRawInput                   // arbitrary bytes, possibly shorter than a selector
	dataTruncated                  // a structured encoding cut short
)

// mode packs a call kind, a data kind and the value flag into the first
// input byte.
func mode(kind callKind, data dataKind, withValue bool) byte {
	b := byte(kind) | byte(data)<<2
	if withValue {
		b |= 0x10
	}
	return b
}

// fuzzReader consumes the fuzz input; it returns zeros once exhausted.
type fuzzReader struct{ data []byte }

func (r *fuzzReader) byte() byte {
	if len(r.data) == 0 {
		return 0
	}
	b := r.data[0]
	r.data = r.data[1:]
	return b
}

func (r *fuzzReader) take(n int) []byte {
	if n > len(r.data) {
		n = len(r.data)
	}
	out := r.data[:n]
	r.data = r.data[n:]
	return out
}

func (r *fuzzReader) rest() []byte {
	out := r.data
	r.data = nil
	return out
}

// abiValue draws a Go value of the given ABI type, mostly from values that
// exist on the chain so that calls reach deep into the precompiles.
func (h *calldataHarness) abiValue(r *fuzzReader, t abi.Type, depth int) reflect.Value {
	typ := t.GetType()
	switch t.T {
	case abi.UintTy, abi.IntTy:
		value := h.integer(r, t.Size, t.T == abi.IntTy)
		if typ.Kind() == reflect.Ptr {
			return reflect.ValueOf(value)
		}
		out := reflect.New(typ).Elem()
		if t.T == abi.IntTy {
			out.SetInt(value.Int64())
		} else {
			out.SetUint(value.Uint64())
		}
		return out
	case abi.BoolTy:
		return reflect.ValueOf(r.byte()&1 == 1)
	case abi.StringTy:
		choice := int(r.byte())
		if choice < len(h.strings) {
			return reflect.ValueOf(h.strings[choice])
		}
		return reflect.ValueOf(string(r.take(int(r.byte() % 80))))
	case abi.AddressTy:
		choice := int(r.byte())
		if choice < len(h.addresses) {
			return reflect.ValueOf(h.addresses[choice])
		}
		return reflect.ValueOf(common.BytesToAddress(r.take(20)))
	case abi.BytesTy:
		return reflect.ValueOf(append([]byte{}, r.take(int(r.byte()%96))...))
	case abi.FixedBytesTy, abi.HashTy:
		out := reflect.New(typ).Elem()
		reflect.Copy(out, reflect.ValueOf(r.take(out.Len())))
		return out
	case abi.SliceTy:
		n := int(r.byte() % 4)
		if depth > 2 {
			n = 0
		}
		out := reflect.MakeSlice(typ, n, n)
		for i := 0; i < n; i++ {
			out.Index(i).Set(h.abiValue(r, *t.Elem, depth+1))
		}
		return out
	case abi.ArrayTy:
		out := reflect.New(typ).Elem()
		for i := 0; i < t.Size; i++ {
			out.Index(i).Set(h.abiValue(r, *t.Elem, depth+1))
		}
		return out
	case abi.TupleTy:
		out := reflect.New(typ).Elem()
		for i, elem := range t.TupleElems {
			out.Field(i).Set(h.abiValue(r, *elem, depth+1))
		}
		return out
	default:
		return reflect.Zero(typ)
	}
}

// integer draws an integer that fits in size bits.
func (h *calldataHarness) integer(r *fuzzReader, size int, signed bool) *big.Int {
	choice := int(r.byte())
	var value *big.Int
	switch {
	case choice < len(h.amounts):
		value = new(big.Int).Set(h.amounts[choice])
	default:
		value = new(big.Int).SetBytes(r.take(size / 8))
	}
	limit := new(big.Int).Lsh(big.NewInt(1), uint(size))
	if signed {
		limit.Rsh(limit, 1)
		value.Mod(value, limit)
		if choice%2 == 1 {
			value.Neg(value)
		}
		return value
	}
	return value.Mod(value, limit)
}

type callInput struct {
	kind  callKind
	data  dataKind
	input []byte
	gas   uint64
	value *uint256.Int
	// view is set when the selector is a view or pure method of the ABI,
	// payable when the call reaches a payable method, receive or fallback.
	view, payable bool
}

func (h *calldataHarness) decode(target precompileTarget, data []byte) callInput {
	r := &fuzzReader{data: data}
	first := r.byte()
	in := callInput{kind: callKind(first & 3), data: dataKind(first >> 2 & 3), value: new(uint256.Int)}
	withValue := first&0x10 != 0
	gasChoice, gasExtra := r.byte(), r.byte()
	if target.abi == nil || len(target.methods) == 0 {
		in.input = r.rest()
	} else {
		method := target.methods[int(r.byte())%len(target.methods)]
		switch in.data {
		case dataRawInput:
			in.input = r.rest()
		case dataRawArgs:
			in.input = append(append([]byte{}, method.ID...), r.rest()...)
		default:
			args := make([]interface{}, 0, len(method.Inputs))
			for _, input := range method.Inputs {
				args = append(args, h.abiValue(r, input.Type, 0).Interface())
			}
			packed, err := method.Inputs.Pack(args...)
			if err != nil {
				packed = r.rest()
			}
			in.input = append(append([]byte{}, method.ID...), packed...)
			if in.data == dataTruncated {
				in.input = in.input[:int(r.byte())*len(in.input)/256]
			}
		}
	}
	// DELEGATECALL carries no value of its own; STATICCALL none at all.
	if withValue && (in.kind == kindCall || in.kind == kindCallCode) {
		in.value = uint256.NewInt(1 + uint64(gasExtra)*1e16)
	}
	if target.abi != nil {
		switch method, err := target.abi.MethodById(in.input); {
		case err == nil:
			in.view, in.payable = method.IsConstant(), method.IsPayable()
		case len(in.input) == 0 && target.abi.HasReceive():
			in.payable = true
		default:
			in.payable = target.abi.HasFallback() && target.abi.Fallback.IsPayable()
		}
	}
	base := h.requiredGas(target, in.input)
	switch gasChoice % 8 {
	case 0:
		if base > 0 {
			in.gas = base - 1 - uint64(gasExtra)%base
		}
	case 1:
		in.gas = base
	case 2:
		in.gas = base + uint64(gasExtra)*97
	case 3:
		in.gas = 30_000
	case 4:
		in.gas = 250_000
	case 5:
		in.gas = 2_000_000
	default:
		in.gas = 10_000_000
	}
	return in
}

func (h *calldataHarness) requiredGas(target precompileTarget, input []byte) uint64 {
	precompiles, found, err := h.app.EVMKeeper.GetPrecompileInstance(h.base.WithGasMeter(storetypes.NewInfiniteGasMeter()), target.address)
	if err != nil || !found {
		panic("precompile " + target.name + " is not active")
	}
	return precompiles.Map[target.address].RequiredGas(input)
}

type callOutcome struct {
	ret       []byte
	gasUsed   uint64
	callErr   string
	commitErr string
	digest    [32]byte
}

// execute runs one call on a fresh cache of the frozen chain state, commits
// the StateDB into that cache as a transaction would, and hashes the result.
func (h *calldataHarness) execute(t *testing.T, target precompileTarget, in callInput) (out callOutcome, callErr error) {
	t.Helper()
	// The context of an EVM transaction after the ante handler: a fresh gas
	// meter limited to the gas wanted and no KV gas (opcodes and
	// precompiles charge gas themselves).
	ctx, _ := h.base.CacheContext()
	ctx = ctx.WithGasMeter(evmtypes.NewInfiniteGasMeterWithLimit(in.gas)).
		WithKVGasConfig(storetypes.GasConfig{}).
		WithTransientKVGasConfig(storetypes.GasConfig{}).
		WithEventManager(sdk.NewEventManager())
	caller := h.users[1].addr
	stateDB := statedb.New(ctx, h.app.EVMKeeper, statedb.NewEmptyTxConfig())
	cfg, err := h.app.EVMKeeper.EVMConfig(ctx, ctx.BlockHeader().ProposerAddress)
	if err != nil {
		t.Fatal(err)
	}
	msg := core.Message{
		From: caller, To: &target.address, GasLimit: in.gas, Value: in.value.ToBig(), Data: in.input,
		GasPrice: new(big.Int), GasFeeCap: new(big.Int), GasTipCap: new(big.Int),
	}
	evm := h.app.EVMKeeper.NewEVM(ctx, msg, cfg, nil, stateDB)

	var leftOver uint64
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				t.Fatalf("%s precompile panicked (kind %d, value %s, gas %d, input %x): %v\n%s", target.name, in.kind, in.value, in.gas, in.input, recovered, debug.Stack())
			}
		}()
		switch in.kind {
		case kindStaticCall:
			out.ret, leftOver, callErr = evm.StaticCall(caller, target.address, in.input, in.gas)
		case kindDelegateCall:
			// A contract (here the second user) delegate-calling the
			// precompile on behalf of the caller.
			out.ret, leftOver, callErr = evm.DelegateCall(caller, h.users[2].addr, target.address, in.input, in.gas, in.value)
		case kindCallCode:
			out.ret, leftOver, callErr = evm.CallCode(caller, target.address, in.input, in.gas, in.value)
		default:
			out.ret, leftOver, callErr = evm.Call(caller, target.address, in.input, in.gas, in.value)
		}
	}()
	if leftOver > in.gas {
		t.Fatalf("%s: %d gas left of %d supplied", target.name, leftOver, in.gas)
	}
	out.gasUsed = in.gas - leftOver
	if callErr != nil {
		out.callErr = callErr.Error()
	}
	if err := stateDB.Commit(); err != nil {
		out.commitErr = err.Error()
	}
	ctx = ctx.WithGasMeter(storetypes.NewInfiniteGasMeter())
	out.digest = stateDigest(ctx, h.keys)
	if out.digest != h.baseDigest && (callErr != nil || in.kind.readOnly() || (in.view && in.value.IsZero())) {
		t.Logf("changed stores: %v", diffStores(h.base.WithGasMeter(storetypes.NewInfiniteGasMeter()), ctx, h.keys))
	}
	return out, callErr
}

func runCalldataInput(t *testing.T, target precompileTarget, data []byte) {
	t.Helper()
	h := sharedCalldataHarness(t)
	in := h.decode(target, data)
	base := h.requiredGas(target, in.input)

	first, callErr := h.execute(t, target, in)
	if in.gas < base {
		if !errors.Is(callErr, vm.ErrOutOfGas) || first.gasUsed != in.gas {
			t.Fatalf("%s: %d gas supplied below the base cost %d: err=%v, used %d", target.name, in.gas, base, callErr, first.gasUsed)
		}
	} else if first.gasUsed < base {
		t.Fatalf("%s: used %d gas, below the base cost %d (kind %d, input %x)", target.name, first.gasUsed, base, in.kind, in.input)
	}
	if in.kind == kindCall && !in.value.IsZero() && !in.payable && callErr == nil && first.commitErr == "" {
		t.Fatalf("%s: a non-payable method accepted %s wei and the precompile kept it (input %x)", target.name, in.value, in.input)
	}
	view := in.view && in.value.IsZero()
	if callErr != nil || in.kind.readOnly() || view {
		kind := "failed"
		if view {
			kind = "view-method"
		} else if in.kind.readOnly() {
			kind = "read-only"
		}
		if first.commitErr != "" {
			t.Fatalf("%s: committing a %s call failed: %s", target.name, kind, first.commitErr)
		}
		if first.digest != h.baseDigest {
			t.Fatalf("%s: a %s call (kind %d, value %s, err %v, input %x) changed state", target.name, kind, in.kind, in.value, callErr, in.input)
		}
	}

	second, _ := h.execute(t, target, in)
	if !bytes.Equal(first.ret, second.ret) || first.gasUsed != second.gasUsed || first.callErr != second.callErr ||
		first.commitErr != second.commitErr || first.digest != second.digest {
		t.Fatalf("%s: non-deterministic result for input %x: %+v vs %+v", target.name, in.input, first, second)
	}
}

// calldataSeeds gives every method of the target a structured seed under
// each call kind, plus value, truncated, raw and short inputs.
func calldataSeeds(f *testing.F, target precompileTarget) {
	f.Helper()
	for _, kind := range []callKind{kindCall, kindStaticCall, kindDelegateCall, kindCallCode} {
		f.Add([]byte{mode(kind, dataRawInput, false), 6, 0, 0})
		f.Add([]byte{mode(kind, dataRawInput, true), 6, 0, 0, 0x01, 0x02})
	}
	f.Add([]byte{mode(kindCall, dataRawInput, false), 6, 0, 0, 0xde, 0xad, 0xbe, 0xef, 0, 0, 0})
	if target.abi == nil {
		// p256: valid-length and malformed inputs.
		f.Add(append([]byte{mode(kindCall, dataStructured, false), 6, 0}, make([]byte, 160)...))
		f.Add(append([]byte{mode(kindStaticCall, dataStructured, false), 1, 0}, bytes.Repeat([]byte{0xff}, 160)...))
		f.Add(append([]byte{mode(kindCall, dataStructured, false), 0, 3}, make([]byte, 159)...))
		return
	}
	for index := range target.methods {
		m := byte(index)
		f.Add([]byte{mode(kindCall, dataStructured, false), 6, 0, m, 0, 0, 0, 0, 0, 0, 0, 0})
		f.Add([]byte{mode(kindCall, dataStructured, false), 5, 0, m, 3, 1, 4, 1, 5, 9, 2, 6, 5, 3, 5})
		f.Add([]byte{mode(kindStaticCall, dataStructured, false), 6, 0, m, 1, 2, 3, 4, 5, 6})
		f.Add([]byte{mode(kindDelegateCall, dataStructured, false), 6, 0, m, 0, 0, 1, 1})
		f.Add([]byte{mode(kindCallCode, dataStructured, true), 6, 0, m, 0, 0, 1, 1})
		f.Add([]byte{mode(kindCall, dataStructured, true), 6, 9, m, 0, 4, 0, 4})
		f.Add([]byte{mode(kindCall, dataStructured, false), 0, 7, m, 0, 0})
		f.Add([]byte{mode(kindCall, dataTruncated, false), 6, 0, m, 0, 0, 0, 0, 0, 0, 0x80})
		f.Add([]byte{mode(kindCall, dataRawArgs, false), 6, 0, m, 0xff, 0xff, 0xff, 0xff})
	}
}

func fuzzPrecompile(f *testing.F, target precompileTarget) {
	f.Helper()
	calldataSeeds(f, target)
	f.Fuzz(func(t *testing.T, data []byte) { runCalldataInput(t, target, data) })
}

func FuzzStakingPrecompileCalldata(f *testing.F)      { fuzzPrecompile(f, stakingTarget) }
func FuzzDistributionPrecompileCalldata(f *testing.F) { fuzzPrecompile(f, distributionTarget) }
func FuzzBankPrecompileCalldata(f *testing.F)         { fuzzPrecompile(f, bankTarget) }
func FuzzBech32PrecompileCalldata(f *testing.F)       { fuzzPrecompile(f, bech32Target) }
func FuzzP256PrecompileCalldata(f *testing.F)         { fuzzPrecompile(f, p256Target) }
func FuzzICS02PrecompileCalldata(f *testing.F)        { fuzzPrecompile(f, ics02Target) }
func FuzzICS20PrecompileCalldata(f *testing.F)        { fuzzPrecompile(f, ics20Target) }
func FuzzGovPrecompileCalldata(f *testing.F)          { fuzzPrecompile(f, govTarget) }
func FuzzSlashingPrecompileCalldata(f *testing.F)     { fuzzPrecompile(f, slashingTarget) }
func FuzzWERC20PrecompileCalldata(f *testing.F)       { fuzzPrecompile(f, werc20Target) }
func FuzzERC20PrecompileCalldata(f *testing.F)        { fuzzPrecompile(f, erc20Target) }

var allTargets = []precompileTarget{
	stakingTarget, distributionTarget, bankTarget, bech32Target, p256Target,
	ics02Target, ics20Target, govTarget, slashingTarget, werc20Target, erc20Target,
}
