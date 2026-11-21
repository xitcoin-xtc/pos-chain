#!/usr/bin/env python3
"""Blocking validation for a candidate Xitcoin mainnet genesis."""

import argparse
import base64
import json
from decimal import Decimal, InvalidOperation
from pathlib import Path


ATOMIC_PER_XTC = 10**18
MAX_SUPPLY_ATOMIC = 5_250_000_000 * ATOMIC_PER_XTC
VALIDATOR_STAKE_ATOMIC = 5_000_000 * ATOMIC_PER_XTC
# The Xitcoin Testnet 2-of-3 administrative multisig. evmd/config/network.go
# refuses it as the mainnet authority; it must not appear in a mainnet genesis.
TESTNET_ADMINISTRATIVE_MULTISIG = "xtc1vza8zsgvrfwmve084ytd8xqdkkm7u9e5csctc2"
EXPECTED_MONIKERS = {
    "Xitcoin Atlas",
    "Xitcoin Borealis",
    "Xitcoin Meridian",
    "Xitcoin Zenith",
}


def fail(message):
    raise AssertionError(message)


def require(mapping, *path):
    value = mapping
    for key in path:
        if not isinstance(value, dict) or key not in value:
            fail(f"missing genesis path: {'.'.join(path)}")
        value = value[key]
    return value


def parse_xtc(value):
    try:
        amount = Decimal(value)
    except InvalidOperation as exc:
        raise argparse.ArgumentTypeError("invalid XTC amount") from exc
    atomic = amount * ATOMIC_PER_XTC
    if amount <= 0 or atomic != atomic.to_integral_value():
        raise argparse.ArgumentTypeError(
            "amount must be positive with no more than 18 decimals"
        )
    return int(atomic)


def load_json(path):
    return json.loads(path.read_text(encoding="utf-8"))


def collect_testnet_identities(testnet):
    app = require(testnet, "app_state")
    accounts = {
        account["address"] for account in require(app, "auth", "accounts")
    }
    operator_addresses = set()
    consensus_keys = set()
    signer_keys = set()
    for gentx in require(app, "genutil", "gen_txs"):
        message = gentx["body"]["messages"][0]
        operator_addresses.add(message["validator_address"])
        consensus_keys.add(message["pubkey"]["key"])
        for signer in gentx["auth_info"]["signer_infos"]:
            signer_keys.add(signer["public_key"]["key"])
    authority = require(app, "validator_admission", "authority")
    return accounts, operator_addresses, consensus_keys, signer_keys, authority


def validate(candidate, testnet, expected_bootstrap):
    if candidate.get("chain_id") != "xitcoin":
        fail("mainnet Cosmos chain ID must be xitcoin")
    if candidate.get("initial_height") != 1:
        fail("mainnet must start at height 1")
    if not candidate.get("genesis_time"):
        fail("genesis_time is required")

    serialized = json.dumps(candidate, sort_keys=True)
    if "xitcoin-testnet" in serialized:
        fail("testnet identity remains in candidate genesis")
    if TESTNET_ADMINISTRATIVE_MULTISIG in serialized:
        fail("candidate genesis contains the testnet administrative multisig")

    app = require(candidate, "app_state")
    if require(app, "bridge", "paused") is not True:
        fail("bridge must be paused in the mainnet genesis")

    for path in (
        ("evm", "params", "evm_denom"),
        ("evm", "params", "extended_denom_options", "extended_denom"),
        ("staking", "params", "bond_denom"),
        ("mint", "params", "mint_denom"),
    ):
        if require(app, *path) != "axtc":
            fail(f"{'.'.join(path)} must be axtc")

    mint = require(app, "mint")
    for key in ("inflation", "annual_provisions"):
        if Decimal(require(mint, "minter", key)) != 0:
            fail(f"mint.minter.{key} must be zero")
    for key in ("inflation_rate_change", "inflation_max", "inflation_min"):
        if Decimal(require(mint, "params", key)) != 0:
            fail(f"mint.params.{key} must be zero")
    if int(require(mint, "params", "max_supply")) != MAX_SUPPLY_ATOMIC:
        fail("max supply must be 5,250,000,000 XTC")

    bank = require(app, "bank")
    supply = require(bank, "supply")
    if supply != [{"denom": "axtc", "amount": str(expected_bootstrap)}]:
        fail("declared initial supply must exactly equal the Cronos bootstrap")

    balances = require(bank, "balances")
    balance_total = 0
    balance_addresses = set()
    for balance in balances:
        address = balance["address"]
        if address in balance_addresses:
            fail(f"duplicate bank balance: {address}")
        balance_addresses.add(address)
        coins = balance["coins"]
        if len(coins) != 1 or coins[0]["denom"] != "axtc":
            fail(f"unexpected bank coin set for {address}")
        amount = int(coins[0]["amount"])
        if amount <= 0:
            fail(f"non-positive balance for {address}")
        balance_total += amount
    if balance_total != expected_bootstrap:
        fail("sum(bank balances) does not equal the Cronos bootstrap")

    metadata = require(bank, "denom_metadata")
    if len(metadata) != 1:
        fail("exactly one native denomination metadata record is required")
    native = metadata[0]
    if (native.get("base"), native.get("display"), native.get("symbol")) != (
        "axtc",
        "xtc",
        "XTC",
    ):
        fail("native denomination metadata is inconsistent")
    units = {unit["denom"]: unit["exponent"] for unit in native["denom_units"]}
    if units != {"axtc": 0, "xtc": 18}:
        fail("native denomination precision must be 18 decimals")

    (
        test_accounts,
        test_operators,
        test_consensus_keys,
        test_signer_keys,
        test_authority,
    ) = collect_testnet_identities(testnet)

    accounts = require(app, "auth", "accounts")
    account_addresses = {account["address"] for account in accounts}
    if account_addresses & test_accounts:
        fail("candidate reuses one or more testnet account addresses")

    gentxs = require(app, "genutil", "gen_txs")
    if len(gentxs) != 4:
        fail("mainnet must start with exactly four reviewed validator gentxs")

    monikers = set()
    operators = set()
    consensus_keys = set()
    signer_keys = set()
    delegated_total = 0
    for gentx in gentxs:
        messages = gentx["body"]["messages"]
        if len(messages) != 1:
            fail("each gentx must contain exactly one message")
        message = messages[0]
        if message.get("@type") != "/cosmos.staking.v1beta1.MsgCreateValidator":
            fail("unexpected gentx message type")
        monikers.add(message["description"]["moniker"])
        operators.add(message["validator_address"])
        consensus_keys.add(message["pubkey"]["key"])
        value = message["value"]
        if value != {"denom": "axtc", "amount": str(VALIDATOR_STAKE_ATOMIC)}:
            fail("every initial validator must self-delegate exactly 5,000,000 XTC")
        if message["min_self_delegation"] != str(VALIDATOR_STAKE_ATOMIC):
            fail("validator minimum self-delegation is inconsistent")
        delegated_total += int(value["amount"])
        for signer in gentx["auth_info"]["signer_infos"]:
            signer_keys.add(signer["public_key"]["key"])
        for signature in gentx["signatures"]:
            base64.b64decode(signature, validate=True)

    if monikers != EXPECTED_MONIKERS:
        fail("initial validator monikers do not match the reviewed set")
    if len(operators) != 4 or len(consensus_keys) != 4 or len(signer_keys) != 4:
        fail("validator identities are not distinct")
    if operators & test_operators:
        fail("candidate reuses testnet validator operator addresses")
    if consensus_keys & test_consensus_keys:
        fail("candidate reuses testnet consensus public keys")
    if signer_keys & test_signer_keys:
        fail("candidate reuses testnet gentx signer public keys")

    admission = require(app, "validator_admission")
    if set(admission["approved_validators"]) != operators:
        fail("validator-admission allowlist must exactly match the four gentxs")
    if admission["authority"] == test_authority:
        fail("candidate reuses the testnet validator-admission authority")
    for module in ("validator_admission", "validator_incentives"):
        authority = require(app, module, "authority")
        if not authority or authority in {test_authority, TESTNET_ADMINISTRATIVE_MULTISIG}:
            fail(f"{module}.authority must be the reviewed mainnet authority, not empty or the testnet multisig")
    if admission["minimum_self_delegation"] != f"{VALIDATOR_STAKE_ATOMIC}axtc":
        fail("validator-admission minimum self-delegation is inconsistent")

    if expected_bootstrap < delegated_total:
        fail("bootstrap amount is smaller than initial validator stake")

    return {
        "initial_supply_atomic": expected_bootstrap,
        "initial_validator_stake_atomic": delegated_total,
        "operational_balance_atomic": expected_bootstrap - delegated_total,
        "validators": sorted(monikers),
    }


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--genesis", required=True, type=Path)
    parser.add_argument(
        "--expected-bootstrap-xtc", required=True, type=parse_xtc
    )
    parser.add_argument(
        "--testnet-genesis",
        type=Path,
        default=Path(__file__).resolve().parents[1]
        / "networks/xitcoin-testnet-v2-1/genesis.json",
    )
    args = parser.parse_args()

    result = validate(
        load_json(args.genesis),
        load_json(args.testnet_genesis),
        args.expected_bootstrap_xtc,
    )
    print(json.dumps(result, indent=2, sort_keys=True))
    print("mainnet_genesis_validation=OK")


if __name__ == "__main__":
    main()
