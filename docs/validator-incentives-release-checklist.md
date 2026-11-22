# Validator Incentives public release checklist

Status date: 2 September 2026

This checklist is the canonical technical release record. The public
documentation at https://xitcoin.com/docs/ describes the product and links here instead of duplicating test logs, audit
evidence or deployment procedures.

## Current readiness

- Canonical public-testnet acceptance: **complete** on 2 September 2026
- Active testnet identity: `xitcoin-testnet-v2-1`
- Active genesis SHA-256: `5db34acf6496b2c76a6f516e0eb605caef6762552584ddbed7c8703239f33d72`
- Source and CI security controls: **present on `main`**
- Reproducible release-artifact workflow: **present on `main`**; V2 genesis packaging corrected by PR #30
- Mainnet release authorization: **not granted**
- Mainnet deployment: **not performed**
- Production funds moved: **none**

Readiness is recorded through evidence-backed gates rather than a percentage.
Completion of the public testnet does not by itself authorize a mainnet launch.

## Completed

- [x] daily balance-derived reward policy specified;
- [x] canonical Staking keeper used for eligible bonded stake;
- [x] canonical Bank keeper used for treasury balance;
- [x] authority configured in genesis (on the testnet, the 2-of-3 administrative multisig; executable governance proposals are disabled);
- [x] treasury account has no mint or burn permission;
- [x] public read-only parameter, period and treasury queries;
- [x] CLI and Cosmos message handlers;
- [x] application, store, keeper and genesis wiring;
- [x] governance, staking and EIP integration regression tests;
- [x] corrected Ethereum Lists pre-launch metadata merged through PR #8621;
- [x] DefiLlama Chainlist submission outcome recorded;
- [x] public guide separated from technical verification records.

PR #15 implements the approved daily dynamic model. Its merge does not by
itself authorize activation.

## Required before public activation

- [ ] complete an independent security review of the final module diff;
- [x] align the module with the 10% treasury-release policy and daily derived APY;
- [x] remove fixed-APR, 20%-ceiling, quarterly-transition and manual-budget paths;
- [x] expose reproducible daily calculation state through read-only queries;
- [x] integrate and test automatic funded distribution through the canonical fee collector;
- [ ] resolve all review findings and rerun the full repository CI matrix;
- [ ] reconcile canonical total supply across PoS, Xitcoin EVM and Cronos EVM;
- [x] document bridge lock/mint and burn/unlock accounting with invariants;
- [ ] define the initial treasury funding transaction and source-of-funds record;
- [ ] rehearse the parameter update and automatic daily distribution on testnet;
- [ ] publish the exact genesis migration or chain-upgrade plan;
- [x] execute a persistent testnet deployment with public endpoints;
- [ ] run load, failure, restart, upgrade and rollback exercises on testnet;
- [x] configure baseline node healthchecks and local failure alerts;
- [ ] complete monitoring dashboards and incident runbooks;
- [ ] obtain release approval with reproducible build artifacts and checksums;
- [x] publish final public-testnet user documentation and deployment evidence;
- [ ] perform a limited production rollout before unrestricted activation.

## External publication follow-up

- Ethereum Lists corrective PR #8621: merged on 21 August 2026; Xitcoin mainnet
  is marked pre-launch.
- DefiLlama Chainlist PR #3073: closed without merge on 28 August 2026. Any new
  submission must be based on the current upstream schema and verified network
  status rather than reusing the closed branch blindly.
- Xitcoin Guide PRs #9 and #10: merged; obsolete staging and pending-cutover
  language is removed and protected by Guide CI.
- Xitcoin PoS Chain PR #30: merged; the Linux AMD64 artifact workflow now
  packages the active V2 genesis and verifies its canonical checksum.

## Release rule

No percentage, passing test suite or merged metadata pull request authorizes a
production deployment. Activation requires every applicable item above to be
closed with evidence and a separately approved operational change.
