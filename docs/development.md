# Development

This guide describes how to build and test the Xitcoin node from source.

## Requirements

- Go version declared in `go.mod`
- A supported Linux, macOS or Windows development environment
- Git

## Build

From the repository root:

```bash
make build
```

The resulting daemon is written to:

```text
build/xitcoind
```

## Test

Run tests in both Go modules with the test build tag:

```bash
GOWORK=off go test -tags=test ./...
(cd evmd && GOWORK=off go test -tags=test ./...)
```

Some permission-oriented client tests must run as a non-privileged user.

## Scope

This repository contains the Xitcoin chain implementation. It does not contain production keys, node state, private genesis files, backups or production infrastructure configuration.

For upstream attribution, see [UPSTREAMS.md](../UPSTREAMS.md).

## Current security and qualification

[Current technical review](security-review-2026-11-04.md) records fresh source
and versioned-binary evidence, full tests, all six historical advisory identities
and the remaining limitations. Root tests alone do not cover evmd. The current
versioned workflow tests both checkout modules and the exact published PR head,
checks graph/checksum identity and scans a symbol-bearing versioned binary.
It does not reuse September results. The strict scanner gate accepts no advisory
exceptions; source drift, new findings, scanner errors and expiry block CI.

The [September assessment](../SECURITY-ASSESSMENT.md) and historical qualification
scripts remain historical records, not clearance of current code. Neither the
current automated technical review nor a green CI run is independent approval
or authorization to deploy a production chain.
