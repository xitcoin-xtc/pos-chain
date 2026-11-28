# Getting started with a new system test

## Overview

The systemtests suite is an end-to-end test suite that runs the evmd process and sends RPC requests from separate Ethereum/Cosmos clients. The systemtests for cosmos/evm use the `cosmossdk.io/systemtests` package by default. For more details, please refer to https://github.com/cosmos/cosmos-sdk/tree/main/tests/systemtests.

## Preparation

Build a new binary from current branch and copy it to the `tests/systemtests/binaries` folder by running system tests.

```shell
make test-system
```

Or via manual steps

```shell
make build
mkdir -p ./tests/systemtests/binaries
cp ./build/xitcoind ./tests/systemtests/binaries/evmd
(cd ./tests/systemtests/Counter && forge build)
```

The binary is copied as `evmd`: the system test framework names each node
home after the binary, and the `testnet init-files` command writes it as
`evmd`. The suite approves the genesis validators in the validator admission
genesis before each chain start.

## Run Individual test

Each scenario now has its own `Test…` wrapper in `main_test.go`, so you can target a specific flow directly. For example, to exercise the mempool ordering suite:

```shell
cd tests/systemtests
go test -failfast -mod=readonly -tags=system_test ./... -run TestMempoolTxsOrdering \
  --verbose --binary evmd --block-time 3s --chain-id local-4221
```

Mempool scenarios:

| Test name | Description |
|-----------|-------------|
| `TestMempoolTxsOrdering` | Ordering of pending transactions across nodes |
| `TestMempoolTxsReplacement` | Replacement behaviour for EVM transactions |
| `TestMempoolTxsReplacementWithCosmosTx` | Replacement when Cosmos transactions are present |
| `TestMempoolMixedTxsReplacementEVMAndCosmos` | Mixed Cosmos/EVM replacement coverage |
| `TestMempoolTxRebroadcasting` | Rebroadcasting and nonce-gap handling |
| `TestMempoolCosmosTxsCompatibility` | Cosmos-only transactions interacting with the mempool |

EIP-712 scenarios:

| Test name | Description |
|-----------|-------------|
| `TestEIP712BankSend` | Single transfer signed via EIP-712 |
| `TestEIP712BankSendWithBalanceCheck` | Transfer plus balance assertions |
| `TestEIP712MultipleBankSends` | Sequential transfers with nonce management |

Account abstraction:

| Test name | Description |
|-----------|-------------|
| `TestAccountAbstractionEIP7702` | Account abstraction (EIP-7702) flow |

> ℹ️ The shared system test suite keeps a single chain alive across multiple tests when the node arguments are identical. Running several tests back-to-back therefore re-uses the same process unless a scenario explicitly changes the node configuration.

## Run all tests

```shell
make test
```

## Updating Node's Configuration

New in systemtests v1.4.0, you can now update the `config.toml` of the nodes. To do so, the system under test should be set up like so:

```go
s := systemtest.Sut
s.ResetChain(t)
s.SetupChain("--config-changes=consensus.timeout_commit=10s")
```
