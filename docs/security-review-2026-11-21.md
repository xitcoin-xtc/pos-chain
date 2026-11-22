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

## Follow-up: Cosmos SDK v0.54.5 base and dependency refresh — 2026-11-21

This section reviews the second pull request of the window. It closes open
findings 5 and 6 above.

- **SDK fork base.** The fork's single armor commit was cherry-picked without
  conflicts from v0.54.4 onto upstream v0.54.5 as `bb5933a0bf0d…`
  ([xitcoin-xtc/cosmos-sdk#1](https://github.com/xitcoin-xtc/cosmos-sdk/pull/1),
  base branch identical to the upstream tag). Upstream v0.54.5 differs from
  v0.54.4 by the x/feegrant queue migration (consensus version 2 to 3),
  dependency updates and retractions of v0.54.0 to v0.54.4. In the fork, `go mod
  verify`, `go build ./...` and the crypto, keyring, armor-compatibility and
  feegrant tests pass. The requirement now names v0.54.5. The fork has no
  v0.54.5 tag, so the pseudo-version is computed from v0.54.4
  (`v0.54.5-0.20261121191520-bb5933a0bf0d`). Its contents are v0.54.5 plus the
  armor patch. Mirroring the upstream tag into the fork is left to the owner.
- **State-breaking.** The feegrant migration changes state. A new upgrade
  handler, `xitcoin-security-hardening-v1`, runs the release's module
  migrations and adds no store. The testnet keeps its current binary until an
  upgrade is scheduled by its operators.
- **go-ethereum requirement.** It now names v1.17.2, the upstream release that
  the Cosmos Geth fork is based on. The replacement and its sums are
  unchanged. No GitHub advisory covers 1.17.2 at the time of review.
- **Routine updates** from the open Dependabot groups: btcutil 1.2.0 (pulls
  btcec/v2 2.5.0, used here only for HD key derivation, not consensus
  signature checks), ledger-cosmos-go 1.0.1, deque 1.2.3, go-metrics 0.7.0,
  grocksdb 1.11.1 (only with the `rocksdb` build tag), gjson 1.20.3, sjson
  1.2.9, otelgrpc 0.72.0, x/sync 0.25.0, x/text 0.44.0, grpc 1.85.0, client/v2
  2.11.0, pflag 1.1.0, and the test libraries ginkgo, gomega and testify.
  `golang.org/x/crypto` stays at 0.58.0, because 0.59.0 removes the openpgp
  package that the armor compatibility test uses as its oracle.
- **Evidence.** Full root (54 packages) and evmd (21 packages) tests pass
  locally. `govulncheck` root, evmd and binary scans report zero called
  vulnerabilities. The module-level records are the same as above. The fork
  provenance verifies in both modules. The evmd anchor is
  `f449f3ec4b1c28ab6d04dc4fe39b6d4071100444`, which is not GPG-signed.

The review window is unchanged and no scanner exception is added.

## Follow-up: validator admission through authz and direct callers — 2026-11-22

This section reviews the third pull request of the window. It closes open
finding 1 above and a related gap found while fixing it.

- **Finding.** The admission ante handler looked only at top-level messages.
  An authz `MsgExec` wrapping `MsgCreateValidator` or `MsgUnjail` skipped the
  approval check and the 5,000,000 XTC minimum. Because the authz keeper
  accepts `MsgExec` without a grant when granter and grantee are the same
  account, any account could use it. The mint parameter and governance
  safeguards could be bypassed the same way. The legacy gov v1beta1
  `MsgSubmitProposal` (registered by the SDK gov module) was not checked even
  at top level. The staking and slashing precompiles call their message
  servers directly. Both are inactive on the testnet
  (`active_static_precompiles` is empty in the genesis).
- **Fix.** The ante handler walks `MsgExec` recursively, bounded at seven
  levels like the existing authz limiter. It rejects `MsgCreateValidator`,
  `MsgUnjail`, mint `MsgUpdateParams` and gov v1/v1beta1 `MsgSubmitProposal`
  inside `MsgExec`, and rejects `MsgGrant` for those types. After genesis,
  `MsgCreateValidator.MinSelfDelegation` must be at least the on-chain minimum,
  so x/staking keeps enforcing the minimum after creation. Genesis transactions
  keep their historical value, because the testnet gentxs use 1 and must
  replay. A staking hook (`AfterValidatorCreated`) repeats the approval and
  `MinSelfDelegation` checks inside x/staking. This covers every creation path,
  including the precompile. Message-server wrappers apply the same checks in
  front of the staking and slashing precompiles.
- **Residual.** Other routes that could dispatch these messages, such as
  governance execution, the group module and interchain-account hosts, are
  either disabled by the governance safeguard or not wired in this
  application. Unjailing has no x/slashing hook. The direct-signature rule,
  the authz walk and the precompile wrapper cover the wired paths.
- **State-breaking.** Yes: these rules change which transactions are valid and
  belong to the coordinated `xitcoin-security-hardening-v1` upgrade. No
  stored state changes.
- **Evidence.** The new tests cover MsgExec self-grant, nested MsgExec, unjail
  of a revoked validator (direct, wrapped and double-wrapped), governance and
  mint messages through MsgExec, restricted grants, the `MinSelfDelegation`
  rule and its genesis exemption, the staking hook and the precompile wrappers.
  These tests fail against the previous ante handler. Full root and evmd
  tests pass locally. The `govulncheck` root, evmd and binary scans report
  zero called vulnerabilities. The evmd anchor is
  `cbab2828b2c59aab7ef123afc7340de0877af8fc`, which is not GPG-signed.

The review window is unchanged and no scanner exception is added.

## Follow-up: Cosmos EVM security fixes of 2026-11-02 — 2026-11-22

This section closes open finding 2 above. The six fixes released upstream in
Cosmos EVM v0.7.4 and v0.6.4 are ported from the commits on upstream `main`,
or from the v0.7.4 commit where `main` has none. All patches applied without
changes except the CHANGELOG hunks and import paths. Each fix is its own commit
and names the upstream commit. The upstream tests are included.

| Advisory | Upstream | Change |
| --- | --- | --- |
| GHSA-wm4f-qpxp-rch4 (high) | d319ba440b | Precompile call counter is `uint64`, checked before it is incremented; reverted calls still count. A `uint8` counter wrapped and reopened the limit of 20. |
| GHSA-2m6r-q6x4-4w5g (high) | 5f7a18534f | Fee checker clamps the tip cap to the fee cap and avoids the overflowing `baseFee + tipCap`; the mempool rechecker recovers ante panics. |
| GHSA-7xxp-gq2c-x47x (medium) | 03c3ba7854 | EIP-712 sign docs with timeout timestamp, unordered flag, fee granter or fee payer are rejected (current and legacy encoders), because the typed data does not cover them. |
| GHSA-p8x8-4gg6-prcc (high) | bd334caaf8 | Internal EVM calls are capped at the gas remaining on the Cosmos gas meter, and gas used before a StateDB flush or commit error is charged instead of dropped. |
| GHSA-c6xv-pppp-rjw6 (high) | 93b2c25cd7 | Self-destruct of an account that still has delegations, redelegations or unbonding delegations fails, so staking claims cannot be replayed against the not-bonded pool. |
| GHSA-9xwc-4fgh-f64f (medium) | ac0febdc0a (v0.7.4) | Panics in the asynchronous mempool insert queue are recovered and reported as insert errors instead of crashing the node. |

Upstream also released v0.6.5 on 2026-11-13. Its only fix (skip StateDB-commit
error transactions in block RPCs, upstream #1107) is already in this
repository.

- **State-breaking.** Yes: gas accounting, fee checks, EIP-712 validity and
  self-destruct behaviour change execution results. They belong to the
  coordinated `xitcoin-security-hardening-v1` upgrade. No store migration.
- **Evidence.** Full root and evmd tests pass locally, including the upstream
  unit and integration tests for each fix (the integration tests run through
  evmd). The `govulncheck` root, evmd and binary scans report zero called
  vulnerabilities.

The review window is unchanged and no scanner exception is added.
The evmd anchor for this section is `ac5392f6186cf2dc32427bfe1c53220030619901`,
which is not GPG-signed.

## Follow-up: per-network authorities, EVM chain ID and the governance option — 2026-11-22

This section closes open findings 3 and 4 above.

- **Authorities.** `evmd/config/network.go` holds the identity of each public
  network, selected by the Cosmos chain ID and compiled into the binary. The
  x/upgrade and x/bridge authorities and the default validator_incentives
  genesis authority come from it. `xitcoin-testnet-v2-1` keeps the 2-of-3
  multisig `xtc1vza8…`, so testnet behaviour is unchanged. Mainnet
  (`xitcoin`) values are not decided: this release refuses to start on that
  chain ID and can never take the testnet multisig there. InitChain refuses a
  mainnet genesis whose validator_admission, validator_incentives or bridge
  authority is the testnet multisig, and `verify-xitcoin-mainnet-genesis.py`
  refuses the multisig anywhere in a candidate. Other chain IDs (local and
  test chains) keep the previous default. Consensus-parameter
  `AuthorityParams` are not used, because setting them would hand every module
  to one account; that is an owner decision.
- **EVM chain ID.** New nodes get `evm-chain-id` 101089 on the testnet and
  101088 on mainnet. A public network refuses to start with another value,
  since nodes with different EVM chain IDs disagree on transaction validity.
  The value stays in `app.toml`. Devnet (`xitcoin-devnet`, 101090 in the
  identity document) is not yet a known network and keeps the previous
  default.
- **Governance option.** `unsafe-enable-governance-proposal-submission`
  was read from node configuration and changed the ante handler and the
  governance precompile, so nodes with different settings could split. It is
  no longer read. Proposal submission is disabled on every node; only test
  code can re-enable it. A node that had set the option behaves like every
  other node after the upgrade.
- **State-breaking.** Only for a node that had set the removed option. The
  testnet authorities, EVM chain ID and defaults are unchanged. It belongs to
  the coordinated release.
- **Evidence.** New tests cover the network table, the refusal of a wrong EVM
  chain ID and of the undecided mainnet, the mainnet genesis guard, and an
  app that still sets the old option rejecting a governance proposal. Full
  evmd and root tests pass locally. The `govulncheck` gates pass.

The review window is unchanged and no scanner exception is added.
