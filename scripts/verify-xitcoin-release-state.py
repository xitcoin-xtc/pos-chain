#!/usr/bin/env python3
"""Check the identity of the active testnet: chain IDs, native denom and
genesis checksum, and the denom enforced by validator admission."""
import hashlib
import json
from pathlib import Path

root = Path(__file__).resolve().parents[1]
network = root / "networks/xitcoin-testnet-v2-1"
chain = json.loads((network / "chain.json").read_text())
retired = json.loads((root / "networks/xitcoin-testnet-1/chain.json").read_text())
genesis_path = network / "genesis.json"
genesis = json.loads(genesis_path.read_text())
app_state = genesis["app_state"]
denom = "axtc"

# Chain IDs and native currency.
assert chain["status"] == "active"
assert retired["status"] == "retired"
assert chain["cosmos_chain_id"] == genesis["chain_id"] == "xitcoin-testnet-v2-1"
assert chain["evm_chain_id"] == 101089
assert chain["evm_chain_id_hex"] == "0x18ae1"
assert chain["native_currency"] == {
    "name": "Xitcoin",
    "symbol": "XTC",
    "decimals": 18,
    "base_denom": denom,
}

# Genesis checksum: the file, genesis.sha256 and chain.json agree.
expected = (network / "genesis.sha256").read_text().split()[0]
actual = hashlib.sha256(genesis_path.read_bytes()).hexdigest()
assert actual == expected == chain["genesis"]["sha256"], "genesis checksum mismatch"

# Every module of the genesis uses the native denom.
assert app_state["staking"]["params"]["bond_denom"] == denom
assert app_state["mint"]["params"]["mint_denom"] == denom
assert app_state["evm"]["params"]["evm_denom"] == denom
assert {coin["denom"] for coin in app_state["gov"]["params"]["min_deposit"]} == {denom}
assert [meta["base"] for meta in app_state["bank"]["denom_metadata"]] == [denom]

# Validator admission only accepts self-delegation in the native denom.
policy = (root / "x/validatoradmission/types/policy.go").read_text()
assert f'coin.Denom != "{denom}"' in policy
assert "ValidateGenesisPolicy" in policy

# Local and test containers must not run privileged.
for path in (
    root / "local_node.sh",
    root / "tests/jsonrpc/docker-compose.yml",
    root / "tests/jsonrpc/scripts/evmd/start-evmd.sh",
):
    assert "--privileged" not in path.read_text(), path

print("chain_identity=OK")
print("genesis_checksum=OK")
print("genesis_denom=OK")
print("validator_admission_denom=OK")
print("no_privileged_containers=OK")
