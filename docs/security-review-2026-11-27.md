# Internal security review — 2026-11-27

This is an **internal, non-independent** technical review of current `main`
(`958f6fd1c7b8cec4d7c224a08bdc7775a6499c33`) plus the changes of the pull
request that introduces this document. It was performed under the
Thomas-Cabrit maintainer account as an automated technical review. It is not
an independent audit, a release authorization, a security acceptance or a
deployment permission. The live testnet keeps its current binary; nothing in
this review changes chain state, genesis, keys or servers.

The previous technical review
([2026-11-21](security-review-2026-11-21.md), record kept as
[security-assessment-2026-11-21.json](security-assessment-2026-11-21.json))
runs until 2026-11-28. This review replaces it before it expires. The gate
logic is unchanged: it still accepts **no scanner advisory exceptions**, binds
the exact Go sources, locks, this document and the fork provenance record, and
fails on expiry, drift or any scanner finding.

## Why main needed more than a date change

Since the previous review, one advisory affecting `main` changed:

| Advisory | Module | Status on main | This PR |
| --- | --- | --- | --- |
| GO-2026-6278 (GHSA-w67g-5rqw-f597) | `github.com/gorilla/websocket` v1.5.3 | Reachable (govulncheck symbol finding in root, evmd and speedtest) | `v1.5.4-0.20240701034025-d67f41855da4`, the upstream fix commit |

The Go vulnerability database revised GO-2026-6278 on 2026-11-23. The GitHub
advisory says the fix (commit `d67f41855da4`, mask keys from `crypto/rand`)
shipped in v1.5.3. It did not: the v1.5.3 tag was cut from a branch that
reverted to v1.5.0 (merge base `ce903f6d1d96`, "Reverts to v1.5.0"), and its
`conn.go` still builds client frame mask keys from `math/rand.Uint32`. The
revised record marks `[1.5.3, d67f41855da4)` as affected. Both facts were
checked in the module sources. Since 2026-11-24 the vulnerability gate fails
on pull requests against `main` for this reason alone (for example run
45491367339 of #86), whatever the review date.

On `main`, govulncheck reports calls from `rpc/websockets.go` (the JSON-RPC
WebSocket server and its connections), from the CometBFT JSON-RPC client and
server started by `server/start.go`, from the Geth RPC client and through the
mempool shutdown path: 13 traces in root, 15 in evmd and 13 in speedtest.
Mask keys protect frames that a client sends. With predictable keys, an
attacker who can influence those frames can try to confuse intermediaries
(proxies, caches) that sit between the client and the server. The node acts as
a WebSocket client towards the CometBFT RPC and in the Geth RPC client.

A renewal without this update would have been dishonest, because the scanner
reports a called vulnerability on `main`.

## Changes reviewed

1. **gorilla/websocket.** No tagged release exists after v1.5.3. v1.5.2 is
   outside the affected range, but upstream retracts it ("tag accidentally
   overwritten"), and CometBFT, the Cosmos SDK, ibc-go and libp2p require
   v1.5.3, so minimal version selection cannot go below it. The root module
   therefore requires the fix commit itself; evmd, speedtest and systemtests
   inherit it, and the JSON-RPC simulator requires it directly. The commit is
   14 commits after v1.5.3 on upstream `main`. Besides the mask key, it uses
   `golang.org/x/net/proxy` instead of a bundled copy (x/net is already in
   the graph), selects a subprotocol in the client's order (RFC 6455), answers
   426 with an `Upgrade` header when the upgrade token is missing (was 400),
   hijacks through `http.ResponseController`, refactors `Dialer.DialContext`,
   requires Go 1.20 and retracts v1.5.2. `rpc/websockets.go` sets no
   subprotocols. Nothing here changes consensus.
2. **Dependabot #75 (go-minor-and-patch, root).** Applied in full:
   - `golang.org/x/crypto` 0.58.0 → 0.59.0. The openpgp packages are carved
     out into the deprecated module `golang.org/x/crypto/openpgp`
     v0.1.0-deprecated, with byte-identical sources. This is the x/crypto fix
     version for GO-2026-5932. The armor compatibility test
     (`crypto/hd/armor_compat_test.go`) keeps its historical oracle through
     that module. No production package imports it; the gate still rejects a
     production import of `golang.org/x/crypto/openpgp`. The 0.58.0 hold
     described in the previous review is no longer needed. The other changes
     are security hardening in `x/crypto/ssh` and `ssh/knownhosts` (certificates
     with an empty principal list rejected, rejected public-key queries counted
     towards `MaxAuthTries`, non-printable banner characters escaped,
     peer-controlled strings quoted in errors, `OpenChannel` failing on a lost
     connection, DSA removed), an `x509roots` bundle update and a loong64
     blake2s implementation. No root or evmd production package imports
     `ssh`, `knownhosts` or `x509roots`. `blake2s` is imported, but its amd64 and
     arm64 code does not change. No advisory is attached to these changes.
   - `golang.org/x/net` 0.62.0 → 0.63.0. The only functional change is in
     `http2/transport_wrap.go` (connection state with the net/http unified
     pool), which builds only with Go 1.27. Go 1.26.11 builds compile the same
     code as before. No security fix.
   - `golang.org/x/sys` 0.50.0 and `golang.org/x/term` 0.48.0 come with them;
     platform additions, no advisory.
   - `tidwall/gjson` 1.20.5 and `tidwall/sjson` 1.2.11. They are used by
     `ethereum/eip712` to build typed data from sign documents, so signature
     validity depends on them. gjson changes only `parseUint` (integer
     conversion of 20-digit strings, leading zeros, numeric path parts); the
     EIP-712 code uses `ValidBytes`, `ParseBytes`, `Get("msgs")`, `Type`, `Raw`
     and `ForEach`, never integer conversion or numeric paths. sjson changes
     the deletion of keys that contain an escaped quote; the EIP-712 code only
     deletes the fixed key `msgs`. The typed data is therefore unchanged, and
     the EIP-712 tests pass. Not state-breaking.
   - `onsi/gomega` 1.44.0: test library only.
3. **Dependabot #74 (gomega in evmd).** Applied.
4. **Test-only modules.** `tests/speedtest` and `tests/systemtests` replace
   the root module with the local checkout, so `go mod tidy` moves them to the
   same versions; without it, systemtests reports `updates to go.mod needed`.
   The JSON-RPC simulator (`tests/jsonrpc/simulator`) gets the WebSocket fix,
   x/crypto 0.59.0 and excelize v2.11.1, which closes Dependabot alert #400
   (GHSA-fq3v-74gv-27gm, open since 2026-11-10). None of these modules is
   part of the node binary, and earlier reviews did not cover them.
5. **Gate.** `scripts/run-govulncheck.sh` and its simulation require
   `golang.org/x/crypto` v0.59.0 instead of v0.58.0, and pin the new evmd
   anchor sums. No check is removed or relaxed.
6. **evmd root anchor.** evmd consumes the published root library at
   `caab55b4c1b6122e24ade842d17476929a8d649b`
   (`v0.1.0-testnet.4.0.20261127035027-caab55b4c1b6`), the first commit of
   this pull request. It carries only the `go.mod` and `go.sum` changes above
   and no Go source change. It is not GPG-signed.

No consensus code, module parameter, store layout or genesis changes. This
pull request adds no state-breaking change.

## Checked with no change needed

- **Go toolchain.** No Go 1.26 release after 1.26.11 (2026-11-10, released
  together with 1.27.4). The toolchain stays at 1.26.11.
- **Upstream projects since 2026-11-21.** Cosmos SDK: no new advisory (latest
  releases v0.54.5 and v0.55.1 of 2026-11-14). Cosmos EVM: no advisory after
  the 2026-11-02 set, which is ported; the v0.6.5 fix is present. CometBFT: no
  new advisory and no release in the 0.39 line after v0.39.4. ibc-go: no
  advisory affecting v11.2.0.
  go-ethereum: the latest advisory (2026-02-17) affects only versions below
  1.17.0; upstream releases up to v1.17.10 (2026-10-23) carry no advisory
  covering 1.17.2, the base of the Cosmos Geth fork used here.
- **Go vulnerability database.** The 139 records modified from 2026-11-20 to
  the database update of 2026-11-24 were matched against the root, evmd and
  speedtest module graphs. GO-2026-6278 is the only one that affects a module
  version that packages import. GO-2026-6974 (GHSA-q7gw-24fr-5p69) and
  GO-2026-6975 (GHSA-rrfc-gjm9-cgrj, W3C baggage parsing) are fixed in
  OpenTelemetry 1.47.0, which the previous review already adopted.
  GO-2026-6940 (`github.com/gobwas/ws` v1.0.2) and six `nats-server/v2`
  v2.8.4 records match module versions that appear only in the module graph,
  through the `go.mod` files of `nhooyr.io/websocket` and `go-kit/kit`.
  `go mod why -m` shows that no module needs them, no package (including
  tests) imports them and no binary contains them.
- **GitHub advisories** in the Go ecosystem published or updated since
  2026-11-20: none affects a module in these graphs. The only open Dependabot
  alert (#400) is closed by item 4.

## Evidence collected for this review

All local runs use Go 1.26.11 (darwin/arm64) and govulncheck v1.7.0 with the
database of 2026-11-24.

- Full root tests: 54 packages ok. Full evmd tests: 22 packages ok. Both with
  `-tags=test -count=1`, exit 0. evmd has 22 test packages on `main` too; the
  previous record kept the earlier count of 21 after the per-network authority
  change added the `evmd/config` tests. The speedtest, systemtests and
  simulator modules compile and vet; systemtests needs built binaries to run
  and is not run locally.
- `govulncheck` source scans of root, evmd and speedtest and a binary scan of
  the symbol-bearing `xitcoind` built with `make build COSMOS_BUILD_OPTIONS=nostrip`
  report **zero called vulnerabilities** and zero imported-package findings.
  One module-level record remains: GO-2025-3442 (CometBFT). GO-2026-5932 is no
  longer reported, because x/crypto 0.59.0 no longer contains openpgp. The
  binary's build information shows `gorilla/websocket`
  v1.5.4-0.20240701034025-d67f41855da4, x/crypto 0.59.0, x/net 0.63.0 and the
  root library at the anchor above. The simulator scan is clean.
- `scripts/verify-security-assessment.py`, `scripts/run-govulncheck.sh` from
  root and evmd and on the binary, `scripts/test-govulncheck-gate.py` (9 tests)
  and `scripts/verify-go-fork-provenance.py` in both modules pass. `go mod
  verify` passes in every module. `go list -deps` shows no `x/crisis`,
  `golang.org/x/crypto/openpgp`, `pion/dtls/v2` or `pion/stun/v2` package in
  the production graphs.
- `scripts/qualify-speedtest.py` passes locally on the pull request head.
  `scripts/qualify-current-go.py` reads `/proc` and runs only on the Linux CI
  runners.
- CI must repeat all of this on the pull request head: full tests, strict
  source and binary scans, the published-module comparison and the speedtest
  qualification. Local evidence does not replace it.

## Historical dispositions re-examined

| Advisory | Re-examination |
| --- | --- |
| GO-2023-1821, GO-2023-1881 | x/crisis is still neither imported nor registered; the import guards remain. |
| GO-2024-2584 | Same SDK fork commit (`bb5933a0bf0d…`, v0.54.5 base) as the previous review; the slashing fix is in the v0.54 line. |
| GO-2025-3442 | CometBFT is still 0.39.4; `blocksync/pool.go` still rejects peers that report a lower base or height and bans them. |
| GO-2026-4479 | Pion DTLS v2 is still only a transitive module requirement and absent from production imports. |
| GO-2026-5932 | Production armor still uses ProtonMail code. x/crypto 0.59.0 no longer contains openpgp; the carved-out module is used only by the armor compatibility test. |

## Known open findings

None at the time of writing. Two operational notes, which are not findings
of this code review:

- The live testnet binary predates this review and the 2026-11-21 changes, so
  it still contains `gorilla/websocket` v1.5.3. Deploying the current code
  requires the coordinated `xitcoin-security-hardening-v1` upgrade. That
  decision belongs to the testnet operators.
- The WebSocket requirement is a pseudo-version of an upstream commit. It
  should move to the first upstream tag that contains `d67f41855da4` when one
  is published.

## Decision

The six historical dispositions remain technically valid for these exact
inputs. No scanner exception is accepted. The review window is
**2026-11-27 to 2026-12-04 UTC**, the same seven-day length as the previous
reviews. There is no automatic renewal; after 2026-12-04 the gate fails again
until a new review is written.
