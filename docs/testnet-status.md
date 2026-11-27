# Xitcoin Testnet Status

## Network identity

| Property | Canonical value |
| --- | --- |
| Public name | Xitcoin Testnet |
| Cosmos Chain ID | `xitcoin-testnet-v2-1` |
| EVM Chain ID | `101089` (`0x18ae1`) |
| Native asset | XTC |
| Atomic denomination | `axtc` |
| Decimals | 18 |
| Genesis time | `2026-08-29T09:49:07Z` |
| Genesis SHA-256 | `5db34acf6496b2c76a6f516e0eb605caef6762552584ddbed7c8703239f33d72` |
| Genesis supply | 477,000,000 XTC |
| Initial validators | Atlas, Borealis, Meridian and Zenith |
| Validator capacity | 258 |
| Minimum self-delegation | 5,000,000 XTC |
| Module authority in deployed genesis | `xtc1vza8zsgvrfwmve084ytd8xqdkkm7u9e5csctc2` |

`xitcoin-testnet-v2-1` is the only active public testnet generation. `xitcoin-testnet-1` is retired and retained only as historical evidence.

## Verified deployment state

The four-validator V2 testnet is publicly active. On 2026-09-02, the sentry, validators, public endpoints, explorers, faucet, Cosmos transactions and EVM transactions were verified against the same live chain. The canonical endpoints are on `xitchain.com`; each one below answered on 2026-11-22.

| Component | Status | Endpoint |
| --- | --- | --- |
| Cosmos RPC | Active | `https://rpc-testnet.xitchain.com` |
| Cosmos REST API | Active | `https://api-testnet.xitchain.com` |
| EVM JSON-RPC | Active | `https://evm-rpc-testnet.xitchain.com` |
| Cosmos explorer | Active | `https://explorer-testnet.xitchain.com` |
| EVM explorer | Active; indexed from height 1 | `https://evm-explorer-testnet.xitchain.com` |
| Faucet | Active; 10 XTC per accepted request | `https://faucet-testnet.xitchain.com` |
| Bridge route | Initialized on 2026-09-03 (height 74270); disabled and paused | — |

## Genesis bank allocations

| Allocation | Amount |
| --- | ---: |
| Sovereign reserve | 386,000,000 XTC |
| Faucet | 50,000,000 XTC |
| Validator-incentives module | 20,000,000 XTC |
| Four validator accounts before genesis transactions | 21,000,000 XTC |
| **Total** | **477,000,000 XTC** |

The four genesis transactions self-delegate 20,000,000 XTC from the validator accounts. The remaining validator liquid balance is 1,000,000 XTC total, or 250,000 XTC per validator.

The faucet uses its finite allocation and does not mint automatically. Its deployed limits are a 24-hour address window, a 24-hour IP window and at most three accepted requests per IP during that window.

No bridge route configuration exists in this genesis. The administrative authority initialized the route on 2026-09-03 at height 74270 (`MsgInitializeRouteConfig`). It is disabled (`enabled: false`) and paused. A read of the bridge store on 2026-11-22 found no processed attestation, control action, guardian pause, outstanding amount or outbound transfer. Enabling and resuming the route requires explicit authorization and a separate acceptance process.

## Acceptance record

- exact canonical genesis published with its SHA-256 checksum;
- four validators active, enabled, not catching up, with equal initial voting power;
- public RPC, API, EVM RPC, explorers and faucet reachable;
- Blockscout indexing complete from the first EVM height;
- native Cosmos and EVM transactions confirmed and visible in their respective explorers;
- bridge route absent at acceptance (it was initialized, disabled and paused, on 2026-09-03);
- mainnet not launched and no Cronos transaction performed as part of this verification.

## Software version and upgrade history

| Heights | Application version | Source |
| --- | --- | --- |
| 1 to 1,455,999 | `0.1.0-testnet.4-8-g743e086` | revision `743e086` (pull request #58) |
| from 1,456,000 | `0.1.0-testnet.5` | tag `v0.1.0-testnet.5`, revision `1924dad` |

On 2026-11-27 the network applied the coordinated upgrade `xitcoin-security-hardening-v1` at height 1,456,000 (plan submitted by the administrative multisig in transaction `F438B892711526517B58DBD8FEB3CF594EC935B50A5D38B69DF3AA7B6494C0DD`). The release binary is the `v0.1.0-testnet.5` prerelease of this repository (`xitcoind` SHA-256 `1d2f14287c6690477e6a54e95a883e460c311dd99579c108dd2cabea979b3238`). After the upgrade the four validators sign blocks again, the public endpoints, explorers and faucet answer, and the bridge route is still disabled and paused.

A node that synchronizes from genesis runs `0.1.0-testnet.4-8-g743e086` until it halts at height 1,456,000, then continues with `0.1.0-testnet.5`. A production release still requires its own reproducible build, published checksums and independent acceptance record.

No production key, mnemonic, password, node key, validator private key or private backup is stored in this repository. Testnet and mainnet must use independent keys, genesis files, state and operational directories.

Testnet XTC has no monetary value.
