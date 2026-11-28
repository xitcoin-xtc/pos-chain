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

## Security checks

The automated checks are listed in [SECURITY.md](../SECURITY.md). Run the
main ones locally before opening a pull request:

```bash
make vulncheck                          # govulncheck, root and evmd
make lint-go                            # golangci-lint (run it in evmd/ too)
bash scripts/check-forks.sh             # fork replacements and banned packages
bash scripts/verify-actions-pinned.sh   # GitHub Actions pinned by SHA
```

The Cosmos SDK and go-ethereum forks are described in [forks.md](forks.md).
