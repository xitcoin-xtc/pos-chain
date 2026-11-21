#!/usr/bin/env python3
"""Verify the planned Member State seat allocation in the networks submodule.

The canonical dataset and its computation script live in xitcoin-xtc/testnets
(networks/xitcoin-testnet-1/). This check recomputes the allocation
independently from the population column and compares it with the published
CSV and JSON, so a change to the method or the numbers cannot go unnoticed.

Canonical method (planned for mainnet, not implemented on chain):
- pool 386,000,000 XTC = 1,500,000 XTC x 193 Member States + 96,500,000 XTC;
- the 96,500,000 XTC population pool is shared by the square root of each
  Member State's UN WPP 2024 population (medium variant, 1 July 2026), as
  published for that Member State: no territory is consolidated, so China
  does not include Hong Kong, Macao or Taiwan;
- whole XTC; the units left by rounding go to the largest remainders, ties by
  ISO3 code.
"""
import csv
import json
import subprocess
import sys
from decimal import Decimal, getcontext
from pathlib import Path

getcontext().prec = 60

repository = Path(sys.argv[1]).resolve() if len(sys.argv) == 2 else Path(__file__).resolve().parents[1]
network = repository / "networks" / "xitcoin-testnet-1"

POOL = 386_000_000
EQUAL = 1_500_000
STATES = 193
POPULATION_POOL = POOL - EQUAL * STATES
ATOMIC = 10**18

with (network / "sovereign-allocation-index-2026.csv").open(encoding="utf-8", newline="") as handle:
    rows = list(csv.DictReader(handle))
with (network / "sovereign-allocation-index-2026.json").open(encoding="utf-8") as handle:
    metadata = json.load(handle)

# The earlier method consolidated territories into Member States. It is no
# longer part of the canonical dataset.
assert not (network / "territorial-consolidation-2026.csv").exists(), "territorial consolidation file must be absent"

assert isinstance(metadata, dict)
assert metadata["status"] == "planned_for_mainnet_not_implemented_on_chain"
assert metadata["selection_basis"] == "united_nations_member_states"
assert metadata["position_count"] == STATES
assert metadata["population_transformation"] == "square_root"
assert "no territorial consolidation" in metadata["population_basis"]
assert metadata["population_source"].startswith("UN World Population Prospects 2024")
assert metadata["pool_xtc"] == str(POOL)
assert metadata["pool_atomic"] == str(POOL * ATOMIC)
assert metadata["equal_share_per_state_xtc"] == str(EQUAL)
assert metadata["equal_pool_xtc"] == str(EQUAL * STATES)
assert metadata["population_pool_xtc"] == str(POPULATION_POOL) == "96500000"

assert len(rows) == STATES
iso3 = [row["iso3"] for row in rows]
assert len(set(iso3)) == STATES
# Not UN Member States, and not added to any Member State.
assert not {"HKG", "MAC", "TWN", "COK", "NIU", "ESH", "PSE", "VAT"}.intersection(iso3)

# Independent recomputation: square root of population, largest remainder.
populations = {row["iso3"]: int(row["population_2026"]) for row in rows}
assert all(value > 0 for value in populations.values())
weights = {code: Decimal(value).sqrt() for code, value in populations.items()}
total_weight = sum(weights.values())
exact = {code: Decimal(POPULATION_POOL) * weight / total_weight for code, weight in weights.items()}
share = {code: int(value) for code, value in exact.items()}
leftover = POPULATION_POOL - sum(share.values())
assert 0 <= leftover < STATES
for code in sorted(exact, key=lambda c: (-(exact[c] - share[c]), c))[:leftover]:
    share[code] += 1
assert metadata["rounding_units_assigned"] == leftover

for row in rows:
    code = row["iso3"]
    assert int(row["equal_xtc"]) == EQUAL, code
    assert int(row["population_share_xtc"]) == share[code], code
    assert int(row["allocation_xtc"]) == EQUAL + share[code], code
    assert int(row["allocation_atomic"]) == (EQUAL + share[code]) * ATOMIC, code

assert sum(int(row["allocation_xtc"]) for row in rows) == POOL
assert sum(int(row["allocation_atomic"]) for row in rows) == POOL * ATOMIC

by_code = {row["iso3"]: row for row in rows}
for example in metadata["examples"].values():
    row = by_code[example["iso3"]]
    assert int(row["population_2026"]) == example["population_2026"], example["iso3"]
    assert int(row["allocation_xtc"]) == example["allocation_xtc"], example["iso3"]

# The canonical script must agree with the published files as well.
script = network / "compute-sovereign-allocation-2026.py"
subprocess.run([sys.executable, "-I", str(script), "--check"], check=True, stdout=subprocess.DEVNULL)

print(f"positions={STATES}")
print("territorial_consolidation=NONE")
print(f"allocation_total_xtc={POOL}")
print(f"population_pool_xtc={POPULATION_POOL} rounding_units={leftover}")
print("canonical_script_check=OK")
print("validation=OK")
