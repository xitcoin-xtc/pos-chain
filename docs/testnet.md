# Xitcoin Testnet

**Xitcoin Testnet** is the public network name. Its active technical Cosmos Chain ID is `xitcoin-testnet-v2-1`. The retired `xitcoin-testnet-1` generation is retained only for historical verification.

## Canonical identity

| Field | Value |
| --- | --- |
| Public name | Xitcoin Testnet |
| Cosmos Chain ID | `xitcoin-testnet-v2-1` |
| EVM Chain ID | `101089` (`0x18ae1`) |
| Native asset | XTC |
| Atomic denomination | `axtc` |
| Decimals | 18 |
| Genesis time | `2026-08-29T09:49:07Z` |
| Genesis supply | 477,000,000 XTC |
| Genesis SHA-256 | `5db34acf6496b2c76a6f516e0eb605caef6762552584ddbed7c8703239f33d72` |

## Deployment state

The canonical four-validator testnet is publicly active. The sentry, four validators, public endpoints, Cosmos explorer, Blockscout and faucet were verified against the active V2 network on 2026-09-02.

## Public services

| Service | Endpoint |
| --- | --- |
| Cosmos RPC | `https://rpc-testnet.xitchain.com` |
| Cosmos REST API | `https://api-testnet.xitchain.com` |
| EVM JSON-RPC | `https://evm-rpc-testnet.xitchain.com` |
| Cosmos explorer | `https://explorer-testnet.xitchain.com` |
| EVM explorer | `https://evm-explorer-testnet.xitchain.com` |
| Faucet | `https://faucet-testnet.xitchain.com` |

The faucet sends 10 testnet XTC per accepted request. Address and IP windows are 24 hours, with at most three accepted requests per IP in that window. It spends from a finite 50,000,000 XTC genesis allocation and does not mint automatically.

## Validator and allocation policy

- Four initial validators: Atlas, Borealis, Meridian and Zenith.
- 5,000,000 XTC self-delegated by each validator through the four genesis transactions.
- 250,000 liquid XTC remaining per validator account after genesis initialization.
- Validator capacity and admission cap: 258.
- Genesis bank allocations: 386,000,000 XTC sovereign reserve; 50,000,000 XTC faucet; 20,000,000 XTC validator-incentives module; and 21,000,000 XTC across the four validator accounts before genesis transactions execute.
- Of the validator accounts' 21,000,000 XTC, 20,000,000 XTC is self-delegated and 1,000,000 XTC remains liquid.
- No bridge route is configured in genesis. The route was initialized afterwards, on 2026-09-03 (height 74270), and is disabled and paused; no transfer has been processed.

## Canonical genesis

The canonical genesis is published at [`networks/xitcoin-testnet-v2-1/genesis.json`](../networks/xitcoin-testnet-v2-1/genesis.json).

Verify it with:

```bash
cd networks/xitcoin-testnet-v2-1
sha256sum -c genesis.sha256
```

See [Testnet Status](testnet-status.md) for the deployment boundary and [Testnet Operations](testnet-operations.md) for verification commands.
