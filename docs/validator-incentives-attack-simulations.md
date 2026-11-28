# Validator incentives attack simulations

`x/validatorincentives` carries Go native fuzz targets that state the
safety properties of the funded validator incentive release and try to
break them. `go test -tags=test ./x/validatorincentives/...` replays every
target's seed corpus, so each known attack stays a regression test. The
nightly fuzzing workflow (`.github/workflows/fuzz.yml`) explores further
with a time box per target and uploads any failing input as an artifact; it
commits nothing.

## Targets

| Target | Property |
| --- | --- |
| `types.FuzzPolicyMath` | over the whole input domain (amounts in `[0, 2^256-1]`, block counts in `[0, 2^64-1]`), against a `big.Int` reference: no panic; `AnnualizedRewardCapacity`, `PeriodProvision`, `CumulativeProvision` and `DerivedAPYBasisPoints` return the exact floor result; the capacity never exceeds the treasury and the period provision never exceeds the capacity; the cumulative provision is monotonic in the elapsed blocks, equals the period provision at the end of the period, never exceeds it, and releases at most `ceil(provision / period)` per block; an APY that does not fit in 256 bits is an error; a new period state validates, is backed by its treasury snapshot and round-trips through its JSON encoding; negative amounts and invalid parameters are errors |
| `keeper.FuzzIncentiveReleaseSequence` | random sequences of blocks, skipped heights across period ends, replays of `ProcessBlock` in one block, treasury funding, a treasury drained behind the module's back, bonded-stake changes (including zero), and `MsgUpdateParams` by the authority and by outsiders (other accounts, empty, padded, upper-case and other-prefix spellings of the authority), on a real `x/auth` and `x/bank` keeper. After every step: each block releases `floor(P * elapsed / length)` minus what the period already released; distributed <= provision <= treasury snapshot; the treasury covers the remaining provision; every balance of the chain matches the model (the fee collector is the only recipient and the treasury the only source); total supply is unchanged (no mint, no burn); the lifetime total equals the sum of releases; only the exact authority changes the parameters and a change never alters the active period; a failed operation leaves the stores byte-identical; `ProcessBlock` fails only when the treasury was drained, which the chain cannot do (a `BeginBlock` error would halt it) |

`TestPolicyMathHasNoOverflowPanic` pins the only failure the targets found:
the policy math multiplied in 256-bit `sdkmath.Int` before dividing and
panicked on amounts above about 2^192 (2^242 for the release rate), although
the results fit. It now uses exact `big.Int` intermediates; the results are
unchanged wherever the old code did not panic, and the maximum supply
(5.25e27 axtc, about 2^92) keeps every reachable product far below 2^256.

## Running locally

```bash
go test -tags=test ./x/validatorincentives/...                          # seed corpora
go test -tags=test -run='^$' -fuzz='^FuzzIncentiveReleaseSequence$' \
  -fuzztime=5m -fuzzminimizetime=10s ./x/validatorincentives/keeper/     # one target
```

A failing input is written to `testdata/fuzz/<Target>/` in the package
directory. Reproduce it with `go test -tags=test -run='<Target>/<file>'`, fix
the code (never the property), and add the input to the target's seeds.
