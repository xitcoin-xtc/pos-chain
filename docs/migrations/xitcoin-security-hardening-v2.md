# Xitcoin security hardening v2

Upgrade name: `xitcoin-security-hardening-v2`

This coordinated binary upgrade changes which `x/bridge` transactions are
valid and what they write, which Cosmos transaction signatures are valid (see
[Transaction signature encoding](#transaction-signature-encoding)) and the
result codes of failed bridge transactions (see
[Bridge error codes](#bridge-error-codes)), so every validator must switch
binaries at the same height. Its handler also raises the `min_self_delegation`
of existing validators to the `x/validatoradmission` minimum, removes
validator approvals stored under a non-canonical operator spelling and stores
the bridge signer set version. It adds three keys to the existing bridge store
(see [Bridge processing records](#bridge-processing-records)), adds or removes
no store, and the bridge consensus version is unchanged.

## Bridge validity changes

Before this release `x/bridge` parsed amounts two ways.
`Attestation.Validate`, `MsgInitiateOutboundTransfer.ValidateBasic`,
route-configuration validation and the limit checks used
`big.Int.SetString(s, 10)`, which reads `"010"` and `"+10"` as 10. Minting,
burning, the outstanding cap and genesis import used
`sdkmath.NewIntFromString`, which is `SetString(s, 0)` and reads `"010"` as
octal 8 (and rejects `"08"`). An outbound transfer of `"010"` therefore
counted 10 against the daily limit, burned 8, and emitted
`bridge_outbound_burned` with `amount="010"`. Since `bridge-relayer` commit
`5c47605` the relayer skips such an event with an
`xitcoin_outbound_event_skipped` alert instead of stopping on the block, so
the loss was silent: the holder's 8 XTC were burned and never released. (A
raw destination that ethers cannot parse, such as a bad-checksum mixed-case
address, a leading space or a `0X` prefix, was skipped the same way.) The
route limits were parsed with their errors ignored, so a limit that only one
parser accepted could leave a nil integer behind.

From this upgrade:

- One parser (`types.ParseAmount`) reads every amount and route limit. It
  accepts only the canonical decimal form of an integer in `[1, 2^256-1]`:
  digits only, no sign, no leading zero, no whitespace, no prefix or separator.
  A genesis `outstanding_amount` may also be `0`.
- `bridge_outbound_burned` carries the burned amount (`amount.String()`) and
  the destination in its EIP-55 form, never the raw request strings.
- Route IDs must be 1 to 96 ASCII bytes from `[a-z0-9_-]` (non-ASCII letters
  and digits were accepted before).
- Attestation destinations must be printable ASCII without spaces. Source
  references must be exactly 32 hexadecimal bytes with an optional `0x` or
  `0X` prefix and any letter case, without whitespace. Before, both were
  trimmed, so whitespace variants of one attestation shared a signed digest.
- Route-configuration signer and guardian addresses must be spelled exactly.
  Before, `" 0x…"` passed validation and then resolved to the zero address (or
  a nibble-shifted one), silently disabling that signer or the guardian.
- `update_route_config` cannot set `max_outstanding_amount` below the current
  `outstanding_amount` (it would break `outstanding <= cap` and make the
  exported genesis invalid). A cap equal to the outstanding amount already
  stops every new mint.
- Mint arithmetic cannot overflow: a sum beyond 256 bits is a limit breach.

The relayer already produces exactly these forms, so no relayer or signer
change is required.

## Bridge processing records

The typed bridge queries (`/xitcoin/bridge/v1/...`, see
[bridge-architecture.md](../bridge-architecture.md#queries)) need state that
earlier binaries did not keep. From the upgrade height:

| Store key | Value | Written by |
| --- | --- | --- |
| `0x0a` followed by the attestation ID | protobuf `AttestationRecord`: every approved field, destination chain ID, payload hash, signing digest, recovered approvers, signer set version, block height, transaction hash | the transaction that processes the attestation (`MsgSubmitAttestation`), with the replay key |
| `0x0b` followed by the request ID | protobuf `OutboundTransferRecord`: route, sender, EIP-55 destination, amount, nonce, block height, transaction hash | the transaction that burns (`MsgInitiateOutboundTransfer`) |
| `0x0c` | signer set version, 8 bytes big-endian | the upgrade handler, `MsgInitializeRouteConfig`, genesis import, and every `update_route_config` that replaces the set of bridge signers (+1) |

Every value is deterministic: the height is the block height, the
transaction hash is SHA-256 of the transaction bytes that every validator
executes, and the approvers are recovered from the submitted signatures. The
writes belong to the transaction and roll back with it.

- The handler stores signer set version `1` if a route is configured and no
  version is stored (`MigrateBridgeSignerSetVersion`); version 1 identifies
  the signer set in force at the upgrade. Changing only the order or the
  spelling of the signer addresses keeps the version. A route with no stored
  version reads as version 1.
- Nothing is backfilled. A source deposit processed before the upgrade (or
  imported from `processed_source_deposits` without a record) is reported as
  `ATTESTATION_STATE_PROCESSED_WITHOUT_RECORD`, never as absent. An outbound
  burn made before the upgrade has no record and its query returns 404, which
  does not mean that it was not burned: verify such a burn from its
  `bridge_outbound_burned` event.
- Genesis gains `signer_set_version`, `daily_usage` (the most recent
  daily-limit bucket, `{"utc_day", "amount"}`), `attestation_records` and
  `outbound_transfers`; all are optional on import. An export no longer
  drops the daily usage: imported on the same UTC day, the bucket keeps
  counting against `daily_limit` (before, an export and import mid-day reset
  it). Validation requires every attestation record to match its replay key
  and digests and to be listed in `processed_source_deposits`, and every
  outbound record to match its request ID and not exceed `outbound_nonce`.
  It also requires `signer_set_version` to be at most 2^64-2 (a rotation
  from that version is rejected, so the version never wraps), every record
  to come from the route's source chain with a signer set version no later
  than the genesis one, and the approvers of a record of the current signer
  set version to be configured bridge signers (a record of an earlier version
  was verified against signers that a rotation may have removed). At
  `InitGenesis` every record must have been processed on the importing chain
  (`destination_chain_id`), so records cannot be carried to a chain with
  another chain ID, and `daily_usage.utc_day` must not be later than the UTC
  day of the genesis time. The daily usage may exceed `daily_limit`: a signed
  update can lower the limit below the usage of the day, and the export keeps
  the exact usage.
- The records cost gas. Measured on the bridge store with the default KV gas
  costs, `MsgSubmitAttestation` uses about 22,000 more gas (a 568-byte
  record: 22,033 gas) and `MsgInitiateOutboundTransfer` about 11,000 more
  (10,970 gas). Set relayer and wallet gas limits with headroom above these,
  for example by simulating each transaction and applying the usual
  adjustment factor rather than reusing a limit measured before the upgrade.
- The existing `AttestationStatus` query keeps its path and two-field
  response, so clients pinned to it are unaffected.

## Precompile execution changes

The release also changes how some EVM calls to the precompiles execute, so
these too take effect for every validator at the upgrade height. They apply
only where the precompile is active: on xitcoin-testnet-v2-1 only the native
WERC20 precompile (`0xEeee...EEeE`) is; `evmd`'s default genesis activates
every static precompile. Details: `docs/precompile-attack-simulations.md`.

- The query (view) methods of every stateful precompile run on a context
  whose writes are reverted. Before, gov `getTallyResult` on a proposal in
  its voting period deleted the votes it counted, and distribution
  `delegationRewards` and `delegationTotalRewards` incremented validator
  periods, even from a STATICCALL. No migration: the extra validator periods
  are valid accounting; deleted votes cannot be restored.
- ICS-02 `updateClient` with an empty client message reverts instead of
  panicking (the panic aborted the transaction).
- The WERC20 precompile rejects value sent with a method that is not payable
  (only `deposit`, receive and the fallback are). Value sent that way before
  the upgrade stays at the WERC20 address.
- A DELEGATECALL to a stateful precompile no longer panics on its missing
  call value: with empty calldata it reverts, and ERC-20 precompile methods
  run read-only.

## Transaction signature encoding

Before this release the `eth_secp256k1` key verified a Cosmos transaction
signature after dropping its 65th byte (`V`) unchecked, and it also accepted
a 64-byte `R || S` signature. The ante handler had no length check. Anyone who
saw a broadcast transaction could re-wrap the same signed content with another
`V` (257 encodings per signature) and broadcast it: a different `TxRaw` and
transaction hash, valid at the same sequence, paid by the signer. On a local
chain every such copy passed `CheckTx` on a node that had not seen the
original, and the copy a proposer included first executed while the original
failed with code 32 (account sequence mismatch). A client that tracks its
transactions by hash (the faucet, the bridge relayer, exchanges, explorers)
saw a transaction fail whose transfer had in fact happened. The same held for
each member signature of a multisig and for EIP-712 signatures.

From this upgrade an `eth_secp256k1` signature, alone or as a multisig member,
must be exactly the recoverable form `R || S || V`, with `S` in the lower half
of the curve order and `V` the recovery ID of the key:

| Signing path | Signed message | Accepted `V` | Produced by |
| --- | --- | --- | --- |
| `SIGN_MODE_DIRECT`, `SIGN_MODE_LEGACY_AMINO_JSON` | Keccak-256 of the sign bytes | `0` or `1` | `xitcoind` keyring (every `tx` command, `tx sign`, `tx multisign`), Cosmos SDK keyring clients, the bridge relayer |
| EIP-712 (current or legacy typed data) | the EIP-712 representation of the sign bytes | `27` or `28` | `eth_signTypedData_v4` (MetaMask), the Ledger Ethereum app (`xitcoind --ledger`) |

Each path accepts the form its signers already produce, and only one `V`
recovers the key, so a signed transaction has exactly one valid encoding: a
copy without `V`, with the other `V`, with `V + 27` on a sign-bytes signature
or `V - 27` on an EIP-712 signature is rejected. A 64-byte signature or a `V`
outside 0, 1, 27 and 28 is rejected by the ante handler with code 4
(`ErrUnauthorized`) and a log naming the problem (`signature 0:
eth_secp256k1 signature must be 65 bytes (R || S || V), got 64`); any other
mismatch fails signature verification, also code 4. `secp256k1` keys keep
the Cosmos SDK rule. Ethereum transactions (`MsgEthereumTx`) and precompile
calls do not use this verification.

Wallet and client impact:

- `xitcoind` (keyring and Ledger), the faucet, the bridge relayer's Xitcoin
  submitter (which already requires this form), the administrative multisig
  members signing with `xitcoind` and MetaMask EIP-712 clients need no
  change. Offline or partially signed multisig transactions made with
  `xitcoind` before the upgrade stay valid.
- Keplr returns a 64-byte signature from `signDirect` and `signAmino`. A
  client that broadcasts such a signature (for example a block explorer's
  send or delegate dialog) must append the recovery ID before the upgrade
  height, or its transactions are rejected after it.
  [transaction-signatures.md](../transaction-signatures.md) gives the code.
- Hermes signs `eth_secp256k1` keys with 64 bytes; an IBC relayer must use
  another client or a `secp256k1` key.
- `SIGN_MODE_LEGACY_AMINO_JSON` transactions still have the Cosmos SDK
  encoding malleability of that sign mode (the body and auth info bytes are
  not signed, see SDK ADR-076). Track them by signer and sequence, not by
  hash alone.

There is no state change and no migration. A transaction signed before the
upgrade in a rejected encoding is rejected when it is broadcast after it.

## Bridge error codes

Before this release every failed bridge transaction reported code `1`,
codespace `undefined`, so a client could only parse the log. From this
upgrade the errors are registered in codespace `bridge` with the stable codes
below. Result codes are part of the consensus results of a block, which is why
this change rides this upgrade. The logs keep the error text (a wrapped error
reads `<detail>: <description>`, for example `duplicate signer approval:
invalid bridge approvals`). An invalid account address reports `sdk`/`7` and a
wrong authority `sdk`/`4`. Code `1` now only means an internal error. Queries
keep their gRPC status codes. [bridge-architecture.md](../bridge-architecture.md#error-codes)
lists the client action for each code.

| Code | Error | Code | Error |
| --- | --- | --- | --- |
| 2 | `ErrEmptyRequest` | 21 | `ErrAttestationSourceChainMismatch` |
| 3 | `ErrInvalidRouteID` | 22 | `ErrAttestationAlreadyProcessed` |
| 4 | `ErrNonCanonicalAmount` | 23 | `ErrAttestationExpired` |
| 5 | `ErrAmountOutOfRange` | 24 | `ErrSettlementUnavailable` |
| 6 | `ErrInvalidAttestation` | 25 | `ErrSettlementDirection` |
| 7 | `ErrInvalidCronosDestination` | 26 | `ErrBlockedDestination` |
| 8 | `ErrInvalidXitcoinDestination` | 27 | `ErrTransferLimitExceeded` |
| 9 | `ErrInvalidChainID` | 28 | `ErrDailyLimitExceeded` |
| 10 | `ErrInvalidRouteConfig` | 29 | `ErrOutstandingLimitExceeded` |
| 11 | `ErrInvalidControlAction` | 30 | `ErrInsufficientOutstanding` |
| 12 | `ErrInvalidGuardianPause` | 31 | `ErrMaximumSupplyExceeded` |
| 13 | `ErrInvalidSignature` | 32 | `ErrCapBelowOutstanding` |
| 14 | `ErrInvalidApprovals` | 33 | `ErrControlActionNotActive` |
| 15 | `ErrRouteDisabled` | 34 | `ErrControlActionProcessed` |
| 16 | `ErrRoutePaused` | 35 | `ErrControlPayloadMismatch` |
| 17 | `ErrRouteAlreadyPaused` | 36 | `ErrControlNonceGap` |
| 18 | `ErrRouteNotPaused` | 37 | `ErrGuardianPauseRouteMismatch` |
| 19 | `ErrRouteAlreadyInitialized` | 38 | `ErrGuardianPauseExpired` |
| 20 | `ErrAttestationRouteMismatch` | 39 | `ErrSignerSetVersionExhausted` |

The bridge relayer treats every non-zero code as a failure and does not parse
logs, so it needs no change; it can now classify failures by code.

## Validator `min_self_delegation` migration

Genesis transactions are exempt from the admission minimum, so the four
`xitcoin-testnet-v2-1` validators kept `min_self_delegation = 1` while
self-bonding 5,000,000 XTC. For every validator whose `min_self_delegation` is
below the on-chain minimum, the handler compares the operator's own delegation,
valued as the staking module values it before jailing an operator, with the
minimum:

- self-delegation at least the minimum: `min_self_delegation` is raised to the
  minimum, with an info log and a `xitcoin_min_self_delegation_raised` event;
- self-delegation below the minimum (for example after a downtime slash): the
  validator is left unchanged, with an error log and a
  `xitcoin_min_self_delegation_below_minimum` event giving its self-delegation.

The second case does not fail the upgrade. Raising the value would put the
validator in breach at once (any self-undelegation jails it, and a jailed
validator below its minimum cannot unjail), and failing the upgrade would halt
the network for one validator. Its operator should self-delegate up to the
minimum and then raise `min_self_delegation` with `MsgEditValidator`, or the
admission authority can revoke it. If the on-chain minimum cannot be used
(unparsable, not positive or not in the bond denomination) no validator is
changed and a `xitcoin_min_self_delegation_migration_skipped` event is
emitted. Running the migration again changes nothing.

After the upgrade the staking module enforces the minimum for these
validators. The four `xitcoin-testnet-v2-1` validators self-delegate exactly
5,000,000 XTC, so without a top-up the handler sets `min_self_delegation`
exactly equal to their self-delegation: any downtime slash (1% of the
validator's stake on the testnet, 50,000 XTC of a 5,000,000 XTC
self-delegation) would jail the validator and leave it unable to unjail until
it self-delegates the slashed amount again. Checklist step 2 is therefore
required. (A double-sign slash tombstones the validator, which no margin
changes.)

## Validator admission changes

Found by the validator admission attack simulations
([validator-admission-attack-simulations.md](../validator-admission-attack-simulations.md)).

- Approvals are stored and looked up under the canonical (lower-case bech32)
  operator address. The bech32 decoder also accepts the upper-case spelling of
  an address, and approvals used to be stored under the spelling given: a
  `MsgRevokeValidator` spelling the operator in upper case jailed its
  validator but deleted nothing, so the lower-case approval stayed and the
  validator could unjail at once, while an approval spelled in upper case
  approved nobody (the staking hook checks the canonical address). From the
  upgrade, an approval or revocation acts on the operator it names, whatever
  the spelling, and `MsgUnjail` or `MsgCreateValidator` spelled in upper case
  is judged as the operator it names. The handler removes every approval
  stored under another spelling (an upper-case or malformed address) with an
  error log and a `xitcoin_validator_approval_removed` event giving the stored
  spelling and its canonical form. Such an entry never matches again from this
  release, so its removal changes no admission decision; the authority
  approves the operator again (any spelling) if it meant to.
- A `MsgCreateValidator` whose `value` has no amount is rejected by the
  admission ante handler. Before, the comparison with the minimum panicked
  for an approved operator: the transaction failed either way, but a block
  proposer could include it and get a panic result code instead of an
  admission error.
- A governance proposal cannot carry a restricted message (validator creation
  or unjail, mint parameters, a grant for them, at any depth of `MsgExec` or
  nested proposals): the ante handler checks the messages of a proposal, and
  an `x/gov` hook checks every new proposal, whatever the path (transaction,
  `MsgExec`, governance precompile). Release binaries keep executable proposals
  disabled, so this changes nothing on the testnet or mainnet today; the hook
  also rejects every proposal while they are disabled.

## Operator checklist

1. Before proposing the upgrade height, run `xitcoind query bridge route` and
   confirm that `max_transfer_amount`, `daily_limit` and
   `max_outstanding_amount` are canonical decimals, that the route ID is ASCII
   and that no signer or guardian address has surrounding whitespace. The
   handler never rewrites a stored route: if it fails the new validation it
   emits `xitcoin_bridge_route_config_invalid` and the route stays unusable
   (no mint, burn or update) until a later upgrade repairs it.
2. Required, before the upgrade height: each validator operator tops up its
   self-delegation with a margin above the 5,000,000 XTC minimum, by
   delegating from the operator account to its own validator, for example
   `xitcoind tx staking delegate <xtcvaloper…> 110000000000000000000000axtc --from <operator key>`.
   The suggested margin is 110,000 XTC: it keeps the self-delegation above
   the minimum after two consecutive 1% downtime slashes
   (5,110,000 × 0.99² ≈ 5,008,000 XTC), and each validator account held
   250,000 liquid XTC after genesis, which leaves about 140,000 XTC for fees
   and a later top-up. Check the current liquid balance with
   `xitcoind query bank balances <operator address>` first; the margin must be
   more than 50,505 XTC to survive even one slash. Then query each validator
   (`xitcoind query staking validator`) and its operator self-delegation and
   confirm that it is above 5,000,000 XTC. A validator whose self-delegation
   is below the minimum at the upgrade height is not migrated.
3. Before the upgrade height, make sure every client that broadcasts
   Cosmos transactions with 64-byte `eth_secp256k1` signatures (the Keplr
   path of the block explorer, any dApp built on CosmJS) appends the recovery
   ID (see [Transaction signature encoding](#transaction-signature-encoding)).
4. Upgrade every validator binary at the agreed height.
5. After the upgrade, check the upgrade block events and confirm that every
   validator reports `min_self_delegation = 5000000000000000000000000` and a
   self-delegation above it.
6. After the upgrade, confirm that the bridge queries answer, for example
   `curl <api>/xitcoin/bridge/v1/routes/0x21121c16b53a726056a6683f00c7eb4da5501ce8a2abc8a4677e06f1e94b5cd9`
   or `xitcoind query bridge route-config 0x21121c16b53a726056a6683f00c7eb4da5501ce8a2abc8a4677e06f1e94b5cd9`,
   and report `signer_set_version` `1`.
7. After the upgrade, look for `xitcoin_validator_approval_removed` events in
   the upgrade block. The admission authority approves again any operator it
   meant to approve (the testnet approvals are expected to be canonical, in
   which case there is none).

## Known issue outside this upgrade: relayer route ID

The live route was initialized with `route_id` set to the Cronos `bytes32`
route identifier `0x21121c16…5cd9` (it satisfies the new route-ID rule). The
`bridge-relayer` Xitcoin destination still hard-codes the label
`cronos-testnet-xitcoin-testnet` as the `route_id` it signs and submits
(`src/xitcoin-destination.js`, `docs/PROTOCOL.md`). Until the relayer uses the
`bytes32` route ID, every inbound `MsgSubmitAttestation` is rejected with
`ErrAttestationRouteMismatch`. This needs a relayer change and is not part of
this upgrade.
