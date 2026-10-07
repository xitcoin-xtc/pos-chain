# Faucet uncertain-result recovery design

Status: implemented candidate; offline validation only, no deployment or transfer.
Related acceptance coordination: https://github.com/xitcoin-xtc/pos-chain/issues/33.
G03 implementation is present alongside this design. Operational acceptance remains open.

## Observed boundary

The historical and still-installed `server.js` journals a claim only after `runTx` returns a hash.
A process crash after submission and before `stateWrite`, an invalid CLI response,
or an indeterminate child exit can therefore leave an unrecorded submission.
The in-memory promise queue serializes only the current process. The address
regular expression checks shape but not the Bech32 checksum. Fetch and child
execution have no explicit deadlines in this source.

On 14 September 2026 the installed source SHA256 was reconfirmed as
`55cf31ce2741e61ed2a2f5ce9e7ef65e2dff140a97d67d26a5d48fd93deb9251`,
while the main source inspected had SHA256
`eeca1fa46cb097872035a41486ff2698e2cf1bf6c543f1e4993ff34ac9b27b64`.
They differ: repository findings must not be represented as a complete review
of the deployed implementation. The installed unit was active, bound to
loopback; allowlisted settings reported 86400-second address/IP windows and
three requests per IP. These values do not establish proxy trust or rotation
behavior. Reconcile installed source and proxy configuration before release.

## Proposed implementation contract

Use a durable journal with a schema version, integrity checksum and unique
request ID. Persist recipient, chain ID, denomination, amount, creation time,
quota reservation and state **before** any submission. Never store a signing
key or mnemonic. Preserve the legacy claims file and migrate it on an isolated
copy; refuse malformed/checksum-invalid input without discarding history.

Use explicit states `reserved`, `prepared`, `submitted`, `confirmed`,
`failed_definite` and `unknown`. Persist transaction identity before broadcast
where the submission interface supports preparation. If the current CLI cannot
provide that boundary, record `unknown` on any ambiguous failure and require
reconciliation; never generate another transfer to make the request succeed.

A request in `unknown` keeps its quota reservation and blocks automated retry.
On restart, recover pending requests from the durable journal, not memory.
Lookup a known transaction hash using a bounded read-only query; validate chain,
sender, recipient, denomination, amount, inclusion and final execution code.
Missing from a query or mempool is not proof that a transaction was never sent.
If no hash is available, reconcile the sender's account sequence and relevant
transaction history with an independently checked result. If ambiguous, retain
`unknown` for authorized operator review. Do not infer success from exit code
or a returned broadcast hash alone.

Use a durable single-writer lock or transactional store across processes.
An atomic rename alone does not demonstrate crash durability: fsync the new
file, rename it and fsync the containing directory, or use a transactional
journal with equivalent guarantees. Keep recovery copies. Bound journal growth
while retaining pending entries and the full quota window. Deletion of old
pending state is not cleanup.

Validate Bech32 checksum, the `xtc` prefix and expected decoded account length.
Enforce body, queue, child-output and concurrency limits. Bound balance and
reconciliation reads. A child deadline after possible submission transitions
to `unknown`; killing a child does not undo a broadcast. Normalize the client IP
only after validating the connection comes from the configured trusted proxy;
ignore spoofed forwarding headers from untrusted peers. Preserve 24-hour address
and three-per-IP limits across restart and rotation.

## Offline acceptance before any operational change

Use deterministic fake balance, prepare, broadcast, receipt and clock adapters;
no real RPC, faucet POST, signer, CLI binary or public service is required.

| Case | Required result |
|---|---|
| Crash before durable reservation | No broadcast; retry may reserve once |
| Crash after reservation, before submission | Recover reservation; no duplicate |
| Accepted submission then crash before response | Unknown; no automatic resend |
| Timeout, invalid JSON, empty hash, truncated output | Unknown if submission possible |
| Known included transaction with nonzero execution code | Definite failure recorded; explicit quota policy |
| Hash absent from read-only lookup | Keep unknown; absence is not non-submission |
| Concurrent processes or restart during write | One journal owner; no duplicate request |
| Corrupt/truncated journal or checksum mismatch | Fail closed and preserve evidence |
| Invalid checksum, prefix, length or oversized request | Reject before reservation or signer boundary |
| Address/IP window across restart and log rotation | Limits remain effective |
| Untrusted forwarding header | Does not override peer identity |
| Pending request older than retention window | Retained for reconciliation |

The candidate implements durable reservation, bounded I/O and injectable receipt
validation. Run `node --test ops/testnet-faucet/recovery.test.js` and required CI.
The production receipt adapter, no-hash operator reconciliation and snapshot
archival remain operational prerequisites; obtain independent acceptance before
any deployment.
No live transaction, signing or operational installation is authorized by this
document. This candidate changes the transaction path only in source control.
