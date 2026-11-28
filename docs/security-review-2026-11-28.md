# Internal security review — 2026-11-28

This is an **internal, non-independent** technical review of current `main`
(`f0c9a90aea720aeac97a9f6b9eac1f304f89ac78`) plus the changes of the pull
request that introduces this document. It was performed under the
Thomas-Cabrit maintainer account as an automated technical review. It is not
an independent audit, a release authorization, a security acceptance or a
deployment permission. The live testnet keeps its current binary; nothing in
this review changes chain state, genesis, keys or servers.

The previous technical review
([2026-11-27](security-review-2026-11-27.md), record kept as
[security-assessment-2026-11-27.json](security-assessment-2026-11-27.json))
runs until 2026-12-04. This review replaces it early because this pull request
changes Go sources and locks, and the gate binds the record to their exact
digest. The gate logic is unchanged: it still accepts **no scanner advisory
exceptions**, binds the exact Go sources, locks, this document and the fork
provenance record, and fails on expiry, drift or any scanner finding.

## Why this renewal

`main` itself has no reachable advisory. On `f0c9a90`, govulncheck reports
zero called vulnerabilities and zero imported-package findings in root, evmd
and speedtest; the only record is the module-level GO-2025-3442 (CometBFT)
already covered by the historical dispositions.

The renewal is needed because two changes touch the bound inputs: Dependabot
#90 (`go.mod`, `go.sum`) and the fix for the flaky WebSocket test (`.go`
files). Either one changes `go_source_sha256`, so both ship together with the
rebound record.

## Changes reviewed

1. **WebSocket subscriptions (flaky test).**
   `TestSubscribeNewHeadsReturnsCanonicalBlockHash` failed once on GitHub, in
   run 45954534481 (Xitcoin CI on `main` at `0ae7422`, 2026-11-27 06:55 UTC):
   `read tcp …: i/o timeout` at `rpc/websockets_test.go:130`, after the 2 s
   read deadline for the `newHeads` notification. The race is in the server,
   not in the test:
   - **Start position decided too late.** `eth_subscribe` started the
     subscriber goroutine and returned the subscription ID at once. The
     goroutine called `Stream.Subscribe`, which read the stream from offset
     `-1`, meaning "from the end as of my first read". An event added between
     the response and that first read was therefore skipped. The test pushes
     its block right after it reads the response, so it waited for a header
     that had been skipped. `newHeads`, `logs` and `newPendingTransactions`
     now take the start position with `ReadNonBlocking(-1)` before the
     response is written, as the filter API (`eth_newBlockFilter` and the
     others) already does, and pass it to `Stream.Subscribe`, which now takes
     the offset as a parameter. Its only callers are `rpc/websockets.go` and
     the stream tests; evmd does not call it.
   - **Missed wake-up.** `Stream.ReadBlocking` released its read lock and
     only then let `Cond.Wait` take the channel that the next `Add` closes.
     An `Add` in between closed the previous channel, and the reader slept
     until the following `Add`. `Cond.Wait` now takes the channel first and
     releases the lock itself, as `sync.Cond` does; `Add` broadcasts while it
     holds the write lock, so no broadcast can fall between the check and the
     wait.

   For a client, the first effect lost the first header, log or pending
   transaction after subscribing; the second delayed an event until the next
   one, which on a halted chain means never. Neither affects consensus. The
   change adds no lock and holds none for longer; the read lock is still
   released while waiting.

   The test keeps every assertion and its 2 s deadline; only the comment that
   relied on the ack giving the goroutine time to start changes. Two stream
   tests are added: `TestStreamSubscribeFromOffset` (an item added after the
   offset is taken and before the first read is delivered) and
   `TestCondWaitSeesBroadcastAfterUnlock` (a broadcast made right after the
   lock is released is seen; with the old order the test fails after its
   5 s guard, which was checked by restoring that order locally).
2. **Dependabot #90 (go-minor-and-patch, root).** `tidwall/gjson` 1.20.5 →
   1.20.6, applied as proposed (the same `go.mod` and `go.sum` lines). The
   release contains one change, upstream commit `f004fea` ("Fix join emit bad
   json", upstream issue 407): with `@join:{"preserve":true}`, an empty object
   in the input array no longer leaves a stray comma (`[{}, {"a":1}]` gave
   `{,"a":1}`, now `{"a":1}`). The change is in the array branch of
   `modJoin`, which runs only for a path that uses the `@join` modifier.
   `ethereum/eip712`, which builds the typed data from sign documents and on
   which signature validity depends, uses `ValidBytes`, `ParseBytes`, `Get`
   with the fixed keys `msgs`, `msgN` and `type`, `Get` with the object keys
   returned by `Map()`, `Array`, `Type`, `Raw` and `ForEach`. Those object
   keys are proto field names of the amino JSON sign document, made of
   letters, digits and underscores: no message type in this chain has a map
   field (the only `map<…>` fields in the protos of the Cosmos SDK fork,
   ibc-go v11 and this repository are in autocli queries and the
   packet-forward genesis). No path the EIP-712 code builds can therefore
   contain a modifier. The legacy encoder builds its typed data by
   reflection and does not use gjson. The typed data is unchanged, and the
   EIP-712 tests pass. Not state-breaking.
   gjson 1.20.7 (published 2026-11-25) completes the same fix: `unwrap`,
   which `@join` and `@flatten` use, also trims whitespace inside the
   brackets, so an object written `{ }` counts as empty too. It is not part
   of #90 and is not taken here; the reasoning above applies to it unchanged.
3. **Test-only modules.** `tests/speedtest` and `tests/systemtests` replace
   the root module with the local checkout, so `go mod tidy` moves them to
   gjson 1.20.6. The JSON-RPC simulator does not use gjson and is unchanged.
4. **evmd root anchor.** evmd consumes the published root library at
   `303b2325e443b770f9c779c6ed0810d7dc8d7e64`
   (`v0.1.0-testnet.5.0.20261128025345-303b2325e443`), the second commit of
   this pull request. It carries the WebSocket fix and gjson 1.20.6; evmd's
   `go.mod` and `go.sum` change only for the anchor and gjson. The
   pseudo-version is now derived from the `v0.1.0-testnet.5` tag. The anchor
   is not GPG-signed.
5. **Gate.** `scripts/run-govulncheck.sh` and its simulation pin the new evmd
   anchor sums; the simulated dates follow the new window. The module pins
   are unchanged, and no check is removed or relaxed.

No consensus code, module parameter, store layout or genesis changes. This
pull request adds no state-breaking change.

## Follow-up from the 2026-11-27 review

- **gorilla/websocket tag.** There is still no upstream tag after v1.5.3: the
  Go module proxy lists v1.0.0 to v1.5.3, and the GitHub tags and releases
  end with v1.5.3 (`ce903f6d1d96`, 2024-06-14). The requirement stays at the
  fix commit `v1.5.4-0.20240701034025-d67f41855da4`, and the follow-up stays
  open.
- **Testnet binary.** Since height 1,456,000 (2026-11-27) the testnet runs
  `v0.1.0-testnet.5` (revision `1924dad`, see
  [testnet-status.md](testnet-status.md)), which contains the 2026-11-27
  changes, including the WebSocket fix. It does not contain this pull request.

## Checked with no change needed

- **Go toolchain.** No Go 1.26 release after 1.26.11 (2026-11-10, released
  together with 1.27.4); go.dev lists no later version. The toolchain stays
  at 1.26.11.
- **Upstream projects since 2026-11-27.** No new release or advisory.
  Cosmos SDK: latest releases v0.54.5 and v0.55.1 (2026-11-14); latest
  repository advisory from 2025. Cosmos EVM: latest release v0.6.5
  (2026-11-13); latest advisories are the 2026-11-02 set, which is ported.
  CometBFT: no release in the 0.39 line after v0.39.4; the latest repository
  advisory change is the 2026-03-09 update of GHSA-22qq-3xwm-r5x4
  (GO-2025-3442). ibc-go: latest release v11.2.0
  (2026-07-15); no advisory since 2025. go-ethereum: latest release v1.17.10
  (2026-10-23); latest advisory 2026-02-17, affecting only versions below
  1.17.0.
- **Go vulnerability database.** The database index at vuln.go.dev was last
  modified on 2026-11-24 at 15:56 UTC, the same database the 2026-11-27
  review used. No record has been added or changed since. Its 139 records
  modified from 2026-11-20 were matched against the graphs on 2026-11-27, and
  the scans below were run again on it.
- **GitHub advisories.** The newest reviewed Go advisory in the GitHub
  database was updated on 2026-11-24 at 22:26 UTC, before the 2026-11-27
  review; nothing was published or updated since, and no unreviewed Go
  advisory was updated after 2026-11-24. The 24 reviewed Go advisories
  published or updated since 2026-11-20 were matched again against the root,
  evmd, speedtest, systemtests and simulator module graphs. Only four
  `nats-server/v2` advisories match its v2.8.4, a module-graph-only entry;
  `go mod why -m` still shows that no module needs it (nor `gobwas/ws`). The
  repository advisories of the Cosmos SDK, Cosmos EVM, CometBFT, ibc-go,
  go-ethereum, gorilla/websocket and gjson have no new entry. No Dependabot
  alert is open for the Go modules; #400 is fixed since 2026-11-27.

## Evidence collected for this review

All local runs use Go 1.26.11 (darwin/arm64) and govulncheck v1.7.0 with the
database of 2026-11-24.

- Full root tests: 54 packages ok. Full evmd tests: 22 packages ok,
  including the EIP-712 suite in `evmd/tests/integration`. Both with
  `-tags=test -count=1`, exit 0.
- Flaky test, on the unmodified test:
  - `go test -race -count=300 -run TestSubscribeNewHeadsReturnsCanonicalBlockHash ./rpc/...`:
    300 of 300 passed, exit 0; the same with `-tags=test`: 300 of 300.
  - Under CPU load (two processes × 1,000 iterations × `-cpu 1,4`, with
    eight busy-loop processes alongside):

    | Build | Runs | Failures (all `i/o timeout` at line 130) |
    | --- | --- | --- |
    | `main` `f0c9a90` | 4,000 | 13 |
    | only the wake-up fix | 4,000 | 11 |
    | only the start-position fix | 4,000 | 0 |
    | this pull request | 8,000 | 0 |

    The start position explains the failures seen; the missed wake-up has a
    window of a few instructions and did not show up on its own.
  - With a temporary 200 ms delay (not committed) either before the
    subscriber goroutine's first read or between releasing the read lock and
    waiting, `main` fails 5 of 5 runs with the CI error and this pull request
    passes 5 of 5 at the same points.
- `govulncheck` source scans of root, evmd and speedtest and a binary scan of
  the symbol-bearing `xitcoind` built with `make build COSMOS_BUILD_OPTIONS=nostrip`
  report **zero called vulnerabilities** and zero imported-package findings.
  One module-level record remains: GO-2025-3442 (CometBFT). The binary's build
  information shows gjson 1.20.6, `gorilla/websocket`
  v1.5.4-0.20240701034025-d67f41855da4 and the root library at the anchor
  above. The source scans of `main` `f0c9a90` give the same result. The JSON-RPC
  simulator scan is clean; for `tests/systemtests`, see the known open
  findings.
- `scripts/verify-security-assessment.py`, `scripts/run-govulncheck.sh` from
  root and evmd and on the binary, `scripts/test-govulncheck-gate.py` (9 tests),
  `scripts/verify-go-fork-provenance.py` in both modules,
  `scripts/verify-xitcoin-release-state.py` and `scripts/verify-actions-pinned.sh`
  pass. `go mod verify` passes in all five modules. `go list -deps` shows no
  `x/crisis`, `golang.org/x/crypto/openpgp`, `pion/dtls/v2` or `pion/stun/v2`
  package in the production graphs.
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
| GO-2026-5932 | Production armor still uses ProtonMail code; the carved-out `golang.org/x/crypto/openpgp` module is used only by the armor compatibility test. |

## Known open findings

One, outside the gated graphs:

- **`tests/systemtests` (test-only module).** govulncheck reports GO-2026-5932
  as called there: the module requires upstream Cosmos SDK v0.54.5 instead of
  the fork, and the upstream `crypto` package still imports
  `golang.org/x/crypto/openpgp/armor` (traces from package initialization and
  an error type's `Error` method). The result is the same on `main`
  `f0c9a90`, and before 2026-11-27 the same code came from x/crypto 0.58.0.
  The module drives built binaries in system tests; it is not part of the
  node binary, not covered by the gate, and earlier reviews report no scan
  of it.
  Replacing the SDK with the fork there, as `tests/speedtest` does, would
  remove the import; that is left for a separate change.

Two notes, which are not findings of this code review:

- The WebSocket requirement is a pseudo-version of an upstream commit. It
  should move to the first upstream tag that contains `d67f41855da4` when one
  is published.
- The testnet binary (`v0.1.0-testnet.5`) predates this pull request. Neither
  change is state-breaking, so deploying them needs no coordinated upgrade
  height; whether and when is an operator decision.

## Decision

The six historical dispositions remain technically valid for these exact
inputs. No scanner exception is accepted. The review window is
**2026-11-28 to 2026-12-05 UTC**, the same seven-day length as the previous
reviews. There is no automatic renewal; after 2026-12-05 the gate fails again
until a new review is written.
