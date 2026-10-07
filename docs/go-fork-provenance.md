# Go fork provenance

Refs #33 and #44. [SECURITY-ASSESSMENT.md](../SECURITY-ASSESSMENT.md) is the
current source of truth for the six advisory dispositions and residual risks.
The 2026-09-14 assessment renews the bounded CI disposition through 2026-09-21;
it is performed by the PR author, without independent approval or release
acceptance. The current machine record is [security-assessment.json](security-assessment.json).
The dependency identities and sums in [go-fork-provenance.json](go-fork-provenance.json)
remain current. Its top-level 2026-09-05 expiry and false acceptance are historical
fields preserved for the acquired qualification, not the active gate decision.

## Versioned dependency sources

Both forks preserve upstream module declarations. The two application modules use remote, immutable Go replacements. The original module identities and all six advisory records in the identity manifest remain authoritative for review. Scanner silence under a fork name is not clearance.

- `github.com/cosmos/cosmos-sdk` required `v0.54.4` → `github.com/xitcoin-xtc/cosmos-sdk v0.54.5-0.20260914091530-52ff14a25bee`. [Signed source](https://github.com/xitcoin-xtc/cosmos-sdk/commit/52ff14a25bee524f0e48ae7f00664443bcb22335), [upstream base](https://github.com/cosmos/cosmos-sdk/commit/dedeb7c80a91c47ae83f5352e29c3dd34e4a3fc6). Go module sum `h1:ZbENildMBRKKYIJ4Otsm1Yk1WFPVfvitYnx3B+/JdnE=`; go.mod sum `h1:/1rNNn6uo2iw2ZEp9N8GK9PdWgH0OmxDly8MEAArYPI=`; qualified patch SHA256 `dc46d5e1e8497222040f0688277217599ce884e52ebdc820ba65f6a79a74902c`.
- `github.com/ethereum/go-ethereum` required `v1.16.9` → `github.com/xitcoin-xtc/go-ethereum v1.17.2-cosmos-0.0.20260913233706-e09f79643cd4`. [Signed source](https://github.com/xitcoin-xtc/go-ethereum/commit/e09f79643cd464404876f3565ce0a8fd8c0aeb16), [upstream base](https://github.com/cosmos/go-ethereum/commit/d99d6fa2c8d98b7cd653de4a9386d2da3db8f25c). Go module sum `h1:9k4Qh3iIr8gxPK6qvgEYaDrb3vMkrH+ullN4vuNQuD4=`; go.mod sum `h1:QtIPOaMKuz4zDZoQQ72nJJ9i5njr+MyRMl2tv7XteJQ=`; qualified patch SHA256 `8316073e371bdccaf788bd0f881d23094f1d486d1d7202d3edaa31fdf2416db4`.

Prepublication CI: [run 34788124256](https://github.com/xitcoin-xtc/pos-chain/actions/runs/34788124256), [run 34791392591](https://github.com/xitcoin-xtc/pos-chain/actions/runs/34791392591), [run 34793489922](https://github.com/xitcoin-xtc/pos-chain/actions/runs/34793489922), [run 34794203018](https://github.com/xitcoin-xtc/pos-chain/actions/runs/34794203018), [run 34788224704](https://github.com/xitcoin-xtc/pos-chain/actions/runs/34788224704), [run 34789811082](https://github.com/xitcoin-xtc/pos-chain/actions/runs/34789811082).

SDK production armor uses ProtonMail/go-crypto v1.4.1. Full upstream tests exposed its deliberate CRC24 omission; the fork restores Tendermint's checksum corruption check using the maintained encoder, with no copied or imported obsolete OpenPGP implementation. Existing encryption algorithms and upstream keyring expectations are preserved. Public golden vectors and CRC regression tests run in the SDK; the original dynamic legacy oracle remains separately available in the review packet. Original root/evmd compatibility tests are retained.

The separate historical CRC reader oracle retains 135 differential cases; its local checkpoint PASS is preserved and the same oracle passed against published SDK 52ff14a25bee524f0e48ae7f00664443bcb22335 (archived sdk-published-legacy-oracle result).

DTLS v2 remains in the module requirement graph through CometBFT v0.39.4 and IBC v11.2.0. Root `go mod why -m` reports it is not needed, and `go list -deps ./...` contains only DTLS v3. This residual module requirement and GO-2026-4479 remain review items. The provenance checker reports residual requirements and rejects effective production imports of DTLS v2 or STUN v2.

The Geth fork changes STUN to v3.1.5 with UDP4 preserved. Full root upstream tests passed; the initially failing keeper module required Go-generated tidy changes. Keeper build/tests, lint, generated-source and bad-dependency checks then passed. The first combined test-command failure is retained rather than relabeled a success.

## Licenses and corresponding source

The SDK core retains Apache-2.0 and its per-file notices. The enterprise modules retain their separate Cosmos Labs evaluation licenses; they were evaluated in upstream qualification and are not added to the application. ProtonMail armor retains its BSD-style license. Geth library files retain LGPL notices, and commands/components retain applicable GPL and per-file notices. Pion STUN retains its MIT license. The fork sources retain the original license files and explicit modification notices. No deployment binary is published by this integration; a symbol-bearing qualification binary is built and scanned on an isolated runner, then only its hash and build information are retained. A later distributor must assess applicable source and relinking obligations for its actual binary distribution.

## Assessment and merge status

The current assessment explains each original advisory against exact source,
including the SDK slashing/OSV reference discrepancy, CometBFT's active fixed
blocksync path, residual DTLS v2 requirements and armor compatibility limits.
The gate validates the module actually scanned, exact replacements and sums,
root anchor and obsolete imports. It also verifies the current assessment,
its explanation and four module locks before scanning. An altered assessment,
lock drift, new finding, scanner error or expiry fails closed.

No independent approval is supplied. GitHub protections and release-specific
review requirements remain unchanged; see the normative-source discussion in
SECURITY-ASSESSMENT.md. Before a later merge, recheck the exact head and all
requirements then in force. No fusion or deployment is part of PR44 assessment.

Historical R1–R3 gate validation: the then-current `python3 scripts/test-govulncheck-gate.py` ran seven groups of isolated wrapper simulations (Go, date and scanner are simulated; the wrapper and provenance checker are real). These simulations include root/evmd drift, missing or wrong replacements, checksums, root anchor, obsolete imports, scanner errors and expiry. Separately, the real acquired root/evmd dependency graphs and provenance passed; real `govulncheck -scan=module` runs collected GO-2025-3442 and GO-2026-5932 and the wrapper rejected the expired review. Module scans do not establish symbol reachability. Real mutation tests reject invalid locks/sums/imports before a scanner sentinel; they are not vulnerability scans. No acquired SDK/Geth/application suite was rerun for R1–R3.

Historical patch-only evidence remains under `security-remediation-20260913` and at signed head `52c0ea290645c4b53b5648178f7176dfdc5a401c`. It describes the earlier local graph; current locks and this provenance file describe the versioned candidate. The complete integration commit and the root library revision consumed by evmd are recorded separately to avoid a self-referential Go version.

Root library consumed by evmd: `v0.1.0-testnet.4.0.20260914101327-3280cdeee8f6`, signed commit `3280cdeee8f6d362c332572d14ae26a7ec75b8d4`. Root Go sources and locks match that immutable library revision.

The first full versioned evmd run exposed stale integration fixtures: Cosmos-prefixed local addresses, an invalid public Amino address checksum, and permissionless ERC20 scenarios relying on a different genesis default. Four test helper files now preserve address bytes under the configured prefix, use valid local receivers, and set the tested permissionless baseline explicitly. Assertions and production behavior are unchanged. Local preflight passed the entire ERC20 suite and then Ledger after the fixture checksum correction; initial failures are retained.

The CI continuation records the exact prior run and artifact digests and rejects changes outside the four fixture files, immutable evmd root pin/sums, and continuation diagnostics. It preserves the passed full root suite and all previously passed evmd packages; the two formerly failing suites run against the new immutable root. Continuation run [34832506821](https://github.com/xitcoin-xtc/pos-chain/actions/runs/34832506821) passed both modules at `5e24531951873ec8d0cf4403d118c45f47fa641e`, including the affected suites, current source diagnostics and a symbol-bearing versioned binary scan. The R1–R3 gate/documentation correction preserves these qualified sources, fixtures, locks, manifests and historical results; only the changed gate is tested again. This guarded reuse is technical evidence, not independent security approval.
