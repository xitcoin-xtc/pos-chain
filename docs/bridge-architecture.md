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

- Inbound: the replay key is the source deposit, `sha256("xitcoin-bridge-source-deposit-v1" || 0x00 || source_chain_id || 0x00 || source_ref)` (`Attestation.ID`). It excludes the deadline and every other signed field, so any number of approvals for one deposit (for example a re-issued approval with a later deadline) can mint at most once. Keys are stored under prefix `0x01`, exported in genesis as `processed_source_deposits` and queried with `attestation-status`.
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

Genesis XTC is backed by the Cronos bootstrap reserve that `CronosBootstrapEscrow` moves into the vault on activation. The mainnet genesis must therefore set `bridge.outstanding_amount` exactly to the bootstrap amount; with `0`, genesis holders could never bridge out because every outbound burn is capped by `outstanding_amount`. `scripts/verify-xitcoin-mainnet-genesis.py` enforces this, together with a paused, disabled route whose `source_chain_id` is `25`, `max_outstanding_amount >= bootstrap`, no pre-processed deposits and zero nonces.

The escrow's rule that the network stays private until activation is an operational procedure. Neither the chain nor any contract enforces it. The escrow has a permissionless refund after a long timeout in the version prepared for future deployment (see the `contracts` repository).

## Reconciliation and pausing

`bridge-relayer` provides a reconciler that compares the vault balance with `outstanding_amount` plus in-flight transfers. A deficit raises a critical alert and, after consecutive deficits, a pause request: logged only in the default dry-run mode, or handed to an injected hook in `request_pause` mode. Nothing pauses the route automatically. A pause always requires a guardian-signed `MsgEmergencyPauseRoute` on Xitcoin and the guardian's `pause()` on the vault.

## Queries

- `xitcoind query bridge attestation-status <ATTESTATION_ID>` (gRPC `AttestationStatus`): whether a source deposit has been minted. The ID is the replay key above.
- `xitcoind query bridge source-deposit-key <SOURCE_CHAIN_ID> <DEPOSIT_ID>`: computes that key offline.
- `xitcoind query bridge route`: route configuration, `paused`, `outstanding_amount`, control and outbound nonces, read from the module store through an ABCI store query at one height.

Typed gRPC queries for the route configuration, outstanding amount and paused state are not yet available; adding them requires a change to `proto/cosmos/evm/bridge/v1/query.proto` and regenerated code.

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
