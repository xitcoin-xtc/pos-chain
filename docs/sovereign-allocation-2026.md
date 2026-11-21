# Xitchain 2026 Member State Seat Allocation

Status: planned for mainnet, not implemented on chain

Member State seats and their vesting are a planned policy. The running chain
software does not implement them: it only caps the validator set at 258 and
admits validators through the validator-admission module. Nothing in this
document transfers tokens, activates a validator or reserves a position on
chain today.

## Reference framework

The index defines a deterministic reference allocation for the 193 United
Nations Member States recorded at the 2026 reference date.

The index records the fixed allocation planned for each State position. It
does not, by itself, transfer tokens, activate a validator or create automatic
validator admission. Release would require an activated position and the
protocol controls described in `docs/sovereign-validator-framework.md`, which
are not implemented yet.

## Seat pool and formula

The seat pool is 386,000,000 XTC, the same amount as the sovereign reserve
account of the running testnet genesis:

- 289,500,000 XTC equal share: 1,500,000 XTC for each of the 193 Member States;
- 96,500,000 XTC population pool, shared in proportion to the square root of
  each Member State's population.

```text
allocation_i = 1,500,000
             + 96,500,000 x sqrt(population_i) / sum(sqrt(population_j))
```

Allocations are whole XTC. The units left by rounding are given to the largest
fractional remainders, with the ISO3 code as the deterministic tie-breaker. The
193 allocations total exactly 386,000,000 XTC.

| Example | Population (1 July 2026) | Allocation |
| --- | ---: | ---: |
| Smallest: Tuvalu | 9,362 | 1,511,216 XTC |
| Median: Hungary | 9,585,818 | 1,858,884 XTC |
| Largest: India | 1,476,625,576 | 5,954,251 XTC |

## Population reference

Population values are the United Nations World Population Prospects 2024,
medium variant, for 1 July 2026: the figure the United Nations publishes for
each Member State, used as it is.

No territory is added to or removed from any Member State. China, for example,
uses the figure the United Nations publishes for the Member State "China",
without adding Hong Kong, Macao or Taiwan. Statistical records that are not
Member States do not create positions and are not added to another position.
This is a neutral technical rule, not a political or diplomatic determination.

Source: <https://population.un.org/wpp/>

## Funding

The pool is funded from the Xitchain reserve of Kcalb Ltd, held on Cronos by
the wallet `0x4993F2012D72C3cFb67eBde946481b9dbFB3F254`. At mainnet the pool
crosses the bridge one for one, locked on Cronos, so the total supply stays
5,250,000,000 XTC. The allocation does not create new supply.

## Economic function

The allocation is a finite contribution supporting an activated Member State
validator position. It is additional to, and never a substitute for, the
State-provided minimum self-delegation of 5,000,000 XTC.

After activation, the fixed allocation is intended to vest linearly over five
years of eligible service (20% a year) under a schedule created for that
position. Activation may occur at any future date and requires no global
calendar change. The schedule is subject to the institutional, staking,
availability and security conditions described in the sovereign validator
framework.

Ordinary staking rewards remain separate. After the allocation has fully
vested, the validator continues under the same staking, commission,
delegation, fee-distribution and slashing rules as other validators.

## Validator capacity

The planned configuration is:

- 258 maximum validators (the only part enforced by the running software);
- 193 Member State positions (planned, not implemented on chain);
- 65 general validator positions;
- 5,000,000 XTC minimum self-delegation for every activated validator.

The 258-position capacity is intended to be conserved. A governance-approved
increase in the Member State category would reduce the general category by the
same number of positions, and a decrease would increase it. Changing the total
validator capacity would require a separate governance decision.

Changes to the Member State registry would require an on-chain governance
proposal tied to an objective change in United Nations membership. A registry
change could not create tokens, rewrite vested allocations or reduce an active
position's accrued rights. Any new allocation would have to be funded from an
identified, unallocated reserve approved in the same governance process.

The same minimum applies to founder, institutional and general validators. A
State must supply this commitment independently; its seat allocation cannot be
counted toward the activation minimum.

Validator admission remains subject to the validator-admission module. The
chain defines the 5,000,000 XTC protocol minimum but does not implement the
Member State position registry, institutional succession, mandates or the
activation-based five-year vesting. Those controls must be implemented, tested
and independently reviewed before any activation.

## Institutional continuity

Each ISO3 position would remain attached to the relevant State. Successive
administrations of that State could transfer the institutional governance and
operating mandate to their authorized successors without replacing the State
position, its history or its remaining allocation.

The position is not attached permanently to an individual office-holder or
service provider. Buying tokens from a State or former operator does not grant
control of the State position.

## Canonical identifiers

Names, United Nations M49 identifiers and ISO codes are the canonical
identifiers. Flags may be used as non-authoritative presentation assets when
their source and licence are recorded.

## Canonical files

The canonical dataset and the script that produces it are published in the
testnets repository:

- [`xitcoin-testnet-1/sovereign-allocation-index-2026.csv`](https://github.com/xitcoin-xtc/testnets/blob/main/xitcoin-testnet-1/sovereign-allocation-index-2026.csv)
- [`xitcoin-testnet-1/sovereign-allocation-index-2026.json`](https://github.com/xitcoin-xtc/testnets/blob/main/xitcoin-testnet-1/sovereign-allocation-index-2026.json)
- [`xitcoin-testnet-1/compute-sovereign-allocation-2026.py`](https://github.com/xitcoin-xtc/testnets/blob/main/xitcoin-testnet-1/compute-sovereign-allocation-2026.py)

Run `python3 compute-sovereign-allocation-2026.py --check` in that folder to
recompute and compare the files.
