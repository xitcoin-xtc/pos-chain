# Go forks

The node replaces two upstream Go modules with forks maintained under
`xitcoin-xtc`. The same `replace` directives are used in the root module,
`evmd`, `tests/speedtest` and `tests/systemtests`.
[`scripts/check-forks.sh`](../scripts/check-forks.sh), run by the Security
checks workflow, enforces this.

| Upstream module | Fork | Upstream base | Fork commit | Version in `go.mod` |
|---|---|---|---|---|
| `github.com/cosmos/cosmos-sdk` | [`xitcoin-xtc/cosmos-sdk`](https://github.com/xitcoin-xtc/cosmos-sdk) | `v0.54.5` ([`4c5119fe2514`](https://github.com/cosmos/cosmos-sdk/commit/4c5119fe2514241b963a6eb75972761797a5cb1d)) | [`bb5933a0bf0d`](https://github.com/xitcoin-xtc/cosmos-sdk/commit/bb5933a0bf0d4a3f574daf20c32325ed2ae946bb) | `v0.54.5-0.20261121191520-bb5933a0bf0d` |
| `github.com/ethereum/go-ethereum` | [`xitcoin-xtc/go-ethereum`](https://github.com/xitcoin-xtc/go-ethereum) | `cosmos/go-ethereum` `v1.17.2-cosmos-0` ([`d99d6fa2c8d9`](https://github.com/cosmos/go-ethereum/commit/d99d6fa2c8d98b7cd653de4a9386d2da3db8f25c)) | [`e09f79643cd4`](https://github.com/xitcoin-xtc/go-ethereum/commit/e09f79643cd464404876f3565ce0a8fd8c0aeb16) | `v1.17.2-cosmos-0.0.20260913233706-e09f79643cd4` |

## Patches

Cosmos SDK, on top of `v0.54.5`:

- `e0fd3b1e55e7` crypto: key armor (`crypto/armor.go`) uses the maintained
  `github.com/ProtonMail/go-crypto` instead of `golang.org/x/crypto/openpgp`,
  which is unmaintained (GO-2026-5932). Existing armored keys keep working and
  the CRC24 checksum is still verified; `crypto/armor_compat_test.go` checks
  historical vectors. Every SDK module's `go.mod` gains the new requirement.
- `bb5933a0bf0d` docs: provenance note and changelog entry.

go-ethereum, on top of `v1.17.2-cosmos-0`:

- `e09f79643cd4` p2p/nat: moves STUN from `github.com/pion/stun/v2` to `v3`,
  which drops `github.com/pion/dtls/v2` (GO-2026-4479); UDP4 is kept, and
  `p2p/nat/stun_local_test.go` covers it.

Each patch can be dropped once upstream ships an equivalent change; check
this on every rebase. golangci-lint (depguard) and `check-forks.sh` reject
`golang.org/x/crypto/openpgp`, `pion/dtls/v2` and `pion/stun/v2` in the node.

## Rebase on a new upstream release

1. In the fork, branch from the upstream tag, for example
   `git switch -c xitcoin/v0.54.6 v0.54.6` (fetch the upstream tags first).
2. Cherry-pick the patch commits listed above and resolve conflicts. Run the
   upstream tests of the touched packages (`go test ./crypto/...` in the SDK;
   `go test ./p2p/nat/...` and `go build ./...` in `cmd/keeper` for geth) and
   `go mod tidy` where a patch changes a `go.mod`.
3. Push, review the diff against the upstream tag in a pull request, then tag
   the result `<upstream tag>-xitcoin.<n>` (for example `v0.54.6-xitcoin.1`).
   `check-forks.sh` requires the commit to be on a tag or a protected branch.
4. In this repository, point every module at the new version and bump the
   upstream requirement to match:

   ```bash
   for m in . evmd tests/speedtest tests/systemtests; do
     (cd "$m" && go mod edit \
       -replace github.com/cosmos/cosmos-sdk=github.com/xitcoin-xtc/cosmos-sdk@v0.54.6-xitcoin.1 \
       -require github.com/cosmos/cosmos-sdk@v0.54.6 && go mod tidy)
   done
   ```

5. Update the table above, then run `bash scripts/check-forks.sh`, the tests
   of the root module and `evmd`, and govulncheck. If the upstream release
   changes state or module consensus versions, add an upgrade handler.

## Licenses

The forks keep the upstream license files and per-file notices: Cosmos SDK
under Apache-2.0 (its `enterprise/` modules have separate licenses and are not
used by the node), go-ethereum library code under LGPL-3.0 and its commands
under GPL-3.0, `ProtonMail/go-crypto` under BSD-3-Clause and `pion/stun` under
MIT. The source of every fork commit used by a release is public at the links
in the table above.
