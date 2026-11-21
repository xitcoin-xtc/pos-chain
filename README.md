# Xitcoin

Xitcoin is an EVM-compatible Proof-of-Stake blockchain built with Cosmos SDK, CometBFT and Cosmos EVM.

It is conceived as a common and evolving infrastructure layer through which public-sector participants, sovereign operators, companies, institutions, DeFi applications, communities and developers can interact using compatible transaction and application standards.

The network provides a shared technical language while each participant retains its own identity, authority, legal framework and operational responsibilities. Access to the protocol is based on published rules rather than institutional hierarchy or political alignment.

## Architecture and interoperability

Xitcoin combines:

- CometBFT consensus and Cosmos SDK protocol modules;
- native EVM execution and Ethereum-compatible tooling;
- Cosmos RPC, REST and gRPC interfaces;
- one canonical native-XTC economy across Cosmos and EVM execution;
- on-chain validator admission separated from staking, governance and operations;
- public source, release checksums and verifiable network state.

Cosmos and EVM activity execute on the same sovereign chain and use the same native XTC accounting. A future Cronos bridge is a separate security boundary using auditable lock/mint and burn/unlock accounting. It is not implied to be active by the existence of bridge source code.

## Network

| Property | Testnet | Mainnet |
| --- | --- | --- |
| Public name | Xitcoin Testnet | Xitcoin |
| Status | Canonical public testnet active | Not launched |
| Cosmos Chain ID | `xitcoin-testnet-v2-1` | `xitcoin` |
| EVM Chain ID | `101089` | `101088` |
| Native asset | XTC | XTC |
| Atomic denomination | `axtc` | `axtc` |
| Decimals | 18 | 18 |

XTC is the native asset on both the Cosmos and EVM interfaces. The EVM interface uses the canonical native precompile; no separate wrapped-XTC token is required inside the Xitcoin network.

The public name is **Xitcoin Testnet**. The technical Cosmos chain ID `xitcoin-testnet-v2-1` identifies the active genesis. The retired `xitcoin-testnet-1` files remain available only for historical verification.

## Participation model

The canonical testnet aligns staking and validator-admission capacity at 258 positions:

- 193 reserved Member-State positions;
- 65 general validator positions;
- 4 initially approved core validators;
- 5,000,000 XTC minimum self-delegation for every validator.

The sovereign framework keeps a defined participation pathway available for every reference position. Activation remains voluntary and begins only through the relevant participant's own initiative, an authorized operator and the same published admission, security and operational standards that apply across the network.

A reference or reserved position does not itself transfer assets or activate a validator. It preserves future access to the framework while the network continues to develop its public infrastructure, applications, developer ecosystem and community participation.

The canonical testnet currently approves only Atlas, Borealis, Meridian and Zenith. The remaining capacity does not announce future operators. Each additional validator would require separate authorization.

During the current launch phase, only the canonical on-chain validator-admission authority can approve or revoke validators. Token balances, staking weight, delegations and ordinary governance voting do not grant or override that authority.

## Project repositories

| Repository | Responsibility |
| --- | --- |
| [`pos-chain`](https://github.com/xitcoin-xtc/pos-chain) | Consensus, Cosmos and EVM execution, native XTC, genesis and network documentation |
| [`contracts`](https://github.com/xitcoin-xtc/contracts) | Canonical Cronos V1, V2, migration and bridge-contract sources, deployments and audit scope |
| [`bridge-relayer`](https://github.com/xitcoin-xtc/bridge-relayer) | Public bridge protocol, relayer and operational safety tooling |
| [`testnets`](https://github.com/xitcoin-xtc/testnets) | Canonical public testnet genesis, checksum and network manifest |
| [`guide`](https://github.com/xitcoin-xtc/guide) | Canonical public user, developer and operator guide |
| [`explorer-testnet`](https://github.com/xitcoin-xtc/explorer-testnet) | Canonical Cosmos explorer source, faucet interface and Blockscout configuration |
| [`explorer-evm-testnet`](https://github.com/xitcoin-xtc/explorer-evm-testnet) | Reproducible standalone Blockscout configuration and provenance records |
| [`brand`](https://github.com/xitcoin-xtc/brand) | Approved standalone Xitcoin token artwork and color references |

Contract addresses and audit claims are maintained only in `contracts`. Network identity and native-asset rules are maintained only in `pos-chain`. Other repositories link to those canonical records instead of duplicating them.

## Build

Prerequisites: Go 1.26.7, Node.js 24 and the toolchain documented by the project.

```bash
git clone https://github.com/xitcoin-xtc/pos-chain.git
cd pos-chain
make build
```

Run the core validation suites:

```bash
go test -tags=test ./x/validatoradmission/...
go test -tags=test ./x/bridge/...
(cd evmd && go test -tags=test ./config ./cmd/evmd/cmd)
(cd evmd && go test -tags=test ./tests/integration/precompiles/werc20)
```

Root and `evmd/` are separate Go modules; root `./...` does not test evmd.
PR44 dependency dispositions and limits are recorded in
[SECURITY-ASSESSMENT.md](SECURITY-ASSESSMENT.md).

## Documentation

- [Network identity](XITCOIN-NETWORK-IDENTITY.md)
- [Testnet](docs/testnet.md)
- [Testnet operations](docs/testnet-operations.md)
- [Bridge architecture](docs/bridge-architecture.md)
- [Bridge acceptance tests](docs/bridge-testnet-acceptance.md)
- [Validator admission](docs/validator-admission.md)
- [Development](docs/development.md)

## Contributing

Read [CONTRIBUTING.md](CONTRIBUTING.md) before submitting changes. Security reports follow the process in [SECURITY.md](SECURITY.md).

## License

Licensed under the [Apache License 2.0](LICENSE).
