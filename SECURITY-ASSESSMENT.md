# SECURITY-ASSESSMENT

Assessment date: **2026-09-14 UTC**. Scope: xitcoin-xtc/pos-chain PR44,
starting head `41da8db5ada202b0bb0a27016ad11dac4b2068b5`, with the exact
production sources, dependency locks and fixtures qualified at
`5e24531951873ec8d0cf4403d118c45f47fa641e`. This is the current source of truth
for the six advisory dispositions and their limits. The machine-readable
[assessment](docs/security-assessment.json) binds this document and the four
module locks. [Dependency provenance](docs/go-fork-provenance.md) records
sources, sums, licenses and historical qualification.

## Decision and authority

The six CI dispositions below are technically accepted for these exact inputs
from **2026-09-14 through 2026-09-21 UTC**, inclusive. Seven days is a deliberately
short re-examination window: advisory metadata still disagrees with source,
DTLS v2 remains transitively required, and parser/platform coverage is bounded.
It is not an upstream promised fix date. There is no automatic renewal.
New findings, lock drift, obsolete imports, an altered assessment or expiry
must fail the gate. This renews the bounded CI disposition, not global security
acceptance, a GitHub approval, release acceptance or deployment permission.

Thomas authorized technical review, targeted corrections and signed publication
in this mission, including a justified actualization. INO performed this
assessment using the PR author's account, Ino-C1. No independent reviewer,
signature of a reviewer, or independent approval is asserted.

The previous mission's task required preserving 2026-09-05 while preparing a
decision pack. Its reports extended this into a requirement for an independent
reviewer designated by xitcoin-xtc. The earlier review task prohibited treating
the same author as independent; it did not itself require such a reviewer for
this technical assessment. The current task expressly permits a justified
technical update and requires finding the normative source of independence.
The repeated report sentence is not an additional authorization requirement.

GitHub main protection was read on 2026-09-14: enforce_admins, linear history
and conversation resolution are enabled; force pushes and deletion are
forbidden. No required_pull_request_reviews or required_status_checks are
configured, branch rules/rulesets are empty, and required_signatures is false
at the server. AGENTS.md nevertheless requires signed conventional commits;
that rule is preserved. `.github/CODEOWNERS` contains `** @cosmos/stack-team`;
this ownership declaration is preserved, but GitHub does not currently require
code-owner approval. Team eligibility or an actual review is not asserted. CONTRIBUTING.md requires explicit review before release of
network-sensitive changes. Other guides require independent review before
production release, bridge acceptance or validator-incentive activation;
those distinct requirements remain intact. None authorizes merging or
releasing PR44 here. Zero GitHub reviews existed at the initial live check.

## Exact inputs and acquired evidence

- SDK required `github.com/cosmos/cosmos-sdk v0.54.4`, replaced by
  `github.com/xitcoin-xtc/cosmos-sdk v0.54.5-0.20260914091530-52ff14a25bee`,
  commit `52ff14a25bee524f0e48ae7f00664443bcb22335`, upstream base
  `dedeb7c80a91c47ae83f5352e29c3dd34e4a3fc6`.
- Geth required `github.com/ethereum/go-ethereum v1.16.9`, replaced by
  `github.com/xitcoin-xtc/go-ethereum v1.17.2-cosmos-0.0.20260913233706-e09f79643cd4`,
  commit `e09f79643cd464404876f3565ce0a8fd8c0aeb16`, Cosmos Geth base
  `d99d6fa2c8d98b7cd653de4a9386d2da3db8f25c`.
- evmd consumes root `v0.1.0-testnet.4.0.20261007125404-5eae6f93a37f`,
  commit `5eae6f93a37facc99ebe4f3048c654390472a8b3`; it is distinct from
  the PR head. Root application sources/locks match that immutable anchor.
- CometBFT `v0.39.4`, ProtonMail/go-crypto `v1.4.1`, STUN `v3.1.5`,
  DTLS `v3.1.4`; residual DTLS v2 `v2.2.12`, x/crypto `v0.56.0`.

The six Go records were fetched directly from vuln.go.dev on the assessment
date and compared with the acquired records; their modification dates remain
unchanged. Maintainer advisories and exact source excerpts were reread; current
GitHub global advisory records were also checked. Global GitHub and Go ranges
are not interchangeable with maintainer prose. No fork-name scanner silence
is used as evidence of remediation.

Acquired SDK continuation [34794203018](https://github.com/xitcoin-xtc/pos-chain/actions/runs/34794203018)
passed full unit tests and corrected crypto lint. The 135-case differential
armor oracle passed against the published SDK. Geth root build/tests passed in
[34788224704](https://github.com/xitcoin-xtc/pos-chain/actions/runs/34788224704);
keeper/tidy, lint, generation and baddeps continuation
[34789811082](https://github.com/xitcoin-xtc/pos-chain/actions/runs/34789811082)
passed. Initial failures remain historical failures.

Application baseline [34828836438](https://github.com/xitcoin-xtc/pos-chain/actions/runs/34828836438)
passed root tests/build and evmd build/other packages, but failed Ledger/ERC20
fixtures. [34832506821](https://github.com/xitcoin-xtc/pos-chain/actions/runs/34832506821)
at `5e245319…` passed the corrected suites, graphs, sums and scans. Its
symbol-bearing binary SHA256 is
`ec80b322aaba119d711b80e87bb15b79550a1c9abb40bae74ea3eec0d46754f6`.
The four immutable qualification artifacts and exact digests remain pinned in
[reuse policy](docs/versioned-go-reuse.json). Reuse does not create a new
binary or relabel old scans. The new gate/guard receives targeted mutation
tests; no acquired SDK/Geth/application campaign is manually repeated.

## Six dispositions

### GO-2023-1821 — non-applicable to examined wiring

[Maintainer advisory](https://github.com/cosmos/cosmos-sdk/security/advisories/GHSA-qfc5-6r3j-jj22)
and [Go record](https://vuln.go.dev/ID/GO-2023-1821.json).
A failing MsgVerifyInvariant transaction cannot halt the chain because the
SDK recovers its panic. Periodic EndBlock invariant checks are a different
path. No patch is announced. The SDK repository contains deprecated crisis
code, but neither acquired production graph nor evmd/app.go module/service
registration includes x/crisis or MsgVerifyInvariant. This demonstrates
non-applicability to this wiring, not an SDK-wide fix or a guarantee that the
chain halts for every invariant. Reintroduction invalidates the disposition.

### GO-2023-1881 — non-applicable to examined wiring

[Maintainer advisory](https://github.com/cosmos/cosmos-sdk/security/advisories/GHSA-w5w5-2882-47pc)
and [Go record](https://vuln.go.dev/ID/GO-2023-1881.json).
x/crisis fails to charge ConstantFee for MsgVerifyInvariant, making expensive
checks cheaper to spam. Ordinary transaction fees remain due and are not the
fix. The same verified absence of production imports/registration prevents
this specific transaction path. This separate advisory remains recorded;
no general CPU/DoS resistance or blanket SDK exemption follows.

### GO-2024-2584 — source fix present

[Maintainer advisory](https://github.com/cosmos/cosmos-sdk/security/advisories/GHSA-86h5-xcpx-cfqc)
and [Go record](https://vuln.go.dev/ID/GO-2024-2584.json).
Redelegating then undelegating stake before its source validator is slashed
could evade punishment. Staking/slashing are active. SDK x/staking/keeper/slash.go
SlashRedelegation handles destination unbonding entries, creation height,
maturity/hold, remaining balance, persistence and burning. The source contains
[fix d1b5b0c5](https://github.com/cosmos/cosmos-sdk/commit/d1b5b0c5ae2c51206cc1849e09e4d59986742cc3)
and the upstream TestSlashRedelegation remains. Maintainer fixed versions are
0.47.10 and 0.50.5; Go metadata still has an open range from 0.50.0. Its
[7dbed2fc reference](https://github.com/cosmos/cosmos-sdk/commit/7dbed2fc0c3ed7c285645e21cb1037d8810372ae)
fixes a separate vesting BlockedAddr issue, also present. This is a source-based
disposition, not a numerical-version shortcut or proof of all staking behavior.

### GO-2025-3442 — source fix present

[Maintainer advisory](https://github.com/cometbft/cometbft/security/advisories/GHSA-22qq-3xwm-r5x4)
and [Go record](https://vuln.go.dev/ID/GO-2025-3442.json).
A blocksync peer announces a high then lower height, leaving an unreachable
maximum target. server/start.go calls node.NewNode and the blocksync reactor;
this path is active. In CometBFT v0.39.4 blocksync/pool.go, SetPeerRange removes
and bans a regressing peer before overwriting its height; removePeer recalculates
the maximum. This matches fixes
[2cebfde0](https://github.com/cometbft/cometbft/commit/2cebfde06ae5073c0b296a9d2ca6ab4b95397ea5)
and [0ee80cd6](https://github.com/cometbft/cometbft/commit/0ee80cd609c7ae9fe856bdd1c6d38553fdae90ce).
The broad Go range ending at 1.0.1 still includes v0.39.4 and yields two module
findings. Current global GitHub ranges distinguish the 1.0 prerelease line;
the metadata disagreement is retained. No absence of a symbol trace proves
unreachability. This fix covers the described height regression, not all
malicious-peer or consensus attacks.

### GO-2026-4479 — obsolete production path removed; v3 fixed

[Maintainer advisory](https://github.com/pion/dtls/security/advisories/GHSA-9f3f-wv7r-qc8r)
and [Go record](https://vuln.go.dev/ID/GO-2026-4479.json).
Random AES-GCM nonce reuse within a DTLS session can permit authentication-key
recovery and forgery. No v2 fix is indicated. Geth p2p/nat/stun.go now uses
STUN v3.1.5 with Dial("udp4"), which calls net.Dial, not DialURI/DTLS. DTLS
v3.1.4 is still imported; its explicit nonce combines 16-bit epoch and 48-bit
sequence as in [61762dee](https://github.com/pion/dtls/commit/61762dee8217991882c5eb79856b9e7a73ee349f).
Fixed v3 versions are 3.0.11/3.1.1. CometBFT/IBC still require DTLS v2.2.12
transitively, but neither acquired production graph imports v2. Two acquired
STUN loopback cases cover response/address handling only. Public NAT, IPv6,
loss/delays, other tags/platforms and TLS/DTLS modes are not qualified. Effective
v2 reintroduction must fail; no claim that all DTLS dependencies disappeared.

### GO-2026-5932 — obsolete armor path replaced

[Go advisory](https://pkg.go.dev/vuln/GO-2026-5932) and
[raw record](https://vuln.go.dev/ID/GO-2026-5932.json).
x/crypto/openpgp is unmaintained and unsafe by design, with no fixed x/crypto
version. SDK crypto/armor.go uses maintained ProtonMail armor/errors v1.4.1.
Legacy OpenPGP appears only in the differential test oracle, not production
imports. x/crypto remains for Argon2/ChaCha20 and other primitives, so its
module finding remains visible. The SDK restores CRC24 corruption checking
through the maintained encoder; CRC24 is not authentication. Existing keyring
encryption algorithms and canonical formats are retained. The 135 cases are
15 sizes and nine variants, not exhaustive fuzzing. Whitespace/header parsing,
END labels and large inputs can differ or remain permissive; allocation is
proportional to input. CLI/keyring is the examined path and raw-key JSON-RPC
import remains prohibited. No new remote exploit was demonstrated, but no
claim of full parser equivalence or safety for arbitrary unbounded inputs.

## Checks, history and remaining scope

At the starting head, 12 checks passed, two failed due to the expired
2026-09-05 review, and tagged publication was skipped. Those outcomes remain
historical. The new signed head must have its own completed check record;
this document does not predeclare CI success. Test and build runs a root
source scan, root tests, build and a release-binary scan; Dependency security
scans root and evmd. Versioned graph validates reuse and targeted guards,
not a new full application campaign. A stripped/devel release binary scan
cannot replace the acquired symbol-bearing versioned binary analysis.

The old top-level review_expires/security_acceptance in
[provenance JSON](docs/go-fork-provenance.json) and old qualification outputs
are preserved historical assessment fields, not today's expiry. Dependency
identities in that manifest remain current and immutable. Current CI disposition
is in docs/security-assessment.json; global security_acceptance remains false.
Dated security-review-20260913.md, security-remediation-20260913, archived reports,
backups and previous failure logs are historical evidence and are not rewritten.

Before any later merge: recheck the exact head, its checks and current
protections; resolve failures, review threads and any approvals then actually
required. Reassess these dispositions if expired or inputs change. Production,
bridge and validator-incentive release requirements remain separate. Public
activation 11.2.8, PR21 and the frozen Cyberscope scope are preserved. No merge,
deployment, transaction, real blockchain key or public restart is authorized
by this assessment. PR44 does not close all Xitcoin work.

## 2026-10-07 organisation rename

The GitHub organisation was renamed from `xitcoin-org` to `xitcoin-xtc`. Module paths, fork replacement paths, links and the recorded checksums were rebound to the new name. The fork commits are unchanged and their trees are byte-identical under both names; the root library pin moved to the rename commit. No logic changed and no renewed security review was performed for this rebinding.
