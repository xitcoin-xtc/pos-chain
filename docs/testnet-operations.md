# Xitcoin Testnet Operations

## Network verification

Check CometBFT identity and synchronization:

```bash
curl -fsS https://rpc-testnet.xitcoin.org/status |
  jq '.result | {
    chain_id: .node_info.network,
    height: .sync_info.latest_block_height,
    catching_up: .sync_info.catching_up
  }'
```

Expected Chain ID: `xitcoin-testnet-v2-1`. The node must report
`catching_up=false`.

Check the EVM Chain ID:

```bash
curl -fsS \
  -H 'content-type: application/json' \
  --data '{"jsonrpc":"2.0","method":"eth_chainId","params":[],"id":1}' \
  https://evm-rpc-testnet.xitcoin.org |
  jq -r '.result'
```

Expected result: `0x18ae1`.

Check Cosmos staking parameters:

```bash
curl -fsS \
  https://api-testnet.xitcoin.org/cosmos/staking/v1beta1/params |
  jq '.params | {max_validators, bond_denom}'
```

Expected values: `max_validators=258`, `bond_denom=axtc`.

## Genesis verification

```bash
git clone --recurse-submodules https://github.com/xitcoin-xtc/pos-chain.git
cd pos-chain/networks/xitcoin-testnet-v2-1
sha256sum -c genesis.sha256
```

Expected SHA-256:

`5db34acf6496b2c76a6f516e0eb605caef6762552584ddbed7c8703239f33d72`

## Blockscout indexing

Compare the chain and explorer heights:

```bash
CHAIN_HEIGHT="$(
  curl -fsS https://rpc-testnet.xitcoin.org/status |
    jq -r '.result.sync_info.latest_block_height'
)"
BLOCKSCOUT_HEIGHT="$(
  curl -fsS https://evm-explorer-testnet.xitcoin.org/api/v2/stats |
    jq -r '.total_blocks'
)"
printf 'chain=%s blockscout=%s lag=%s\n' \
  "$CHAIN_HEIGHT" "$BLOCKSCOUT_HEIGHT" \
  "$((CHAIN_HEIGHT - BLOCKSCOUT_HEIGHT))"
```

A short moving lag is expected because indexing happens after block production.
Investigate a lag that persists, grows across repeated measurements, or makes
the explorer unavailable.

## Faucet policy

- Amount per accepted request: 10 XTC.
- Address cooldown: 24 hours.
- IP window: 24 hours.
- Maximum accepted requests per IP per window: 3.
- Genesis faucet allocation: 50,000,000 XTC.
- Automatic minting: disabled.

If the faucet allocation becomes low, refill it by an explicitly approved
on-chain transfer from an authorized testnet reserve. Do not change genesis on
a running chain and do not enable automatic minting as an operational shortcut.

## Release verification

A release record must identify a reachable source revision, binary checksum,
genesis checksum and compatibility notes. Source changes do not activate a
genesis or network upgrade automatically.

## Operational principles

- Publish release artifacts with reproducible checksums.
- Validate RPC identity before signing or broadcasting a transaction.
- Apply rate limiting and monitoring to public endpoints.
- Test backup, restoration and rollback procedures before upgrades.
- Coordinate genesis resets and consensus upgrades with every validator.
- Keep testnet and mainnet keys, genesis, state, services and backups separate.
