# Xitcoin Bridge Architecture

## Overview

The canonical route connects the Cronos XTC contract with native XTC on Xitcoin:

```text
Cronos XTC <-> Xitcoin native XTC
```

Cronos XTC contract:

```text
0xE45FE733BC8617FA6DAC8437FC44B5FFFA949991
```

The bridge route is disabled until its contracts, signer configuration, limits and monitoring are approved for the target environment. No vault exists on Cronos mainnet and the testnet route is disabled and paused.

Components: the `x/bridge` module on Xitcoin (this repository), `CronosBridgeVault` and `CronosBootstrapEscrow` on Cronos (`contracts` repository), and the watchers, coordinator, signer services and reconciler of `bridge-relayer`. This document describes the chain module; the other repositories document their own components.

## Transfer model

### Cronos to Xitcoin

1. A holder calls `CronosBridgeVault.deposit(amount, recipient)`; the vault locks the XTC and emits `Deposited(depositId, routeId, depositor, recipient, amount, nonce)`.
2. The relayer waits for the configured confirmation depth and builds an approval request for that deposit.
3. Each signer service independently re-checks the finalized deposit and signs the attestation digest (two of three signatures are needed).
4. Anyone submits `MsgSubmitAttestation` with the attestation fields and two signatures.
5. The module checks, in order: route configured, route ID, route enabled and not paused, deadline against block time, source chain ID equals the route's `source_chain_id`, two distinct configured signers over the network-specific digest, source deposit not processed before, per-transfer and daily limits, recipient is not a blocked module account, outstanding limit and XTC maximum supply. It then records the replay key, mints native XTC to the recipient and increases `outstanding_amount`. Any failure reverts the whole transaction.

### Xitcoin to Cronos

1. A holder submits `MsgInitiateOutboundTransfer(route_id, destination, amount)`.
2. The module checks the route is enabled and not paused, the destination is an EVM address other than the zero and `0x…dEaD` addresses (the vault refuses to release to them), per-transfer and daily limits, and `amount <= outstanding_amount`. It then moves the XTC to the module, burns it, decreases `outstanding_amount` and emits `bridge_outbound_burned` with a `request_id`.
3. Signers verify the finalized burn and sign the vault's EIP-712 `Release` for `sourceBurnId = request_id`.
4. Anyone submits the release to the vault, which enforces its own replay protection and limits.

## Signed messages

Every bridge signature on Xitcoin is a raw secp256k1 signature over a 32-byte digest whose domain includes the Xitcoin chain ID of the chain that verifies it (`types/domain.go`). An approval produced for `xitcoin-testnet-v2-1` is therefore invalid on `xitcoin` and vice versa.

| Message | Domain | Binds |
| --- | --- | --- |
| Inbound attestation (`types/signatures.go`) | `xitcoin-bridge-attestation-v2/<chain id>` | Xitcoin (destination) chain ID, route ID, direction, source chain ID, source reference (Cronos `depositId`), deposit nonce, destination, amount, deadline |
| Guardian pause (`types/pause.go`) | `xitcoin-bridge-guardian-pause-v2/<chain id>` | route ID, pause nonce, expiry |
| Control action (`types/control.go`) | `xitcoin-bridge-control-v2/<chain id>` | route ID, action (`update_route_config` or `resume_route`), payload hash (exact new configuration, or configuration plus target state), control nonce, validity window |

Attestation digest: `keccak256(domain || sha256(join(0x00, chain_id, route_id, direction, source_chain_id, source_ref, nonce, destination, amount, deadline_unix)))`. The relayer computes the same value (`bridge-relayer` `src/protocol.js`); both repositories test against shared vectors (`types/remediation_test.go`, relayer `test/fixtures/attestation-cross-check.json`).

The Cronos `depositId` itself commits to the Cronos chain ID, vault, route, depositor, recipient, amount and deposit nonce. The vault's `Release` approvals use EIP-712 bound to the Cronos chain ID and vault address; see the `contracts` repository.

## Replay protection

- Inbound: the replay key is the source deposit, `sha256("xitcoin-bridge-source-deposit-v1" || 0x00 || source_chain_id || 0x00 || source_ref)` (`Attestation.ID`). It excludes the deadline and every other signed field, so any number of approvals for one deposit (for example a re-issued approval with a later deadline) can mint at most once. Keys are stored under prefix `0x01`, exported in genesis as `processed_source_deposits` and queried with `attestation` (or `attestation-status`).
- Outbound: the vault records each `sourceBurnId` once.
- Control actions: the control nonce is strictly sequential (`control_nonce`, store key `0x09`). An action is accepted only with nonce `control_nonce + 1`; `update_route_config` and `resume_route` share the sequence. An older signed action that is still inside its validity window can therefore never be applied after a newer one.
- Guardian pauses: each pause action ID is recorded once and expires; a pause can only set `paused = true`.

## Roles

| Actor | Can | Cannot |
| --- | --- | --- |
| Bridge authority (account set at app wiring) | `MsgInitializeRouteConfig` once: creates the only route, always disabled and paused | Change the route afterwards, enable it, mint or burn |
| Guardian (one address in the route config) | Sign a pause: sets `paused = true` | Resume, change configuration, mint, burn |
| Two of the three bridge signers | Approve inbound attestations; approve `update_route_config` (signers, guardian, limits, `enabled`); approve `resume_route` | Change the route ID or `source_chain_id`; reuse a control nonce |
| Any account | Submit messages that carry the required signatures (the submitter is not part of what is signed) | Anything without those signatures |
| Any holder | `MsgInitiateOutboundTransfer` for its own XTC while the route is enabled and not paused | Burn above the limits or `outstanding_amount` |

There is no close-route action. Disabling the route is an `update_route_config` with `enabled = false`, which blocks new mints and new outbound burns. Releases of burns that already happened are performed by the vault and are not affected by the chain route state.

The signer set has no timelock or separate key for raising limits; options are proposed, not implemented, in the `contracts` repository (`docs/GOVERNANCE_PROPOSAL.md`).

## Limits

On Xitcoin (`RouteConfig`, all amounts in `axtc`):

- `max_transfer_amount`: maximum amount of one inbound mint or one outbound burn;
- `daily_limit`: one bucket per route per UTC day of block time, shared by inbound mints and outbound burns;
- `max_outstanding_amount`: an inbound mint is rejected if `outstanding_amount` would exceed it;
- the XTC maximum supply, enforced on every mint;
- an outbound burn cannot exceed `outstanding_amount`;
- `update_route_config` cannot set `max_outstanding_amount` below the current `outstanding_amount`.

Every amount and limit is read by one parser (`types.ParseAmount`) for validation, limits, minting, burning and genesis. It accepts only the canonical decimal form of an integer in `[1, 2^256-1]` (`^[1-9][0-9]*$`, the form the relayer requires); `"010"`, `"+10"`, `" 10"` and `"0x10"` are rejected. `bridge_outbound_burned` carries the burned amount and the EIP-55 destination, not the raw request strings. Route IDs are ASCII `[a-z0-9_-]`, attestation destinations and source references contain no whitespace, and configured signer and guardian addresses are spelled exactly. See [the security hardening v2 migration](migrations/xitcoin-security-hardening-v2.md).

On Cronos, `CronosBridgeVault` limits releases (per release and per day) and, in the version prepared for future deployment, deposits (per deposit, per day and total locked balance). The vault limits should be configured no higher than the chain limits so that a deposit the chain cannot mint is rejected at deposit time. A deposit that is already locked but cannot mint (above a chain limit, or to a blocked module account) is handled by the operational procedure in `bridge-relayer` `docs/OPERATIONS.md` (wait for the next day, raise limits by quorum, or an owner-approved refund).

## Accounting

```text
outstanding_amount = genesis outstanding_amount + bridge mints - bridge burns
vault balance      = outstanding_amount + deposits not yet minted
                     + burns not yet released + direct transfers to the vault
```

Genesis XTC is backed by the Cronos bootstrap reserve that `CronosBootstrapEscrow` moves into the vault on activation. The mainnet genesis must therefore set `bridge.outstanding_amount` exactly to the bootstrap amount; with `0`, genesis holders could never bridge out because every outbound burn is capped by `outstanding_amount`. `scripts/verify-xitcoin-mainnet-genesis.py` enforces this, together with a paused, disabled route whose `source_chain_id` is `25`, `max_outstanding_amount >= bootstrap`, no pre-processed deposits, transfer records or daily usage, signer set version 1 and zero nonces.

The escrow's rule that the network stays private until activation is an operational procedure. Neither the chain nor any contract enforces it. The escrow has a permissionless refund after a long timeout in the version prepared for future deployment (see the `contracts` repository).

## Reconciliation and pausing

`bridge-relayer` provides a reconciler that compares the vault balance with `outstanding_amount` plus in-flight transfers. A deficit raises a critical alert and, after consecutive deficits, a pause request: logged only in the default dry-run mode, or handed to an injected hook in `request_pause` mode. Nothing pauses the route automatically. A pause always requires a guardian-signed `MsgEmergencyPauseRoute` on Xitcoin and the guardian's `pause()` on the vault.

## Queries

Typed gRPC queries (`proto/cosmos/evm/bridge/v1/query.proto`), each with a gRPC-gateway REST route on the node's API server and an `xitcoind query bridge` command. Responses are protobuf JSON: 64-bit integers are strings and default values are always present (`"processed": false`, `"record": null`). Every response except `params` carries `height`, the committed block height whose state answered (also in the `x-cosmos-block-height` header).

| gRPC | REST (GET) | CLI | Returns |
| --- | --- | --- | --- |
| `Params` | `/xitcoin/bridge/v1/params` | `params` | authority, native denom, maximum supply, 2-of-3 policy, chain ID and the three signing domains |
| `Route` | `/xitcoin/bridge/v1/routes/{route_id}` | `route-config ROUTE_ID` | configuration, `paused`, `signer_set_version`, control and outbound nonces; `route_id` is the stored ID (on `xitcoin-testnet-v2-1` the Cronos bytes32 `0x21121c16…5cd9`), anything else is 404 |
| `Outstanding` | `/xitcoin/bridge/v1/outstanding` | `outstanding` | `outstanding_amount`, `max_outstanding_amount`, `remaining_amount` |
| `DailyUsage` | `/xitcoin/bridge/v1/daily_usage` | `daily-usage` | usage of the daily limit on the UTC day of the queried block (`utc_day`, `utc_date`, `used_amount`, `remaining_amount`) |
| `Attestation` | `/xitcoin/bridge/v1/attestations/{attestation_id}` | `attestation ATTESTATION_ID` | `status` and, when processed, the record described below |
| `SourceDeposit` | `/xitcoin/bridge/v1/source_deposits/{source_chain_id}/{source_ref}` | `source-deposit SOURCE_CHAIN_ID SOURCE_REF` | the `Attestation` answer for the replay key of a Cronos `depositId` |
| `OutboundTransfer` | `/xitcoin/bridge/v1/outbound_transfers/{request_id}` | `outbound-transfer REQUEST_ID` | the burn: sender, EIP-55 destination, amount, nonce, `height`, `tx_hash`; 404 if none is recorded (see below: a 404 does not prove that nothing was burned) |
| `AttestationStatus` | `/cosmos/evm/bridge/v1/attestations/{attestation_id}` | `attestation-status ATTESTATION_ID` | unchanged two-field response (`attestation_id`, `processed`) for clients pinned to it |

IDs are accepted as 32 hexadecimal bytes with or without `0x`, in any case, and returned as `0x`-prefixed lowercase. A malformed ID or chain ID is HTTP 400, an unknown route or request ID 404, an unknown path 501 (`"Not Implemented"`).

`status` is one of:

- `ATTESTATION_STATE_ABSENT`: no committed transaction has consumed the replay key at `height`. It does not prove that no submission is pending; never retry an uncertain send on this alone.
- `ATTESTATION_STATE_PROCESSED`: minted. `record` binds the attestation ID to every approved field (route ID, direction, source chain ID, source reference, nonce, recipient, amount, deadline), the destination chain ID, the SHA-256 `payload_hash` and the `signing_digest` the approvals signed, the recovered `approvers` and their `signer_set_version`, and the `height` and `tx_hash` of the processing transaction.
- `ATTESTATION_STATE_PROCESSED_WITHOUT_RECORD`: the replay key was consumed before records were kept (before `xitcoin-security-hardening-v2`, or imported from `processed_source_deposits` without a record). Treat it as minted.

`OutboundTransfer` answers only for burns recorded since the `xitcoin-security-hardening-v2` upgrade. A burn made before the upgrade height has no record, so for such a request ID a 404 does **not** mean "not burned": verify it from the `bridge_outbound_burned` event of the burning transaction (CometBFT `/tx` or `/block_results` at its height) instead, and never release or refund on a 404 alone.

`tx_hash` is the CometBFT transaction hash (SHA-256 of the transaction bytes, uppercase hex), so CometBFT `/tx?hash=0x<tx_hash>` returns the transaction and its result at `height`. The record is consensus state at bridge store key `0x0a || attestation_id` (protobuf `AttestationRecord`; outbound transfers at `0x0b || request_id`). An independent client can verify it without trusting the API node: request `/store/bridge/key` with `prove=true` through CometBFT `abci_query` at height `H`, decode the value, check the ICS-23 proof against the `app_hash` in the header of block `H+1`, and verify that header with a light client. CometBFT finality is immediate: a record visible in committed state is final.

`GET /xitcoin/bridge/v1/attestations/0x7ea0d462430dd3540723abd5c38f4caa56f63770795a13af82610b91f8b99abb`:

```json
{
  "attestation_id": "0x7ea0d462430dd3540723abd5c38f4caa56f63770795a13af82610b91f8b99abb",
  "status": "ATTESTATION_STATE_PROCESSED",
  "processed": true,
  "record": {
    "attestation_id": "0x7ea0d462430dd3540723abd5c38f4caa56f63770795a13af82610b91f8b99abb",
    "route_id": "0x21121c16b53a726056a6683f00c7eb4da5501ce8a2abc8a4677e06f1e94b5cd9",
    "direction": "cronos_to_xitcoin",
    "source_chain_id": "338",
    "source_ref": "9f3c1d2e4b5a69788796a5b4c3d2e1f00112233445566778899aabbccddeeff0",
    "nonce": "12",
    "recipient": "xtc1yg3zyg3zyg3zyg3zyg3zyg3zyg3zyg3z97g2qj",
    "amount": "250000000000000000000",
    "deadline_unix": "1795869000",
    "signer_set_version": "1",
    "approvers": [
      "0x5B0aD6a4D5b5D3b2E1d6E5c3e0F9a8B7c6D5E4f3",
      "0x7d2cf8c6F7d7f5d4A3F8a7e5A2b1C0D9e8F7a6B5"
    ],
    "destination_chain_id": "xitcoin-testnet-v2-1",
    "payload_hash": "0xe049144b14143969935ece70073c7144a1133388e41cedfacd9173848f9288bb",
    "signing_digest": "0x01b4003f5242bad7eec397cdc90cc3cd1c25d65f0554673975ccb4d0877b390a",
    "height": "1290412",
    "tx_hash": "EF094712C5B31715AA31A3809336262231B809A701664797BA7537A7DD568E8F"
  },
  "height": "1290450"
}
```

`GET /xitcoin/bridge/v1/source_deposits/338/0xeeee…eeee` for a deposit that was not minted:

```json
{
  "attestation_id": "0xc703fc5dd0d02552c19d90342894281784d9db42b0849c04877e8fb6991da5cb",
  "status": "ATTESTATION_STATE_ABSENT",
  "processed": false,
  "record": null,
  "height": "1290450"
}
```

`GET /xitcoin/bridge/v1/routes/0x21121c16b53a726056a6683f00c7eb4da5501ce8a2abc8a4677e06f1e94b5cd9`:

```json
{
  "route": {
    "route_id": "0x21121c16b53a726056a6683f00c7eb4da5501ce8a2abc8a4677e06f1e94b5cd9",
    "source_chain_id": "338",
    "bridge_signers": [
      "0x5B0aD6a4D5b5D3b2E1d6E5c3e0F9a8B7c6D5E4f3",
      "0x6c1Be7b5E6C6e4C3F2E7F6D4F1a0B9C8D7e6f5a4",
      "0x7d2cf8c6F7d7f5d4A3F8a7e5A2b1C0D9e8F7a6B5"
    ],
    "guardian": "0x8e3dA9D7A8E8A6e5b4A9b8f6B3c2D1e0f9A8B7C6",
    "max_transfer_amount": "1000000000000000000000",
    "daily_limit": "10000000000000000000000",
    "max_outstanding_amount": "100000000000000000000000",
    "enabled": true,
    "paused": false,
    "signer_set_version": "1",
    "control_nonce": "3",
    "outbound_nonce": "1"
  },
  "height": "1290450"
}
```

`GET /xitcoin/bridge/v1/outstanding`:

```json
{
  "route_id": "0x21121c16b53a726056a6683f00c7eb4da5501ce8a2abc8a4677e06f1e94b5cd9",
  "outstanding_amount": "150000000000000000000",
  "max_outstanding_amount": "100000000000000000000000",
  "remaining_amount": "99850000000000000000000",
  "height": "1290450"
}
```

`GET /xitcoin/bridge/v1/daily_usage` (the bucket of the next block can be a later UTC day):

```json
{
  "route_id": "0x21121c16b53a726056a6683f00c7eb4da5501ce8a2abc8a4677e06f1e94b5cd9",
  "utc_day": "20785",
  "utc_date": "2026-11-28",
  "used_amount": "350000000000000000000",
  "daily_limit": "10000000000000000000000",
  "remaining_amount": "9650000000000000000000",
  "block_time_unix": "1795868400",
  "height": "1290450"
}
```

`GET /xitcoin/bridge/v1/outbound_transfers/0xe1494d61d7465b2abfb4f3e067ee95256570205831f01c746616bd94cfa5bb33`:

```json
{
  "transfer": {
    "request_id": "0xe1494d61d7465b2abfb4f3e067ee95256570205831f01c746616bd94cfa5bb33",
    "route_id": "0x21121c16b53a726056a6683f00c7eb4da5501ce8a2abc8a4677e06f1e94b5cd9",
    "sender": "xtc1yg3zyg3zyg3zyg3zyg3zyg3zyg3zyg3z97g2qj",
    "destination": "0x3333333333333333333333333333333333333333",
    "amount": "100000000000000000000",
    "nonce": "1",
    "height": "1290433",
    "tx_hash": "B4D32FCAF1C74A971647A319AFEAE0E8B8B1F5F048004CEC742A801A1338A5BE"
  },
  "height": "1290450"
}
```

`GET /xitcoin/bridge/v1/params` on `xitcoin-testnet-v2-1`:

```json
{
  "params": {
    "authority": "xtc1vza8zsgvrfwmve084ytd8xqdkkm7u9e5csctc2",
    "native_denom": "axtc",
    "max_supply": "5250000000000000000000000000",
    "required_approvals": 2,
    "signer_set_size": 3,
    "chain_id": "xitcoin-testnet-v2-1",
    "attestation_domain": "xitcoin-bridge-attestation-v2/xitcoin-testnet-v2-1",
    "control_domain": "xitcoin-bridge-control-v2/xitcoin-testnet-v2-1",
    "guardian_pause_domain": "xitcoin-bridge-guardian-pause-v2/xitcoin-testnet-v2-1",
    "source_deposit_domain": "xitcoin-bridge-source-deposit-v1"
  }
}
```

The example values are illustrative. Other commands: `xitcoind query bridge source-deposit-key <SOURCE_CHAIN_ID> <DEPOSIT_ID>` computes a replay key offline, and `xitcoind query bridge route` reads the raw route keys through an ABCI store query at one height.

## Error codes

From the `xitcoin-security-hardening-v2` upgrade a failed bridge transaction
reports one of these codes in codespace `bridge` (`x/bridge/types/errors.go`).
Before, every bridge failure was code `1`, codespace `undefined`. The codes are
stable: new ones are only added after the last. The log keeps the error text,
for a wrapped error prefixed with the detail (for example
`duplicate signer approval: invalid bridge approvals`). Code `1`, codespace
`undefined`, now only means an internal error (for example an unreadable
store value). An invalid account address is the SDK's
`sdk`/`7` (`ErrInvalidAddress`) and a wrong authority `sdk`/`4`
(`ErrUnauthorized`). Queries keep their gRPC status codes.

| Code | Error | Meaning | Client action |
| --- | --- | --- | --- |
| 2 | `ErrEmptyRequest` | empty bridge message | fix the request |
| 3 | `ErrInvalidRouteID` | route ID is not 1 to 96 bytes of `[a-z0-9_-]` | fix the request |
| 4 | `ErrNonCanonicalAmount` | amount is not a canonical positive decimal | fix the request |
| 5 | `ErrAmountOutOfRange` | amount exceeds 2^256-1 | fix the request |
| 6 | `ErrInvalidAttestation` | invalid direction, source reference, nonce, destination or deadline | fix the request |
| 7 | `ErrInvalidCronosDestination` | outbound destination is not an address the Cronos vault releases to | fix the request |
| 8 | `ErrInvalidXitcoinDestination` | inbound destination is not a Xitcoin account address | fix the request |
| 9 | `ErrInvalidChainID` | chain ID is empty, too long, has a forbidden character or is not a canonical EVM chain ID | fix the request |
| 10 | `ErrInvalidRouteConfig` | route configuration (signers, guardian, limits) is invalid. A stored route that fails validation makes every route transaction fail with the code of the failing field (3, 9 or 10) | fix the request, or the route needs repair |
| 11 | `ErrInvalidControlAction` | control action, payload hash, nonce or validity window is invalid | fix the request |
| 12 | `ErrInvalidGuardianPause` | guardian pause nonce or expiry is invalid | fix the request |
| 13 | `ErrInvalidSignature` | an approval or control signature is malformed (length, recovery ID, values) | re-sign |
| 14 | `ErrInvalidApprovals` | wrong number of approvals, an approval not from a configured signer (or the guardian), or a duplicate approval | collect valid approvals |
| 15 | `ErrRouteDisabled` | no route is configured, or it is disabled | wait for the route |
| 16 | `ErrRoutePaused` | the route is paused | wait for the resume |
| 17 | `ErrRouteAlreadyPaused` | the route is already paused | none |
| 18 | `ErrRouteNotPaused` | resume of a route that is not paused | none |
| 19 | `ErrRouteAlreadyInitialized` | a route is already configured | none |
| 20 | `ErrAttestationRouteMismatch` | route ID differs from the configured route | fix the request |
| 21 | `ErrAttestationSourceChainMismatch` | source chain differs from the route's | fix the request |
| 22 | `ErrAttestationAlreadyProcessed` | the source deposit was already processed | done: query the attestation record |
| 23 | `ErrAttestationExpired` | the attestation deadline has passed | re-attest |
| 24 | `ErrSettlementUnavailable` | the node has no bank keeper wired | report |
| 25 | `ErrSettlementDirection` | the attestation is not an inbound Cronos transfer | fix the request |
| 26 | `ErrBlockedDestination` | the destination is a blocked module account | fix the request |
| 27 | `ErrTransferLimitExceeded` | amount above `max_transfer_amount` | split or wait for a limit change |
| 28 | `ErrDailyLimitExceeded` | amount above what remains of `daily_limit` today (UTC) | retry the next UTC day |
| 29 | `ErrOutstandingLimitExceeded` | mint above `max_outstanding_amount` | wait for burns or a limit change |
| 30 | `ErrInsufficientOutstanding` | burn above the outstanding bridged amount | lower the amount |
| 31 | `ErrMaximumSupplyExceeded` | mint above the XTC maximum supply | report |
| 32 | `ErrCapBelowOutstanding` | update sets `max_outstanding_amount` below the outstanding amount | fix the update |
| 33 | `ErrControlActionNotActive` | control action outside its validity window | re-sign with a valid window |
| 34 | `ErrControlActionProcessed` | control nonce already used, or the pause already applied | none |
| 35 | `ErrControlPayloadMismatch` | control payload does not match the route | fix the request |
| 36 | `ErrControlNonceGap` | control nonce is not the next one | apply the earlier action first |
| 37 | `ErrGuardianPauseRouteMismatch` | guardian pause for another route | fix the request |
| 38 | `ErrGuardianPauseExpired` | guardian pause past its expiry | re-sign |
| 39 | `ErrSignerSetVersionExhausted` | signer set version cannot be increased | report |

## Attack simulations

The properties above are checked by Go native fuzz targets in `x/bridge/types/fuzz_test.go` and `x/bridge/keeper/fuzz_test.go`, described in [bridge-attack-simulations.md](bridge-attack-simulations.md).

## Route configuration fields

`source_chain_id` is the EVM chain ID of the Cronos network feeding the route. It is stored explicitly at initialization: genesis may set it; `MsgInitializeRouteConfig` derives it from the Xitcoin chain ID (`25` for `xitcoin`, `338` otherwise). `update_route_config` preserves it. A configuration stored before this field existed uses the same derivation.

## Upgrade notes (v1 to v2, state-breaking)

This version changes the signed formats and state layout:

- all three signing domains changed from fixed `xitcoin-bridge-testnet-*-v1` strings to network-specific `*-v2/<chain id>` domains, and the attestation payload now includes the Xitcoin chain ID;
- the inbound replay key changed from a hash of every attestation field to the source-deposit key;
- source chain IDs must be canonical decimal EVM chain IDs matching the route;
- control nonces are sequential (store key `0x09`); v1 processed-control records under `0x04` are no longer read;
- the `close_route` and `pause_route` control action names were removed (they had no handler);
- genesis gains `control_nonce` and `processed_source_deposits`; route configuration gains `source_chain_id`.

The relayer, all three signers and the chain binary must be upgraded together (see [the migration note](migrations/xitcoin-bridge-v2.md)). No v1 attestation or control action was ever applied (zero transfers, route disabled and paused), so no state migration is required; signatures prepared for v1 must be discarded.

## Burn semantics

A token burn invokes the token's burn mechanism and reduces `totalSupply`. A transfer to a dead address changes the holder balance but does not, by itself, prove a reduction in `totalSupply`.

Administrative Cronos supply reduction is separate from bridge settlement and must follow the verified contract ownership and governance controls.

## Read-only Cronos verification

```bash
export CRONOS_RPC='https://evm.cronos.org'
export CRONOS_XTC='0xE45FE733BC8617FA6DAC8437FC44B5FFFA949991'

cast code "$CRONOS_XTC" --rpc-url "$CRONOS_RPC"
cast call "$CRONOS_XTC" 'totalSupply()(uint256)' --rpc-url "$CRONOS_RPC"
cast call "$CRONOS_XTC" 'owner()(address)' --rpc-url "$CRONOS_RPC"
```

Contract implementation, ABI and ownership must be verified against the canonical explorer before an administrative action.

## Deployment stages

1. Unit and integration testing.
2. Isolated testnet deployment with test assets.
3. Independent security review.
4. Limited production rollout.
5. Continuous monitoring and reconciliation.

Additional networks require route-specific adapters, confirmation policies and threat models.
