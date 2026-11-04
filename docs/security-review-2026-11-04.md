# Current dependency review — 2026-11-04

This is a new technical review for PR55, not independent approval, a release
authorization or a renewal of the September scanner exceptions. The September
assessment and historical qualification remain available unchanged. The current
gate accepts **no scanner advisory exceptions**. A nonzero scanner status,
unexpected dependency, source drift or expired review blocks CI.

## Fresh evidence and scope

- Full root and evmd tests: [run 41774520680](https://github.com/xitcoin-xtc/pos-chain/actions/runs/41774520680)
  at f28a80ad66bdb8fc593c992a89d02ce42677f268, 54 and 21 tested packages.
  Source manifests and Go inputs have not changed since these tests.
- Published, symbol-bearing evmd build: [run 41776289042](https://github.com/xitcoin-xtc/pos-chain/actions/runs/41776289042)
  at 694bc90e1420f34f3bbf4f749d5d73ff32028db3. The module resolves to the exact
  requested commit; its graph matches the checkout, checksums verify and build
  information contains the published version without `(devel)`.
- Fresh source scans of both modules and that binary report two module-level
  advisories, zero imported-package findings and zero function-level findings.
  Unfiltered, plain-text source scans also exit zero in both modules. JSON exit
  status alone is never treated as clearance.
- CI must now repeat full tests and strict source scans on the current checkout,
  exercise published modules at the current PR head, compare dependency graphs
  and scan the symbol-bearing versioned binary. No historical artifact reuse is
  accepted for this current qualification.
- The assessment binds the exact Go source, embedded resources, manifests,
  checksums, this review and the immutable fork provenance record. Source changes
  require a fresh review. No consensus/state, genesis, network identity, live
  node, VPN or migration changes are authorized by this review.

## Advisory dispositions

All six historical identities remain in the provenance record. None is in a
scanner allowlist. This matters because fork coordinates can hide upstream
advisories and a scanner can conservatively report only a module match.

| Advisory | Current evidence and residual limitation |
| --- | --- |
| GO-2023-1821 | Deprecated crisis module absent from production imports and registration; forbidden-import guards remain. |
| GO-2023-1881 | Separate crisis/ConstantFee issue; same absence is checked independently of the advisory identity. |
| GO-2024-2584 | Exact SDK fork and its Go-generated sums remain pinned to the previously source-verified slashing fix; provenance validation is mandatory. |
| GO-2025-3442 | CometBFT 0.39.4 `blocksync/pool.go` contains the upstream peer-height/base regression rejection, removal and ban before updating peer range. The broad `internal/blocksync` module match remains visible; no imported-package or function finding is present. No consensus-library major upgrade is used to silence it. |
| GO-2026-4479 | Obsolete Pion v2 remains transitive but is forbidden in production imports. The reviewed STUN/DTLS v3 paths and immutable Geth fork remain pinned. |
| GO-2026-5932 | Production armor uses maintained ProtonMail code. The unmaintained x/crypto parser is only an explicit compatibility test oracle; it is forbidden in production imports. Its module advisory remains visible. CRC24 checks compatibility, not authentication. |

Primary references: [CometBFT fix](https://github.com/cometbft/cometbft/commit/2cebfde06ae5073c0b296a9d2ca6ab4b95397ea5),
[GO-2025-3442](https://pkg.go.dev/vuln/GO-2025-3442),
[GO-2026-5932](https://pkg.go.dev/vuln/GO-2026-5932).

## Remaining maintenance

Legacy JavaScript compatibility clients must retain their original generations.
Their full dependency audits, including module-level findings above, remain
visible. Updated overrides require actual compatibility tests on disposable
runners; passing a Go gate does not clear those separate audits. This review
does not certify the absence of every vulnerability or authorize a production
chain deployment. The technical review expires on 2026-11-11.
