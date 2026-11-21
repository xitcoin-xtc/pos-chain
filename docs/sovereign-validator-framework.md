# Xitchain Sovereign Validator Framework

Status: planned for mainnet, not implemented on chain
Reference date: 20 August 2026 (seat amounts aligned on 22 November 2026)

Member State seats and their vesting are planned. The running chain software
does not implement them: it only caps the validator set at 258 and admits
validators through the validator-admission module. No position is reserved,
funded or vesting on chain today.

## Purpose

The framework plans one institutional validator position for each of the
193 United Nations Member States recorded in the Xitchain 2026 Member State
Seat Allocation (`docs/sovereign-allocation-2026.md`). A position is attached permanently to the relevant State, identified
by its canonical ISO3 code. It is not attached to a president, minister,
administration, signatory or infrastructure provider.

Successive administrations of the same State may transfer the institutional
governance mandate to their authorized representatives and operating teams
without changing the State position, its accounting history or its remaining
allocation.

## Validator capacity

The planned validator capacity is:

- 193 Member-State positions (planned, not implemented on chain);
- 65 general validator positions;
- 258 positions in total;
- 5,000,000 XTC minimum self-delegation for every activated validator.

The same minimum applies to founder, institutional and general validators.
The total capacity remains fixed at 258 positions. A governance-approved
increase in the Member-State category would reduce the general category
one-for-one; a decrease increases the general category one-for-one. Any change
to the total capacity requires a separate governance decision.

The planned registry follows the published United Nations Member-State list.
Adding or removing a Member-State position would require an on-chain proposal supported
by an objective membership change. The proposal must identify any allocation
funding and cannot create supply, rewrite vested allocations or reduce accrued
rights.

A Member-State position is not active and receives no reward while its
admission conditions are not satisfied.

## Institutional control

Each sovereign position separates four roles:

1. **State position** — the permanent ISO3 position and its immutable history;
2. **institutional controller** — the currently authorized State-controlled
   group or multisignature policy;
3. **operational mandatary** — a time-limited and revocable operating mandate;
4. **validator operator** — the team authorized to operate the validator for
   the duration of that mandate.

A change of government or service provider must not sell, extinguish or move
the State position. It transfers the governance and operating mandate between
successive authorized teams representing the same State. The transition must
support a future effective date, the end of the former mandate, the beginning
of the successor mandate and a complete public event history.

No individual mandatary may permanently appropriate the position or obtain an
unrestricted right to transfer the position's controlled stake.

## Activation requirements

A sovereign position may be activated only after all of the following are
verified:

- the State identity and ISO3 position;
- a valid institutional controller;
- a dated operational mandate;
- compliant validator infrastructure operated under the active mandate;
- at least 5,000,000 XTC of State-provided self-delegation;
- approved reward and withdrawal destinations;
- acceptance of uptime, slashing, reporting and operational-security requirements.

The sovereign allocation does not satisfy the minimum self-delegation. The
State must provide the required commitment independently.

## Seat pool

The seat pool is 386,000,000 XTC: 1,500,000 XTC for each of the 193 Member
States (289,500,000 XTC) plus 96,500,000 XTC shared in proportion to the square
root of each Member State's population. Populations are the United Nations
World Population Prospects 2024 figures (medium variant, 1 July 2026) published
for each Member State, used as they are, with no territory added to or removed
from any State. Allocations are whole XTC, rounded by the largest-remainder
method, and total exactly 386,000,000 XTC. They range from 1,511,216 XTC
(Tuvalu) to 5,954,251 XTC (India), with a median of 1,858,884 XTC (Hungary).
The method is recorded in `docs/sovereign-allocation-2026.md`; the canonical
dataset and the script that produces it are in the
[testnets repository](https://github.com/xitcoin-xtc/testnets/tree/main/xitcoin-testnet-1).

The pool is funded from the Xitchain reserve of Kcalb Ltd, held on Cronos by
the wallet `0x4993F2012D72C3cFb67eBde946481b9dbFB3F254`. At mainnet the pool
crosses the bridge one for one, locked on Cronos, so the total supply stays
5,250,000,000 XTC.

The allocation is a finite support grant for an activated sovereign validator
position. It is separate from ordinary validator rewards,
does not create new supply and does not grant automatic admission.

## Activation-based five-year release

Each position receives its own deterministic vesting schedule when it is
activated. A position may therefore join years or decades after network launch
without requiring a global calendar change.

Activation stores the applicable schedule version, activation reference,
fixed allocation, vesting duration and amount already released in on-chain
state. Later protocol upgrades may introduce a new schedule version for future
activations, but must not silently rewrite an active position's accrued rights.

The schedule accrues linearly for five years of eligible service, measured by
the chain's canonical block accounting:

```text
vested allocation =
canonical ISO3 allocation x eligible service blocks / five-year service blocks

claimable allocation = vested allocation - allocation already released
```

No quarterly transaction or recurring company intervention is required. The
authorized institutional controller may claim the amount accrued at any time.
An unclaimed amount remains recorded as claimable.

Accrual continues only while the position satisfies the applicable service
conditions, including:

- minimum self-delegation;
- valid institutional and operational mandates;
- validator availability;
- absence of unresolved double-signing or other disqualifying fault;
- compliance with reporting and security obligations.

A failed condition pauses future accrual and opens a defined remediation
period. Reactivation resumes the schedule without retroactive accrual for the
suspended interval. It must not permit arbitrary seizure of an amount already
vested. Cancelled or unvested amounts remain in the seat pool.

An inactive position's allocation remains in the seat pool
until valid activation or an explicit network-governance decision governed by
the published reserve rules. Company availability must not be required for the
vesting calculation to continue.

After five years of eligible service have fully vested, no further sovereign
allocation is created. The validator continues under the same staking,
commission, delegation, fee-distribution and slashing rules as other
validators.

## Ordinary validator rewards

Ordinary rewards are distinct from the sovereign allocation. An active
sovereign validator may receive, under the same network rules as other active
validators:

- rewards attributable to its self-delegation;
- validator commission on third-party delegations;
- its share of funded validator incentives;
- its share of transaction-fee distribution.

The sovereign allocation must not be included when calculating whether the
State supplied the 5,000,000 XTC activation commitment.

The Validator Incentive treasury remains pre-funded and has no mint or burn
permission. Transaction fees are accounted for by the chain distribution
mechanism unless a separately reviewed governance upgrade defines another
route.

## Continuity and suspension

The State position remains recorded if a government changes, a mandate
expires, an operator is replaced or the validator is suspended.

If no valid mandatary exists, or self-delegation falls below the required
minimum:

- validation and future allocation releases are suspended;
- the position remains reserved for the same State;
- already vested ownership and the accounting history remain recorded;
- reactivation requires the admission conditions to be satisfied again.

Buying tokens from a State or former operator never transfers control of the
State position to the buyer.

## Required protocol implementation

This policy requires explicit implementation and independent review before it
can be activated. At minimum, the protocol must provide:

- a unique 193-entry ISO3 Member-State position registry;
- separate reserved and general admission capacity;
- institutional-controller and dated-mandate records;
- safe institutional succession between successive governance and operating
  mandates of the same State;
- per-position activation height, eligible-service vesting and
  remaining-allocation accounting;
- immutable schedule versioning and reviewed upgrade migration rules;
- reward accounting separated from sovereign allocation accounting;
- suspension, remediation and reactivation procedures;
- immutable events and public read-only queries;
- invariants preventing double allocation, unauthorized control, premature
  release, reserve overdraft and mint-based funding.

Until those controls are implemented, tested, audited and activated by the
required governance process, the allocation index remains a reference only and
does not transfer tokens or activate validators.
