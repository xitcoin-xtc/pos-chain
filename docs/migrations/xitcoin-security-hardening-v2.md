# Xitcoin security hardening v2

Upgrade name: `xitcoin-security-hardening-v2`

This coordinated binary upgrade changes which `x/bridge` transactions are
valid, so every validator must switch binaries at the same height. Its handler
also raises the `min_self_delegation` of existing validators to the
`x/validatoradmission` minimum. It adds or removes no store and the bridge
consensus version is unchanged.

## Bridge validity changes

Before this release `x/bridge` parsed amounts two ways.
`Attestation.Validate`, `MsgInitiateOutboundTransfer.ValidateBasic`,
route-configuration validation and the limit checks used
`big.Int.SetString(s, 10)`, which reads `"010"` and `"+10"` as 10. Minting,
burning, the outstanding cap and genesis import used
`sdkmath.NewIntFromString`, which is `SetString(s, 0)` and reads `"010"` as
octal 8 (and rejects `"08"`). An outbound transfer of `"010"` therefore
counted 10 against the daily limit, burned 8, and emitted
`bridge_outbound_burned` with `amount="010"`, which the relayer refuses with
an error for the whole block, stalling every later release. The route limits
were parsed with their errors ignored, so a limit that only one parser
accepted could leave a nil integer behind.

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
validators: a validator self-bonded with exactly 5,000,000 XTC that is slashed
for downtime (1% on the testnet) is jailed and can unjail only after it has
self-delegated the slashed amount again. Operators should keep a small
self-delegation margin above the minimum.

## Operator checklist

1. Before proposing the upgrade height, run `xitcoind query bridge route` and
   confirm that `max_transfer_amount`, `daily_limit` and
   `max_outstanding_amount` are canonical decimals, that the route ID is ASCII
   and that no signer or guardian address has surrounding whitespace. The
   handler never rewrites a stored route: if it fails the new validation it
   emits `xitcoin_bridge_route_config_invalid` and the route stays unusable
   (no mint, burn or update) until a later upgrade repairs it.
2. Query each validator (`xitcoind query staking validator`) and its operator
   self-delegation. A validator whose self-delegation is below 5,000,000 XTC
   will not be migrated; its operator can top up before the upgrade.
3. Upgrade every validator binary at the agreed height.
4. After the upgrade, check the upgrade block events and confirm that every
   validator reports `min_self_delegation = 5000000000000000000000000`.
