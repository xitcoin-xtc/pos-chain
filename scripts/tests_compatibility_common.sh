#!/usr/bin/env bash

# Common helper functions for CI compatibility scripts

set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"

# Start evmd node in background
start_node() {
	local print_log="${1:-false}"
	pushd "$ROOT" >/dev/null
	if [ "$print_log" = true ]; then
		./local_node.sh -y &
	else
		./local_node.sh -y >/tmp/evmd.log 2>&1 &
	fi
	NODE_PID=$!
	popd >/dev/null
}

# Wait until the node reaches the given block number
wait_for_node() {
	local target="${1:-10}"
	local rpc="http://127.0.0.1:8545"
	local timeout=60
	local elapsed=0
	echo "Waiting for evmd node to be ready..."
	while [ $elapsed -lt $timeout ]; do
		RESPONSE=$(curl -s -X POST -H "Content-Type: application/json" \
			--data '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}' \
			"$rpc" 2>/dev/null || true)
		if [ -n "$RESPONSE" ]; then
			BLOCK_HEX=$(echo "$RESPONSE" | grep -o '"result":"[^"]*"' | cut -d'"' -f4 || true)
			if [ -n "$BLOCK_HEX" ] && [ "$BLOCK_HEX" != "null" ]; then
				if BLOCK_NUMBER=$((16#${BLOCK_HEX#0x})); then
					echo "Current block number: $BLOCK_NUMBER (waiting for >= $target)"
					if [ "$BLOCK_NUMBER" -ge "$target" ]; then
						echo "Node is ready! Block number: $BLOCK_NUMBER"
						break
					fi
				fi
			fi
		fi
		echo "Waiting for node... ($elapsed/$timeout seconds)"
		sleep 2
		elapsed=$((elapsed + 2))
	done
	if [ $elapsed -ge $timeout ]; then
		echo "Error: Node failed to reach block $target within $timeout seconds"
		echo "Last response: $RESPONSE"
		echo "Checking node logs:"
		tail -20 /tmp/evmd.log 2>/dev/null || echo "No evmd logs found"
		exit 1
	fi
}

# Read the configured EIP-155 ID, independently of the JSON-RPC result under test.
load_test_evm_chain_id() {
	local config_file="${1:-$HOME/.evmd/config/app.toml}"
	local configured_id
	configured_id="$(awk '
		/^[[:space:]]*\[evm\][[:space:]]*(#.*)?$/ { inside_evm=1; next }
		/^[[:space:]]*\[/ { inside_evm=0 }
		inside_evm && /^[[:space:]]*evm-chain-id[[:space:]]*=/ {
			line=$0
			sub(/^[^=]*=[[:space:]]*/, "", line)
			sub(/[[:space:]]*(#.*)?$/, "", line)
			print line
		}' "$config_file")" || return 1
	if [[ ! "$configured_id" =~ ^[1-9][0-9]*$ ]]; then
		echo "Invalid or missing configured test EVM chain ID" >&2
		return 1
	fi
	EXPECTED_EVM_CHAIN_ID="$configured_id"
	export EXPECTED_EVM_CHAIN_ID
}

# Public identity of the validator created by local_node.sh; no private key export.
load_test_validator_address() {
	local address
	address="$(evmd keys show mykey -a --bech val --keyring-backend test --home "${1:-$HOME/.evmd}")" || return 1
	if [[ ! "$address" =~ ^[a-z0-9]+valoper1[a-z0-9]+$ ]]; then
		echo "Invalid disposable test validator address" >&2
		return 1
	fi
	TEST_VALIDATOR_ADDRESS="$address"
	export TEST_VALIDATOR_ADDRESS
}

# Stop the node
cleanup_node() {
	if [ -n "${NODE_PID:-}" ]; then
		echo "Stopping evmd node..."
		kill "$NODE_PID" 2>/dev/null || true
		wait "$NODE_PID" 2>/dev/null || true
	fi
}

# Run the dependency setup script
setup_compatibility_tests() {
	local print_log="${1:-false}"
	echo "Running tests_compatibility_setup.sh..."
	if [ "$print_log" = true ]; then
		"$ROOT/scripts/tests_compatibility_setup.sh"
	else
		if ! "$ROOT/scripts/tests_compatibility_setup.sh" >/tmp/tests_compatibility_setup.log 2>&1; then
			tail -80 /tmp/tests_compatibility_setup.log >&2
			return 1
		fi
	fi
}
