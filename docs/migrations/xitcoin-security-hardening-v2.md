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
3. Upgrade every validator binary at the agreed height.
4. After the upgrade, check the upgrade block events and confirm that every
   validator reports `min_self_delegation = 5000000000000000000000000` and a
   self-delegation above it.

## Known issue outside this upgrade: relayer route ID

The live route was initialized with `route_id` set to the Cronos `bytes32`
route identifier `0x21121c16…5cd9` (it satisfies the new route-ID rule). The
`bridge-relayer` Xitcoin destination still hard-codes the label
`cronos-testnet-xitcoin-testnet` as the `route_id` it signs and submits
(`src/xitcoin-destination.js`, `docs/PROTOCOL.md`). Until the relayer uses the
`bytes32` route ID, every inbound `MsgSubmitAttestation` is rejected with
`ErrAttestationRouteMismatch`. This needs a relayer change and is not part of
this upgrade.
