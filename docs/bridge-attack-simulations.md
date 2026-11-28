# Bridge attack simulations

`x/bridge` carries Go native fuzz targets that state the bridge's safety
properties and try to break them. `go test -tags=test ./x/bridge/...` replays
every target's seed corpus, so each known attack stays a regression test. The
`Bridge fuzzing` workflow (`.github/workflows/fuzz.yml`) explores further every
night with a time box per target and uploads any failing input as an artifact;
it commits nothing.

## Targets

| Target | Property |
| --- | --- |
| `types.FuzzParseAmount` | `ParseAmount` accepts exactly the canonical decimals in `[1, 2^256-1]`, returns their value, and both historical parsers agree on every accepted input |
| `types.FuzzAmountValidationAgrees` | message validation, attestation validation, every route limit and the genesis outstanding amount accept exactly the canonical amounts |
| `types.FuzzAttestationDigest` | no panic; same input, same digest; the payload follows the documented 0x00-separated encoding with exactly 9 fields; whitespace or NUL variants of a field are rejected or change the digest; route IDs are ASCII `[a-z0-9_-]` |
| `types.FuzzSourceDepositKey`, `types.FuzzSourceDepositKeyArbitrary` | case, `0x`/`0X` and whitespace spellings of one source reference map to the documented replay key or are rejected |
| `types.FuzzApprovals` | for attestation, control and guardian-pause approvals: recovery bytes 0, 1, 27 and 28 of one signer count once, high-s twins, wrong recovery IDs, corrupted or wrong-length signatures, unconfigured signers, duplicates, and signatures for another chain ID or another domain never count toward 2-of-3 (or toward the guardian) |
| `types.FuzzRouteConfigValidate` | an accepted configuration has an ASCII route ID, limits that `ParseAmount` reads, and signer and guardian addresses that resolve to the address they spell |
| `types.FuzzMessageDecoding` | decoding any bytes as a bridge message (directly or through `Any`), as genesis or as a route configuration, then validating it and computing its sign bytes and digests, never panics |
| `keeper.FuzzAmountAgreement` | for any amount string, `ValidateBasic`, the limits, the mint and the burn reject it together or use the same number, and the outbound event carries it |
| `keeper.FuzzBridgeOperationSequence` | random sequences of inbound transfers, replays, tampered approvals, outbound transfers, guardian pauses (and their replays), resumes and configuration updates with next, stale or skipped control nonces, and block-time warps across UTC days, on a real `x/auth` and `x/bank` keeper; after every step: outstanding = mints - burns, outstanding <= cap, supply <= maximum supply, the day's usage equals the accepted transfers of that UTC day and no transfer pushes it above the limit, at most one mint per deposit, no mint or burn while paused or disabled, the control nonce follows the model, the exported genesis validates (including every attestation and outbound transfer record), every accepted mint and burn leaves a query record with the minted or burned values, and each operation is accepted exactly when the model predicts |

## Running locally

```bash
go test -tags=test ./x/bridge/...                       # seed corpora
go test -tags=test -run='^$' -fuzz='^FuzzApprovals$' \
  -fuzztime=5m -fuzzminimizetime=10s ./x/bridge/types/   # one target
```

A failing input is written to `testdata/fuzz/<Target>/` in the package
directory. Reproduce it with `go test -tags=test -run='<Target>/<file>'`, fix
the code (never the property), and add the input to the target's seeds.
