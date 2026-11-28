# Security policy

## Reporting a vulnerability

Report vulnerabilities privately through
[GitHub private vulnerability reporting](https://github.com/xitcoin-xtc/pos-chain/security/advisories/new)
(Security tab, "Report a vulnerability"). Do not open a public issue, pull
request or discussion for an undisclosed vulnerability.

Please include:

- the affected component, and the commit or release you tested;
- the impact, and who can trigger it;
- steps to reproduce, or a proof of concept on a local network.

We will acknowledge the report, keep you informed while we investigate and
fix it, and agree with you on the disclosure date. Please give us time to ship
a fix before disclosing publicly. Never test against the public testnet in a
way that disrupts it or other users.

Fixes are made on `main` and shipped in the next testnet release.

## Scope

In scope, for the code in this repository:

- consensus and state-transition logic, upgrade handlers and migrations;
- the Xitcoin modules: `x/bridge`, `x/validatoradmission`, `x/validatorincentives`;
- EVM execution, precompiles, ante handlers, fee market and mempool;
- JSON-RPC, gRPC and REST behavior implemented here;
- the Cosmos SDK and go-ethereum forks we maintain ([docs/forks.md](docs/forks.md));
- build and release integrity: workflows, release binaries, genesis checksums.

Out of scope:

- bridge contracts and the relayer: report them in
  [`contracts`](https://github.com/xitcoin-xtc/contracts) and
  [`bridge-relayer`](https://github.com/xitcoin-xtc/bridge-relayer);
- third-party services, and upstream projects (Cosmos SDK, CometBFT, IBC-Go,
  Cosmos EVM, go-ethereum): report to their maintainers, and tell us if
  Xitcoin is affected;
- volumetric denial of service, social engineering, and issues that require
  a compromised validator, signer or maintainer key.

## Automated checks

These checks run in GitHub Actions on every pull request and on every push to
`main`, unless noted otherwise.

| Check | What it covers | Workflow |
|---|---|---|
| Tests | Root module and `evmd`, built with the `test` tag | `ci.yml` |
| [govulncheck](https://go.dev/doc/security/vuln/) | Known Go vulnerabilities reachable from the code: source of every Go module, plus the release binary; also daily | `security.yml`, `ci.yml`, `build.yml` |
| [golangci-lint](https://golangci-lint.run/) | gosec, staticcheck, errcheck and others; depguard bans obsolete packages, forbidigo bans raw private-key import (new findings fail a pull request) | `lint.yml` |
| `scripts/check-forks.sh` | Forks replaced at the same xitcoin-xtc commit everywhere, no local replaces, banned packages not linked | `security.yml` |
| [zizmor](https://docs.zizmor.sh/) | Security of the GitHub Actions workflows | `security.yml` |
| `scripts/verify-actions-pinned.sh` | Every action pinned to a commit SHA or image digest | `ci.yml` |
| [Gitleaks](https://github.com/gitleaks/gitleaks) | Secrets in the full Git history | `secret-scan.yml` |
| Dependency review | New dependencies with high or critical advisories (pull requests) | `dependency-review.yml` |
| npm audit | Contract tooling dependencies (pull requests) | `dependencies.yml` |
| [Slither](https://github.com/crytic/slither) | Solidity static analysis (pull requests changing contracts) | `slither.yml` |
| CodeQL | Code scanning (GitHub default setup) | repository settings |
| Dependabot | Weekly updates of Go modules, npm packages and actions | `.github/dependabot.yml` |

Tagged releases publish SHA-256 checksums and a
[build provenance attestation](https://docs.github.com/actions/security-for-github-actions/using-artifact-attestations)
for the binary and the genesis file. Verify one with
`gh attestation verify xitcoind --repo xitcoin-xtc/pos-chain`.

## Audit reports

[`docs/audits/`](docs/audits/) holds the 2025 Sherlock audit of upstream Cosmos
EVM, which this repository derives from. It does not cover the Xitcoin
modules, the changes made here to the upstream code, or the forks.
