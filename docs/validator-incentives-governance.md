# Validator Incentive governance operations

Status: approved economic specification; implementation aligned in PR #15
Reference date: 27 August 2026

## Authority model

The Validator Incentive module's on-chain authority is stored in its genesis
state. On Xitcoin Testnet it is the 2-of-3 administrative multisig
`xtc1vza8zsgvrfwmve084ytd8xqdkkm7u9e5csctc2`, set in the testnet genesis. It is
not the Cosmos governance module account. Executable governance proposals are
disabled on every node. The mainnet authority is not decided yet; the node
software refuses the testnet multisig on the mainnet chain ID.

This authority may update the treasury release policy or pause future reward
calculation with a transaction signed by two of its three keys. It does not approve or
revoke validators. Validator admission remains a separate policy and execution
path.

## Launch parameters

| Parameter | Mainnet launch value |
| --- | ---: |
| Initial eligible bonded stake | 20,000,000 XTC |
| Initial Validator Incentive Treasury | 20,000,000 XTC |
| Treasury annual release rate | 1,000 basis points (10%) |
| Recalculation interval | One day |
| Protocol inflation | 0% |
| Treasury mint or burn permission | None |

The release rate applies to the current funded treasury balance. It is not a
fixed APR target, an APY promise or a percentage of global token supply.

## Deterministic daily calculation

At each daily boundary, the module reads the canonical Bank and Staking keeper
state. Neither balance may be supplied by a transaction.

```text
annualized_reward_capacity
= current_treasury_balance * release_rate_basis_points / 10,000

derived_apy
= annualized_reward_capacity / current_eligible_bonded_stake

daily_reward_pool
= annualized_reward_capacity / days_per_year

participant_daily_share
= daily_reward_pool
  * participant_eligible_stake
  / total_eligible_bonded_stake
```

The implementation uses the network's canonical block-year constant to map one
day and one year to block intervals. Integer rounding must be deterministic and
must never make cumulative distributions exceed the funded balance.

## Safety boundaries

The module must enforce all of the following:

- the treasury balance is read from the Bank keeper;
- eligible bonded stake is read from the Staking keeper;
- the next daily capacity is derived from those live balances;
- no message accepts a manually selected APR or reward budget;
- a treasury deposit affects the next daily calculation without changing the
  release-rate parameter;
- zero treasury balance produces zero treasury-funded rewards;
- zero eligible stake produces no distribution and no division by zero;
- the treasury module account has no mint or burn permission;
- cumulative transfers cannot exceed verified treasury funding;
- failed bank transfers do not advance distribution accounting;
- reward accounting remains separate from validator admission;
- bridge lock/mint and burn/unlock accounting remains separate from rewards.

The launch release rate is 10%. A future rate change requires an authorized,
observable on-chain parameter update and does not retroactively alter completed
daily calculations.

## Fees and other funding

Ordinary transaction-fee distribution follows the active chain distribution
parameters. Fees do not enter the Validator Incentive Treasury unless a
separately reviewed protocol route transfers them there.

Verified bridge funding, approved application revenue or approved buybacks may
credit the treasury. Once credited, the new balance is included in the next
daily calculation. Funding grants no withdrawal or governance authority.

## Supply boundary

The current Cronos contract `totalSupply` is reconciled independently of the
staking calculation. Confirmed burns reduce that value. The global supply is
not the denominator of the derived APY.

The bridge must preserve:

```text
bridge_authorized_xtc_on_xitcoin
<= canonical_xtc_locked_on_cronos
```

The incentive treasury must preserve:

```text
cumulative_treasury_rewards
<= cumulative_verified_treasury_funding
```

## Required read-only endpoints

The aligned implementation must expose enough state to reproduce the daily
calculation independently:

- release rate in basis points;
- daily and annual block constants;
- current funded treasury balance;
- current eligible bonded stake;
- current annualized reward capacity;
- current derived APY;
- current daily reward pool;
- last calculation height and next calculation height;
- cumulative funded distributions.

## Implementation status

PR #15 aligns the module with this specification. It replaces the fixed-APR
and manual-budget paths, creates deterministic daily snapshots, transfers each
block's funded share to the canonical fee collector, exposes reproducible
read-only state, and includes a versioned state migration.

Independent review, testnet rehearsal and explicit release approval remain
mandatory before production activation.

## Release boundary

This document is the approved economic specification, not a deployment record.
Production activation additionally requires final code review, full CI,
testnet rehearsal, supply reconciliation, a genesis or upgrade plan, security
review and public deployment evidence.
