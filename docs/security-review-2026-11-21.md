# Internal security review — 2026-11-21

This is an **internal, non-independent** technical review of current `main`
(`9e49329f61d32e198d5586e6d8fcd34c3d8369f9`) plus the changes of the pull
request that introduces this document. It was performed under the
Thomas-Cabrit maintainer account as an automated technical review. It is not
an independent audit, a release authorization, a security acceptance or a
deployment permission. The live testnet keeps its current binary; nothing in
this review changes chain state, genesis, keys or servers.

The previous technical review
([2026-11-04](security-review-2026-11-04.md), record kept as
[security-assessment-2026-11-04.json](security-assessment-2026-11-04.json))
expired on 2026-11-11. From that date every CI run that reached the
vulnerability gate failed, as designed. The gate logic is unchanged by this
review: it still accepts **no scanner advisory exceptions**, binds the exact Go
sources, locks, this document and the fork provenance record, and fails on
expiry, drift or any scanner finding.

## Why main needed more than a date change

Since the previous review, three new advisories affect `main`:

| Advisory | Module | Status on main | This PR |
| --- | --- | --- | --- |
| GO-2026-6865 | `github.com/klauspost/compress` 1.18.7 | Reachable (govulncheck symbol finding) | 1.20.5 |
| GO-2026-6859 | Go standard library `net/http`, Go 1.26.10 | Reachable (govulncheck symbol finding) | Go 1.26.11 in all modules and workflows |
| GHSA-q7gw-24fr-5p69 | OpenTelemetry Go `otel/sdk`, `otel/trace` < 1.47.0, `otel/sdk/log` < 1.47.0 | Imported through Cosmos SDK telemetry (`otelconf`) | 1.47.0, `otel/sdk/log` 1.47.0 |

A renewal without these updates would have been dishonest, because the
scanner reported two called vulnerabilities on `main`.

## Changes reviewed

1. **Organisation rename.** The module path, all imports, the Cosmos SDK and
   Geth fork replacement paths, the `networks` submodule URL, scripts and
   documentation move from the unclaimed `xitcoin-org` name to
   `xitcoin-xtc`. Resolving modules through an unclaimed organisation name is
   a dependency-hijack risk; this closes it. Fork revisions are unchanged
   (`52ff14a25bee…` and `e09f79643cd4…`); only their `h1:` archive sums
   change, because the module path is part of the archive. Links to upstream
   Cosmos EVM pull requests and issues point to `github.com/cosmos/evm` again.
   The regenerated gogoproto descriptors were decoded and compared one by one:
   each differs from the previous descriptor only in `go_package`. `x/bridge`
   is limited to import-path lines.
2. **Dependency patches.** Effective root and evmd graphs were compared with
   `main`. Changed modules: klauspost/compress, the OpenTelemetry family
   (core 1.47.0, log exporters 0.23.0, `metric/x` 0.69.0, otelgrpc and
   otelhttp 0.70.0), grpc-gateway/v2 2.30.0, protobuf 1.36.12, genproto
   (googleapis/api, rpc), httpsnoop 1.1.0 and testify 1.12.1. go-openapi and
   oapi-codegen modules appear in the module graph through exporter
   requirements but are not imported by any package. No consensus code,
   module parameter, store layout or genesis changes.
3. **evmd root anchor.** evmd keeps consuming the published root library at an
   exact commit, now `cf8a79a7f4950e98f6c609e71b92f9b26222d659`
   (`v0.1.0-testnet.4.0.20261121185321-cf8a79a7f495`). That commit is on the
   pull request branch; it carries the rename and dependency changes and no
   other Go change. It is not GPG-signed.

## Evidence collected for this review

- Full root tests (54 packages) and evmd tests (21 packages) pass locally with
  Go 1.26.11, `-tags=test -count=1`, the same package counts as the previous
  review.
- `govulncheck` v1.7.0 source scans of root and evmd and a binary scan of the
  symbol-bearing `xitcoind` built with `make build COSMOS_BUILD_OPTIONS=nostrip`
  report **zero called vulnerabilities** and zero imported-package findings.
  Two module-level records remain visible: GO-2026-5932 and GO-2025-3442.
- `scripts/verify-go-fork-provenance.py` passes in both modules with the new
  sums; `go list -deps` shows no `x/crisis`, `golang.org/x/crypto/openpgp`,
  `pion/dtls/v2` or `pion/stun/v2` package in either production graph.
- CI must repeat all of this on the pull request head: full tests, strict
  source and binary scans, the published-module comparison and the speedtest
  qualification. Local evidence does not replace it.

## Historical dispositions re-examined

| Advisory | Re-examination |
| --- | --- |
| GO-2023-1821, GO-2023-1881 | x/crisis is still neither imported nor registered; the import guards remain. |
| GO-2024-2584 | Same SDK fork commit as the previous review; the slashing fix is in the v0.54 line. |
| GO-2025-3442 | CometBFT is still 0.39.4; `blocksync/pool.go` still rejects peers that report a lower base or height and bans them. |
| GO-2026-4479 | Pion DTLS v2 is still only a transitive module requirement and absent from production imports. |
| GO-2026-5932 | Production armor still uses ProtonMail code; `golang.org/x/crypto/openpgp` is only a test oracle. An upstream x/crypto pseudo-version now carries a fix; adopting it is left to the dependency refresh. |

## Known open findings on main

These were known when this review was written. They are **not** fixed by this
pull request and are not accepted risks; each is scheduled as a separate pull
request that must rebind this assessment inside the same window.

1. `x/validatoradmission` checks only top-level messages. An authz `MsgExec`
   (including a self-grant, which the SDK accepts when granter equals grantee)
   wrapping `MsgCreateValidator` or `MsgUnjail` skips the approval check and the
   5,000,000 XTC minimum. The staking precompile path also calls
   `CreateValidator` (inactive on the testnet).
2. Cosmos EVM security fixes released on 2026-11-02 (v0.7.4 / v0.6.4) are not
   yet ported: GHSA-wm4f (precompile call counter), GHSA-2m6r (fee checker
   overflow), GHSA-7xxp (EIP-712 sign-doc fields), GHSA-p8x8 (internal EVM call
   gas cap); GHSA-c6xv and GHSA-9xwc still need to be checked.
3. The node-local option `unsafe-enable-governance-proposal-submission` changes
   ante and governance-precompile behaviour, so nodes with different settings
   can disagree on transaction validity.
4. The testnet administrative multisig is hard-coded as the authority for
   x/upgrade, the bridge and the validator-incentives default, and the CLI
   defaults to the testnet EVM chain id.
5. Upstream retracted Cosmos SDK v0.54.0 to v0.54.4; the fork is based on
   v0.54.4 and lacks the v0.54.5 feegrant fix. Moving the base is
   state-breaking (feegrant store migration).
6. `go.mod` requires `github.com/ethereum/go-ethereum` v1.16.9 while the
   replacement is the Cosmos Geth v1.17.2 fork, so scanners that read only the
   requirement report GHSA-689v-6xwf-5jf3.

## Decision

The six historical dispositions remain technically valid for these exact
inputs. No scanner exception is accepted. The review window is
**2026-11-21 to 2026-11-28 UTC**, the same seven-day length as the previous
review, because the open findings above are being changed in the following
days. There is no automatic renewal; after 2026-11-28 the gate fails again
until a new review is written.
