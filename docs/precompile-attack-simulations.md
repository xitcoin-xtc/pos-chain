# Precompile attack simulations

`evmd/tests/integration/precompiles/attack` attacks every Cosmos precompile
of the application (staking, distribution, bank, bech32, p256, ICS-02,
ICS-20, gov, slashing, the native WERC20 precompile and a dynamic ERC-20
precompile) on a real `evmd` application built as release binaries build it
(executable governance proposals disabled). `go test -tags=test ./...` in
`evmd` replays the seed corpora and runs the contract attacks, so each known
attack stays a regression test. The nightly fuzzing workflow
(`.github/workflows/fuzz.yml`) explores the calldata targets further with a
time box per target and uploads any failing input as an artifact; it commits
nothing.

## Calldata targets

`Fuzz<Precompile>PrecompileCalldata` (one per precompile above). Each input
selects a call kind (CALL, STATICCALL, DELEGATECALL, CALLCODE), optional
value, a calldata (a selector of the precompile's ABI with arguments drawn
from addresses, strings and amounts that exist on the chain, raw argument
bytes, a truncated encoding, or arbitrary bytes) and a gas limit below, at
and above the base cost. The chain is frozen with a delegation, an unbonding
delegation, accrued rewards, a proposal in its voting period with votes, and
an IBC voucher balance; every input runs twice on a fresh cache of that
state, with the context an EVM transaction has after the ante handler.

| Property | Meaning |
| --- | --- |
| no panic | a precompile reverts or fails, it never panics (a panic aborts the whole transaction and cannot be caught) |
| base cost | the call consumes at least `RequiredGas(input)`; with less gas it fails with out-of-gas |
| failed call | a failed or reverted call leaves every persistent KV store byte-identical, Cosmos and EVM state alike |
| read-only call | STATICCALL, DELEGATECALL and CALLCODE (run read-only by the EVM) leave every store byte-identical |
| view method | a view method called without value leaves every store byte-identical, whatever the call kind |
| value | value sent to a method that is not payable never stays with the precompile |
| determinism | the same input gives the same output, gas, error and state |

`TestPrecompileTransactionMethodsMatchTheABI` checks that each precompile's
`IsTransaction` names exactly the methods its ABI declares state-changing.

## Contract attacks

The contracts are in `precompiles/testutil/contracts` (`PrecompileAttacker`,
`StakingSelfDestructor`, `StakingSelfDestructorFactory`, `GasBurningERC20`);
each attack runs through signed transactions included in committed blocks,
and checks that the EVM and the bank agree on balances, that the supply
equals the sum of balances, and that the staking pools hold exactly the
tokens they back.

| Test | Attack |
| --- | --- |
| `TestRevertAfterPrecompileCallLeavesNoTrace` | delegate (and send native value), then revert, uncaught and caught: nothing persists but the delegation outside the reverted frames |
| `TestReentrantFramesKeepCosmosAndEVMStateInSync` | six re-entrant frames, each delegating and sending value, with reverts chosen by a mask: exactly the frames whose ancestry did not revert persist |
| `TestPrecompileCallLimitCannotBeBypassed` | GHSA-wm4f-qpxp-rch4: 19 and 20 calls before a delegation, 20 calls in reverted frames, 261 and 515 attempts: the limit of 20 holds and the counter never wraps |
| `TestSelfDestructWithStakingPositionsFails` | GHSA-c6xv-pppp-rjw6: a CREATE2 contract that delegates or unbonds and self-destructs in the same transaction is refused; no staking claim outlives its account |
| `TestViewCallsDoNotChangeCosmosState` | a transaction made of view calls (gov tally, distribution rewards), and a direct tally call, deletes no vote and increments no validator period |
| `TestInternalCallGasIsCappedAndCharged` | GHSA-p8x8-4gg6-prcc: a registered token whose `transfer` burns gas and then makes the StateDB commit fail: the gas is charged, and the internal call never gets more gas than the transaction has |

## Findings fixed with these simulations

| Finding | Fix |
| --- | --- |
| gov `getTallyResult` on a proposal in its voting period deleted the votes it counted; distribution `delegationRewards` and `delegationTotalRewards` incremented validator periods, even from a STATICCALL | every precompile query runs on a context whose writes are reverted (`cmn.QueryContext`) |
| ICS-02 `updateClient` with an empty client message panicked | rejected with `ErrInvalidHeader` |
| value sent to a non-payable WERC20 method stayed at the WERC20 address | rejected with `ErrCannotReceiveFunds` |
| a DELEGATECALL with empty calldata to any stateful precompile, and any DELEGATECALL to an ERC-20 precompile, panicked on the nil call value | `cmn.CallValue` treats it as zero |

All four change transaction outcomes and ship with the
`xitcoin-security-hardening-v2` upgrade.

## Running locally

```bash
cd evmd
go test -tags=test ./tests/integration/precompiles/attack/                # seeds and contract attacks
go test -tags=test -run='^$' -fuzz='^FuzzGovPrecompileCalldata$' \
  -fuzztime=5m -fuzzminimizetime=20s ./tests/integration/precompiles/attack/  # one target
```

A failing input is written to `testdata/fuzz/<Target>/` in the package
directory. Reproduce it with `go test -tags=test -run='<Target>/<file>'`, fix
the code (never the property), and add the input to the target's seeds.
Solidity sources are compiled with `make contracts-compile` (solc 0.8.20,
optimizer 100 runs, via IR).
