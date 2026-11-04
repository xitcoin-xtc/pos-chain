# Solidity regression suites

The generic suites use the audited Hardhat 3 toolchain in `contracts/`.
Their Solidity fixtures and tests live in `contracts/test-suites/`; the manifests
in `suites/` retain the existing suite names and command entry points. The helper
uses each manifest's `testDirectory` for discovery.

| Suite | Cases retained | Solidity compiler |
| --- | ---: | --- |
| basic | 9 | 0.8.18 |
| eip1559 | 1 | 0.8.18 |
| exception | 1 | 0.8.18 |
| opcode | 3 | 0.5.17 |

The Solidity sources are unchanged. Ethers 6 replaces the Truffle test API while
preserving contract deployment, state, event history, revert, opcode and typed
transaction assertions. The modern runner fails on zero tests, skipped cases or
exclusive selection. `test-local` runs its disposable simulated EVM;
`test-ganache` remains a command alias for callers of the old helper. It now uses
Hardhat's simulated EVM, not Ganache. `test-cosmos` still targets the loopback
development node with remote test accounts and the discovered EVM chain ID.

The chain-specific `precompiles` and `revert_cases` suites retain their existing
runner and protected-governance policy. The separate client compatibility suites
under `tests/evm-tools-compatibility/` retain their client generations.

Truffle, Truffle DB and the standalone Apollo server are removed rather than
substituted with incompatible package aliases. Checks reject reintroduction of
the retired packages and exercise the remaining template, HTTP and WebSocket
consumers with synthetic local data. No production account or endpoint is used.

Run the **Dependency security** workflow with `suite_group=legacy` to exercise all
six suites on disposable GitHub runners. The four modern suites use Node 24 and
the locked contracts npm installation; the two unchanged suites use Node 22 and
the locked Yarn workspace. Application contracts and node configuration are not
changed by this tooling migration.
