# Xitcoin bridge v2 (state-breaking)

Upgrade name: to be assigned by the release owner. The upgrade requires a
coordinated binary upgrade of every validator, the relayer and all three bridge
signers at the same time. `evmd/go.mod` pins the root module, so the chain
binary only contains this version after that pin is updated.

## What changes

- Inbound replay protection is keyed on the source deposit
  (`source_chain_id`, Cronos `depositId`) instead of a hash of every attestation
  field. Two approvals for one deposit can no longer both mint.
- Attestation, guardian-pause and control signatures use network-specific
  domains `xitcoin-bridge-*-v2/<chain id>`; the attestation payload binds the
  Xitcoin chain ID. Signatures made for v1 (`xitcoin-bridge-testnet-*-v1`) are
  rejected.
- Inbound attestations must carry the route's `source_chain_id`, a canonical
  decimal EVM chain ID. The route configuration gains `source_chain_id`; a
  stored configuration without it uses `25` on chain ID `xitcoin` and `338`
  elsewhere, and `MsgInitializeRouteConfig` stores the derived value.
- Control actions (`update_route_config`, `resume_route`) require the next
  sequential control nonce (store key `0x09`). v1 processed-control records
  (prefix `0x04`) are no longer read. The unhandled `close_route` and
  `pause_route` control action names are removed.
- Outbound burns to the zero or `0x…dEaD` address are rejected before burning;
  inbound mints to blocked module accounts are rejected.
- Genesis gains `control_nonce` and `processed_source_deposits`.

## Store migration

None. The module consensus version is unchanged. No v1 attestation, control
action or transfer was ever processed: the testnet route is disabled and paused
and no vault exists on Cronos mainnet. If a route was initialized under v1 its
stored configuration remains valid. Any signature prepared under v1 must be
discarded and produced again with the v2 domains.

## Operator checklist

1. Upgrade the relayer and every signer to the matching `bridge-relayer`
   release and configure each signer's `journalPath` and destination Xitcoin
   chain ID.
2. Upgrade the chain binary.
3. Confirm with `xitcoind query bridge route` that the route is still disabled
   and paused and that `source_chain_id` is the intended Cronos network.
